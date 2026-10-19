// CVTROAD — 파일 변환 앱 (macOS)
// 이미지 · 영상 · 오디오 · 문서 · PDF · 압축 파일을 이 컴퓨터 안에서 바꿔요. 파일은 밖으로 보내지 않아요.
// made by. Nevertheless_D
package main

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// build is the app version, set with -ldflags "-X main.build=1.2.3".
var build = "0.0.0-dev"

//go:embed web
var webFiles embed.FS

//go:embed helper.js
var helperJS []byte

type Settings struct {
	Appearance   string `json:"appearance"`   // system | light | dark
	OutMode      string `json:"outMode"`      // beside(원본 옆) | folder(기본 폴더)
	OutDir       string `json:"outDir"`       // 기본 폴더
	Reveal       bool   `json:"reveal"`       // 끝나면 Finder에서 보기
	Notify       bool   `json:"notify"`       // 끝나면 알림
	ImageQuality int    `json:"imageQuality"` // 50–100
	AudioQuality string `json:"audioQuality"` // 128 | 192 | 320 | lossless
	VideoQuality string `json:"videoQuality"` // high | normal | small
	VideoSize    string `json:"videoSize"`    // same | 1080 | 720 | 480
	PDFDpi       int    `json:"pdfDpi"`       // PDF → 그림 해상도
	AutoUpdate   bool   `json:"autoUpdate"`
	Agent        bool   `json:"agent"`      // 백그라운드 도우미 (앱을 열지 않아도 쓰기)
	ShiftDrag    bool   `json:"shiftDrag"`  // Shift-드래그 원형 메뉴
	Bubble       bool   `json:"bubble"`     // 화면에 떠 있는 변환 창
	BubbleSide   string `json:"bubbleSide"` // right | left
	Seen         bool   `json:"seen"`       // 처음 안내를 봤는지
}

func defaultSettings() Settings {
	return Settings{
		Appearance: "system", OutMode: "beside", OutDir: downloadsFolder(), Notify: true,
		ImageQuality: 85, AudioQuality: "320", VideoQuality: "normal", VideoSize: "same", PDFDpi: 150,
		AutoUpdate: true, Agent: true, ShiftDrag: true, BubbleSide: "right",
	}
}

type App struct {
	support string
	token   string
	port    int
	binDir  string
	home    string

	mu       sync.Mutex
	settings Settings

	jobs  *JobManager
	tools *Tools
	upd   *Updater
	agent *Agent

	allowMu sync.Mutex
	allowed map[string]bool // 화면에 보여 줘도 되는 파일 (고른 파일 · 만든 파일)

	lastSeen atomic.Int64
	lastPage atomic.Int64 // 열려 있는 앱 창이 마지막으로 물어본 때
	server   bool
}

var app *App

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--run": // 백그라운드 도우미가 부르는 변환
			newApp(false)
			os.Exit(runCLI(os.Args[2:]))
		case "--quick": // Finder 빠른 동작 · 단축키
			newApp(false)
			os.Exit(runQuick(os.Args[2:]))
		case "--set": // 도우미 메뉴에서 설정 바꾸기: --set shiftDrag false
			newApp(false)
			os.Exit(runSet(os.Args[2:]))
		case "--agent": // 도우미 켜기/끄기/상태 (설치 스크립트 · 시험용)
			newApp(false)
			os.Exit(runAgentCmd(os.Args[2:]))
		default:
			// 모르는 옵션으로 불렸을 때 엉뚱하게 두 번째 엔진(서버)이 켜지지 않게
			if strings.HasPrefix(os.Args[1], "-") {
				fmt.Fprintln(os.Stderr, "cvtroad-engine: 알 수 없는 옵션", os.Args[1])
				os.Exit(2)
			}
		}
	}
	runEngine()
}

func newApp(server bool) {
	log.SetFlags(log.LstdFlags)
	home, _ := os.UserHomeDir()
	support := os.Getenv("CVTROAD_SUPPORT")
	if support == "" {
		support = defaultSupportDir(home, "CVTROAD")
	}
	app = &App{support: support, binDir: filepath.Join(support, "bin"), home: home, server: server, allowed: map[string]bool{}}
	os.MkdirAll(app.binDir, 0o755)
	logName := "cli.log"
	if server {
		logName = "engine.log"
	}
	if f, err := os.OpenFile(filepath.Join(support, logName), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
		if st, _ := f.Stat(); st != nil && st.Size() > 2<<20 {
			f.Truncate(0)
		}
		log.SetOutput(io.MultiWriter(os.Stderr, f))
	}
	app.loadSettings()
	app.tools = NewTools(support, app.binDir)
	app.agent = NewAgent(support, app.binDir)
}

func runEngine() {
	newApp(true)
	token := os.Getenv("CVTROAD_TOKEN")
	if token == "" {
		token = randID(16)
	}
	app.token = token
	app.lastSeen.Store(time.Now().Unix())
	app.startDiag() // 지난번에 갑자기 멈췄는지 확인 · 이번 오류는 기록
	writeFileAtomic(filepath.Join(app.binDir, "helper.js"), helperJS, 0o644)
	os.RemoveAll(filepath.Join(app.support, "tmp"))
	cleanOld(filepath.Join(app.support, "uploads"), 2)
	cleanOld(filepath.Join(app.support, "thumbs"), 30)

	app.jobs = NewJobManager()
	app.upd = NewUpdater(app.support)
	if p := os.Getenv("CVTROAD_APP"); p != "" {
		writeFileAtomic(filepath.Join(app.support, "app-path"), []byte(p), 0o644)
	}

	ln := listen(os.Getenv("CVTROAD_PORT"))
	app.port = ln.Addr().(*net.TCPAddr).Port
	writeFileAtomic(filepath.Join(app.support, "token"), []byte(token), 0o600)
	writeFileAtomic(filepath.Join(app.support, "port"), []byte(strconv.Itoa(app.port)), 0o644)
	log.Printf("CVTROAD %s listening on %d", build, app.port)

	go func() { defer guard("도우미 준비"); app.agent.Sync(app.Settings(), false) }() // 도우미 파일을 최신으로 · 켜 두기
	go func() { defer guard("변환 도구 준비"); app.tools.EnsureLater() }()            // 변환 도구(ffmpeg)를 미리 준비
	go func() { defer guard("쉬는 시간 확인"); app.idleWatcher() }()
	go func() { defer guard("자동 업데이트"); app.upd.autoLoop() }()
	srv := &http.Server{Handler: hostGuard(app.routes())}
	log.Fatal(srv.Serve(ln))
}

// cleanOld removes entries older than days.
func cleanOld(dir string, days int) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cut := time.Now().AddDate(0, 0, -days)
	for _, e := range ents {
		if info, err := e.Info(); err == nil && info.ModTime().Before(cut) {
			os.RemoveAll(filepath.Join(dir, e.Name()))
		}
	}
}

func defaultSupportDir(home, name string) string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", name)
	case "windows":
		return filepath.Join(os.Getenv("LOCALAPPDATA"), name)
	default:
		return filepath.Join(home, "."+strings.ToLower(name))
	}
}

func listen(pref string) net.Listener {
	if p, err := strconv.Atoi(pref); err == nil && p > 0 {
		for i := 0; i < 20; i++ {
			if ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p)); err == nil {
				return ln
			}
			time.Sleep(150 * time.Millisecond)
		}
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	return ln
}

// 창을 닫고 (깨어 있는 시간으로) 2시간 동안 아무 요청이 없고, 하는 작업도 없으면 스스로 종료합니다.
// 맥이 잠자기에서 깨어나면 시계가 훌쩍 넘어가므로, 벽시계 대신 20초 틱 수로 셉니다.
// (앱을 열지 않아도 쓰는 기능은 백그라운드 도우미가 맡아요)
func (a *App) idleWatcher() {
	idleTicks, last := 0, a.lastSeen.Load()
	for range time.Tick(20 * time.Second) {
		if seen := a.lastSeen.Load(); seen != last || a.jobs.Busy() || a.upd.Busy() || a.tools.Running() {
			idleTicks, last = 0, seen
			continue
		}
		idleTicks++
		if idleTicks*20 >= 7200 {
			a.shutdown()
		}
	}
}

func (a *App) shutdown() {
	log.Printf("엔진을 끕니다")
	a.clearRunning()
	a.jobs.CancelAll()
	if b, err := os.ReadFile(filepath.Join(a.support, "port")); err == nil && string(b) == strconv.Itoa(a.port) {
		os.Remove(filepath.Join(a.support, "port"))
	}
	os.Exit(0)
}

// ---------------------------------------------------------------- settings

func (a *App) settingsPath() string { return filepath.Join(a.support, "settings.json") }

func (a *App) loadSettings() {
	s := defaultSettings()
	if b, err := os.ReadFile(a.settingsPath()); err == nil {
		json.Unmarshal(b, &s)
	}
	a.settings = fixSettings(s)
}

func fixSettings(s Settings) Settings {
	d := defaultSettings()
	switch s.Appearance {
	case "system", "light", "dark":
	default:
		s.Appearance = d.Appearance
	}
	if s.OutMode != "folder" {
		s.OutMode = "beside"
	}
	if s.OutDir == "" {
		s.OutDir = d.OutDir
	}
	if s.ImageQuality < 40 || s.ImageQuality > 100 {
		s.ImageQuality = d.ImageQuality
	}
	switch s.AudioQuality {
	case "128", "192", "320", "lossless":
	default:
		s.AudioQuality = d.AudioQuality
	}
	switch s.VideoQuality {
	case "high", "normal", "small":
	default:
		s.VideoQuality = d.VideoQuality
	}
	switch s.VideoSize {
	case "same", "1080", "720", "480":
	default:
		s.VideoSize = d.VideoSize
	}
	if s.PDFDpi < 72 || s.PDFDpi > 600 {
		s.PDFDpi = d.PDFDpi
	}
	if s.BubbleSide != "left" {
		s.BubbleSide = "right"
	}
	return s
}

func (a *App) saveSettings() {
	b, _ := json.MarshalIndent(a.settings, "", "  ")
	writeFileAtomic(a.settingsPath(), b, 0o644)
}

func (a *App) Settings() Settings {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.settings
}

func downloadsFolder() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Downloads")
}

func folderLabel(p string) string {
	home, _ := os.UserHomeDir()
	names := map[string]string{
		filepath.Join(home, "Downloads"): "다운로드",
		filepath.Join(home, "Desktop"):   "바탕 화면",
		filepath.Join(home, "Documents"): "문서",
		filepath.Join(home, "Pictures"):  "사진",
		filepath.Join(home, "Movies"):    "동영상",
		filepath.Join(home, "Music"):     "음악",
		home:                             "홈",
	}
	if n, ok := names[p]; ok {
		return n
	}
	return filepath.Base(p)
}

// ---------------------------------------------------------------- routes

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	sub, _ := fs.Sub(webFiles, "web")
	static := http.FileServer(http.FS(sub))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			if r.URL.Query().Get("t") != a.token {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusForbidden)
				io.WriteString(w, "<meta charset=utf-8><body style='font:15px -apple-system;padding:40px'>CVTROAD 앱을 다시 실행해 주세요.</body>")
				return
			}
			b, _ := webFiles.ReadFile("web/index.html")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.Write(b)
			return
		}
		if strings.HasSuffix(r.URL.Path, ".js") || strings.HasSuffix(r.URL.Path, ".css") || strings.HasSuffix(r.URL.Path, ".html") {
			w.Header().Set("Cache-Control", "no-cache")
		}
		static.ServeHTTP(w, r)
	})

	// 원형 메뉴 화면 (도우미와 같은 파일 — 앱의 "도우미" 화면에서 체험용으로 보여 줘요)
	ringFS, _ := fs.Sub(agentFiles, "agent")
	ringFiles := http.StripPrefix("/ring/", http.FileServer(http.FS(ringFS)))
	mux.HandleFunc("/ring/", func(w http.ResponseWriter, r *http.Request) {
		// 도우미 폴더에서는 글꼴이 ring.html 옆에 복사돼요 — 체험 화면에서도 같은 글꼴을 줘요
		if strings.HasSuffix(r.URL.Path, ".woff2") {
			if b, err := webFiles.ReadFile("web/fonts/" + filepath.Base(r.URL.Path)); err == nil {
				w.Header().Set("Content-Type", "font/woff2")
				w.Write(b)
				return
			}
		}
		ringFiles.ServeHTTP(w, r)
	})

	api := func(path string, h http.HandlerFunc) {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("x-token") != a.token && r.URL.Query().Get("t") != a.token {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			a.lastSeen.Store(time.Now().Unix())
			if path != "/api/ping" {
				a.lastPage.Store(time.Now().Unix())
			}
			h(w, r)
		})
	}

	api("/api/ping", func(w http.ResponseWriter, r *http.Request) {
		// page: 열려 있는 앱 창이 방금 다시 연결됐는지 (실행기가 창을 또 열지 판단해요)
		page := time.Now().Unix()-a.lastPage.Load() < 8
		writeJSON(w, map[string]any{"ok": true, "build": build, "page": page})
	})
	api("/api/quit", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"ok": true})
		go func() { time.Sleep(300 * time.Millisecond); a.shutdown() }()
	})
	api("/api/state", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"build":    build,
			"os":       uiOS(),
			"settings": a.settingsJSON(),
			"formats":  kinds,
			"tools":    a.tools.State(),
			"agent":    a.agent.Status(),
			"update":   a.upd.State(),
			"helper":   helperAvailable(),
			"crash":    lastCrashInfo(),
		})
	})
	api("/api/poll", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"build":   build,
			"jobs":    a.jobs.Snapshot(),
			"update":  a.upd.State(),
			"tools":   a.tools.State(),
			"pending": a.peekPending(),
		})
	})
	api("/api/settings", a.handleSettings)
	api("/api/choose-folder", a.handleChooseFolder)
	api("/api/choose-files", a.handleChooseFiles)
	api("/api/resolve", a.handleResolve)
	api("/api/upload", a.handleUpload)
	api("/api/info", a.handleInfo)
	api("/api/thumb", a.handleThumb)
	api("/api/file", a.handleFile)
	api("/api/open", a.handleOpen)
	api("/api/copy", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Text string `json:"text"`
		}
		readJSON(r, &in)
		copyText(in.Text)
		writeJSON(w, map[string]any{"ok": true})
	})
	api("/api/pending", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"ok": true, "pending": a.takePending()})
	})
	api("/api/pdf/pages", a.handlePDFPages)

	// 변환 작업
	api("/api/jobs/start", a.handleJobStart)
	api("/api/jobs/cancel", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID string `json:"id"`
		}
		readJSON(r, &in)
		a.jobs.Cancel(in.ID)
		writeJSON(w, map[string]any{"ok": true})
	})
	api("/api/jobs/clear", func(w http.ResponseWriter, r *http.Request) {
		a.jobs.ClearFinished()
		writeJSON(w, map[string]any{"ok": true})
	})

	// 변환 도구 (ffmpeg)
	api("/api/tools/install", func(w http.ResponseWriter, r *http.Request) {
		a.tools.Start()
		writeJSON(w, a.tools.State())
	})

	// 백그라운드 도우미
	api("/api/agent", a.handleAgent)

	// 업데이트
	api("/api/update/state", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, a.upd.State()) })
	api("/api/update/check", func(w http.ResponseWriter, r *http.Request) {
		a.upd.Check(true)
		writeJSON(w, a.upd.State())
	})
	api("/api/update/install", func(w http.ResponseWriter, r *http.Request) {
		go func() { defer guard("업데이트 설치"); a.upd.Install() }()
		writeJSON(w, map[string]any{"ok": true})
	})
	api("/api/update/rollback", func(w http.ResponseWriter, r *http.Request) {
		if err := a.upd.Rollback(); err != nil {
			jsonError(w, 500, err.Error())
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	})
	api("/api/update/restart", func(w http.ResponseWriter, r *http.Request) {
		if err := a.upd.Restart(a.port, a.token); err != nil {
			jsonError(w, 500, err.Error())
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	})
	return mux
}

// Only answer requests addressed to 127.0.0.1 / localhost (DNS rebinding protection).
func hostGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if host != "127.0.0.1" && host != "localhost" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) settingsJSON() map[string]any {
	s := a.Settings()
	b, _ := json.Marshal(s)
	m := map[string]any{}
	json.Unmarshal(b, &m)
	m["outDirLabel"] = folderLabel(s.OutDir)
	m["downloads"] = downloadsFolder()
	return m
}

func (a *App) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var patch map[string]json.RawMessage
		if err := readJSON(r, &patch); err != nil {
			jsonError(w, 400, "설정을 읽지 못했어요")
			return
		}
		a.mu.Lock()
		cur, _ := json.Marshal(a.settings)
		m := map[string]json.RawMessage{}
		json.Unmarshal(cur, &m)
		for k, v := range patch {
			m[k] = v
		}
		merged, _ := json.Marshal(m)
		var s Settings
		json.Unmarshal(merged, &s)
		if s.OutDir != "" {
			if st, err := os.Stat(s.OutDir); err != nil || !st.IsDir() {
				s.OutDir = a.settings.OutDir
			}
		}
		prev := a.settings
		a.settings = fixSettings(s)
		a.saveSettings()
		now := a.settings
		a.mu.Unlock()
		if prev.Agent != now.Agent || prev.ShiftDrag != now.ShiftDrag || prev.Bubble != now.Bubble || prev.BubbleSide != now.BubbleSide || prev.Appearance != now.Appearance {
			go func() { defer guard("도우미 설정"); a.agent.Sync(now, false) }()
		}
	}
	writeJSON(w, map[string]any{"ok": true, "settings": a.settingsJSON()})
}

// ---------------------------------------------------------------- files: allow list

func (a *App) allow(paths ...string) {
	a.allowMu.Lock()
	for _, p := range paths {
		a.allowed[filepath.Clean(p)] = true
	}
	a.allowMu.Unlock()
}

func (a *App) isAllowed(p string) bool {
	a.allowMu.Lock()
	defer a.allowMu.Unlock()
	p = filepath.Clean(p)
	if a.allowed[p] {
		return true
	}
	// 고른 폴더 안의 파일, 만든 폴더 안의 파일
	for d := filepath.Dir(p); d != "/" && d != "." && len(d) > 1; d = filepath.Dir(d) {
		if a.allowed[d] {
			return true
		}
	}
	return false
}

type FileInfo struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	Ext      string `json:"ext"`
	Kind     string `json:"kind"`
	Size     int64  `json:"size"`
	Mtime    int64  `json:"mtime"`
	Dir      bool   `json:"dir"`
	Uploaded bool   `json:"uploaded,omitempty"`
	Missing  bool   `json:"missing,omitempty"`
}

func fileInfo(p string) FileInfo {
	fi := FileInfo{Path: p, Name: filepath.Base(p), Ext: extOf(p), Kind: kindOf(p)}
	st, err := os.Stat(p)
	if err != nil {
		fi.Missing = true
		return fi
	}
	fi.Size, fi.Mtime, fi.Dir = st.Size(), st.ModTime().UnixMilli(), st.IsDir()
	if fi.Dir {
		fi.Size = dirSize(p, 20000)
	}
	if strings.HasPrefix(p, filepath.Join(app.support, "uploads")+string(os.PathSeparator)) {
		fi.Uploaded = true
		// 올린 파일은 "up-xxxx" 폴더 안에 원래 이름으로 있어요
	}
	return fi
}

func dirSize(p string, limit int) int64 {
	var total int64
	n := 0
	filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		n++
		if n > limit {
			return filepath.SkipAll
		}
		if !d.IsDir() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}

func (a *App) handleInfo(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Paths []string `json:"paths"`
	}
	readJSON(r, &in)
	out := []FileInfo{}
	for _, p := range in.Paths {
		if p == "" {
			continue
		}
		p = filepath.Clean(p)
		a.allow(p)
		out = append(out, fileInfo(p))
	}
	writeJSON(w, map[string]any{"ok": true, "files": out})
}

// 창에 끌어다 놓은 파일의 진짜 위치 찾기 (이름 · 크기 · 수정 시각이 모두 같은 파일, Spotlight 사용)
func (a *App) handleResolve(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Files []struct {
			Name  string `json:"name"`
			Size  int64  `json:"size"`
			Mtime int64  `json:"mtime"`
		} `json:"files"`
	}
	readJSON(r, &in)
	out := make([]string, len(in.Files))
	for i, f := range in.Files {
		out[i] = resolveDropped(f.Name, f.Size, f.Mtime)
		if out[i] != "" {
			a.allow(out[i])
		}
	}
	writeJSON(w, map[string]any{"ok": true, "paths": out})
}

func resolveDropped(name string, size, mtime int64) string {
	cands := []string{}
	if t := os.Getenv("CVTROAD_TEST_RESOLVE_DIR"); t != "" {
		filepath.WalkDir(t, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && d.Name() == name {
				cands = append(cands, p)
			}
			return nil
		})
	} else if runtime.GOOS == "darwin" && name != "" {
		q := fmt.Sprintf("kMDItemFSName == '%s'", strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(name))
		out, _ := exec.Command("/usr/bin/mdfind", q).Output()
		for _, l := range strings.Split(string(out), "\n") {
			if l = strings.TrimSpace(l); l != "" {
				cands = append(cands, l)
			}
		}
		// Spotlight가 아직 모르는 곳: 바탕 화면 · 다운로드 · 문서 바로 아래
		home, _ := os.UserHomeDir()
		for _, d := range []string{"Desktop", "Downloads", "Documents"} {
			cands = append(cands, filepath.Join(home, d, name))
		}
	}
	for _, c := range cands {
		st, err := os.Stat(c)
		if err != nil || st.IsDir() || st.Size() != size {
			continue
		}
		if mtime > 0 {
			diff := st.ModTime().UnixMilli() - mtime
			if diff < -2000 || diff > 2000 {
				continue
			}
		}
		return c
	}
	return ""
}

// 위치를 찾지 못한 파일은 앱으로 올려서 바꿔요 (결과는 기본 폴더에 저장)
func (a *App) handleUpload(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(r.URL.Query().Get("name"))
	if name == "" || name == "." || name == "/" {
		jsonError(w, 400, "파일 이름이 없어요")
		return
	}
	dir := filepath.Join(a.support, "uploads", "up-"+randID(4))
	os.MkdirAll(dir, 0o755)
	dest := filepath.Join(dir, name)
	f, err := os.Create(dest)
	if err != nil {
		jsonError(w, 500, "파일을 받지 못했어요")
		return
	}
	_, err = io.Copy(f, r.Body)
	f.Close()
	r.Body.Close()
	if err != nil {
		os.RemoveAll(dir)
		jsonError(w, 500, "파일을 받는 중에 끊겼어요")
		return
	}
	if m, _ := strconv.ParseInt(r.URL.Query().Get("mtime"), 10, 64); m > 0 {
		t := time.UnixMilli(m)
		os.Chtimes(dest, t, t)
	}
	a.allow(dest)
	writeJSON(w, map[string]any{"ok": true, "path": dest, "file": fileInfo(dest)})
}

func (a *App) handleFile(w http.ResponseWriter, r *http.Request) {
	p := filepath.Clean(r.URL.Query().Get("p"))
	if !a.isAllowed(p) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, p)
}

// 미리 보기 그림 (Quick Look · 영상 첫 장면)
func (a *App) handleThumb(w http.ResponseWriter, r *http.Request) {
	p := filepath.Clean(r.URL.Query().Get("p"))
	if !a.isAllowed(p) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	st, err := os.Stat(p)
	if err != nil || st.IsDir() {
		http.NotFound(w, r)
		return
	}
	dir := filepath.Join(a.support, "thumbs")
	os.MkdirAll(dir, 0o755)
	key := sha256Hex(fmt.Sprintf("%s|%d|%d", p, st.Size(), st.ModTime().UnixNano()))[:24]
	out := filepath.Join(dir, key+".png")
	if !fileExists(out) {
		if err := makeThumb(p, out, dir); err != nil {
			http.NotFound(w, r)
			return
		}
	}
	w.Header().Set("Cache-Control", "max-age=86400")
	http.ServeFile(w, r, out)
}

func makeThumb(p, out, dir string) error {
	if runtime.GOOS == "darwin" {
		tmp, _ := os.MkdirTemp(dir, "tmp-")
		defer os.RemoveAll(tmp)
		cmd := exec.Command("/usr/bin/qlmanage", "-t", "-s", "360", "-o", tmp, p)
		if err := runTimeout(cmd, 20*time.Second); err == nil {
			if m, _ := filepath.Glob(filepath.Join(tmp, "*.png")); len(m) > 0 {
				return os.Rename(m[0], out)
			}
		}
	}
	// 맥이 아닐 때(시험용) · Quick Look이 못 만들 때: ffmpeg로 첫 장면
	ff := app.tools.Path("ffmpeg")
	if !isExecutable(ff) {
		return errors.New("no thumb")
	}
	k := kindOf(p)
	if k != "image" && k != "video" {
		return errors.New("no thumb")
	}
	args := []string{"-y", "-v", "error"}
	if k == "video" {
		args = append(args, "-ss", "1")
	}
	args = append(args, "-i", p, "-frames:v", "1", "-vf", "scale='min(360,iw)':-2", out)
	return runTimeout(exec.Command(ff, args...), 20*time.Second)
}

func runTimeout(cmd *exec.Cmd, d time.Duration) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(d):
		cmd.Process.Kill()
		return errors.New("timeout")
	}
}

func (a *App) handleOpen(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Path string `json:"path"`
		How  string `json:"how"` // open | reveal | quicklook
	}
	readJSON(r, &in)
	if in.Path == "" || !fileExists(in.Path) {
		jsonError(w, 404, "파일을 찾을 수 없어요. 옮기거나 지웠을 수 있어요")
		return
	}
	switch in.How {
	case "reveal":
		revealPath(in.Path)
	case "quicklook":
		quickLook(in.Path)
	default:
		openPath(in.Path)
	}
	writeJSON(w, map[string]any{"ok": true})
}

// ---------------------------------------------------------------- 도우미가 앱에 넘긴 일 (예: PDF 페이지 정리)

type Pending struct {
	Tool  string   `json:"tool"`
	Files []string `json:"files"`
	At    int64    `json:"at"`
}

func (a *App) pendingPath() string { return filepath.Join(a.support, "pending.json") }

func (a *App) peekPending() *Pending {
	b, err := os.ReadFile(a.pendingPath())
	if err != nil {
		return nil
	}
	var p Pending
	if json.Unmarshal(b, &p) != nil || time.Now().UnixMilli()-p.At > 10*60*1000 {
		return nil
	}
	return &p
}

func (a *App) takePending() *Pending {
	p := a.peekPending()
	os.Remove(a.pendingPath())
	if p != nil {
		a.allow(p.Files...)
	}
	return p
}

// ---------------------------------------------------------------- folder / file pickers

func (a *App) handleChooseFolder(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Start  string `json:"start"`
		Prompt string `json:"prompt"`
	}
	readJSON(r, &in)
	if t := os.Getenv("CVTROAD_TEST_CHOOSE"); t != "" {
		os.MkdirAll(t, 0o755)
		writeJSON(w, map[string]any{"ok": true, "path": t, "label": folderLabel(t)})
		return
	}
	if runtime.GOOS != "darwin" {
		jsonError(w, 501, "macOS에서만 지원해요")
		return
	}
	start := in.Start
	if st, err := os.Stat(start); err != nil || !st.IsDir() {
		start = downloadsFolder()
	}
	prompt := in.Prompt
	if prompt == "" {
		prompt = "저장할 폴더를 고르세요"
	}
	out, err := osascriptLines([]string{
		"on run argv",
		"activate",
		"try",
		"set f to choose folder with prompt (item 2 of argv) default location (POSIX file (item 1 of argv))",
		"return POSIX path of f",
		"on error number -128",
		"return \"\"",
		"end try",
		"end run",
	}, start, prompt)
	if err != nil {
		jsonError(w, 500, "폴더 선택 창을 열지 못했어요")
		return
	}
	p := strings.TrimSpace(out)
	if p != "/" {
		p = strings.TrimSuffix(p, "/")
	}
	writeJSON(w, map[string]any{"ok": true, "path": p, "label": folderLabel(p)})
}

func (a *App) handleChooseFiles(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Start  string `json:"start"`
		Prompt string `json:"prompt"`
		Folder bool   `json:"folder"`
	}
	readJSON(r, &in)
	if t := os.Getenv("CVTROAD_TEST_CHOOSE_FILES"); t != "" {
		ps := strings.Split(t, "|")
		a.allow(ps...)
		writeJSON(w, map[string]any{"ok": true, "paths": ps})
		return
	}
	if runtime.GOOS != "darwin" {
		jsonError(w, 501, "macOS에서만 지원해요")
		return
	}
	start := in.Start
	if st, err := os.Stat(start); err != nil || !st.IsDir() {
		start = filepath.Join(a.home, "Desktop")
	}
	prompt := in.Prompt
	if prompt == "" {
		prompt = "바꿀 파일을 고르세요 (⌘를 누른 채 여러 개를 고를 수 있어요)"
	}
	chooser := "choose file"
	if in.Folder {
		chooser = "choose folder"
	}
	out, err := osascriptLines([]string{
		"on run argv",
		"activate",
		"try",
		"set fs to " + chooser + " with prompt (item 2 of argv) default location (POSIX file (item 1 of argv)) with multiple selections allowed",
		"set out to \"\"",
		"repeat with f in fs",
		"set out to out & POSIX path of f & linefeed",
		"end repeat",
		"return out",
		"on error number -128",
		"return \"\"",
		"end try",
		"end run",
	}, start, prompt)
	if err != nil {
		jsonError(w, 500, "파일 선택 창을 열지 못했어요")
		return
	}
	paths := []string{}
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimSpace(l)
		if l != "" {
			if l != "/" {
				l = strings.TrimSuffix(l, "/")
			}
			paths = append(paths, l)
		}
	}
	a.allow(paths...)
	writeJSON(w, map[string]any{"ok": true, "paths": paths})
}

func osascriptLines(lines []string, argv ...string) (string, error) {
	args := []string{}
	for _, l := range lines {
		args = append(args, "-e", l)
	}
	args = append(args, argv...)
	out, err := exec.Command("/usr/bin/osascript", args...).Output()
	return string(out), err
}

func notify(title, msg string) {
	if app.Settings().Notify {
		notifyOS(title, msg)
	}
}

// ---------------------------------------------------------------- utils

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": msg})
}

func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(io.LimitReader(r.Body, 32<<20)).Decode(v)
}

func randID(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func writeFileAtomic(path string, b []byte, perm os.FileMode) {
	tmp := path + ".tmp"
	if os.WriteFile(tmp, b, perm) == nil {
		os.Rename(tmp, path)
	}
}

func fileExists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

func isExecutable(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir() && (runtime.GOOS == "windows" || st.Mode()&0o111 != 0)
}

// uniquePath returns dir/name, or "name (2).ext" … when that file already exists.
func uniquePath(dir, name string) string {
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	if strings.HasSuffix(strings.ToLower(name), ".tar.gz") {
		ext = name[len(name)-7:]
		stem = name[:len(name)-7]
	}
	p := filepath.Join(dir, name)
	for n := 2; ; n++ {
		if _, err := os.Lstat(p); errors.Is(err, os.ErrNotExist) && !reserved(p) {
			reserve(p)
			return p
		}
		p = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", stem, n, ext))
	}
}

// 동시에 같은 이름을 쓰지 않도록 잠깐 맡아 두는 이름
var (
	resMu   sync.Mutex
	resName = map[string]time.Time{}
)

func reserved(p string) bool {
	resMu.Lock()
	defer resMu.Unlock()
	t, ok := resName[strings.ToLower(p)]
	return ok && time.Since(t) < 10*time.Minute
}

func reserve(p string) {
	resMu.Lock()
	resName[strings.ToLower(p)] = time.Now()
	resMu.Unlock()
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "\n"); i >= 0 {
		s = s[i+1:]
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

func init() {
	mime.AddExtensionType(".js", "text/javascript")
	mime.AddExtensionType(".mjs", "text/javascript")
	mime.AddExtensionType(".woff2", "font/woff2")
	mime.AddExtensionType(".svg", "image/svg+xml")
}

func uiOS() string {
	if o := os.Getenv("CVTROAD_FAKE_OS"); o != "" {
		return o
	}
	return runtime.GOOS
}
