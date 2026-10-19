package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 변환 도구 (처음 한 번만 받아요, 약 90MB)
//
//	ffmpeg  — 영상 · 오디오(MP3 포함) · WEBP · AVIF 변환
//	ffprobe — 파일 분석
//
// 이미지 · 문서 · PDF · 압축은 맥에 들어 있는 기능과 앱 안의 엔진으로 하니까 도구 없이도 돼요.
type toolDef struct {
	Name   string
	Label  string
	Kind   string // zip | copy
	URLs   []string
	Member string
}

func exe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func toolDefs() []toolDef {
	arch := "arm64"
	if runtime.GOARCH == "amd64" {
		arch = "amd64"
	}
	switch runtime.GOOS {
	case "darwin":
		mr := func(t string) []string {
			return []string{
				fmt.Sprintf("https://ffmpeg.martin-riedl.de/redirect/latest/macos/%s/release/%s.zip", arch, t),
				fmt.Sprintf("https://ffmpeg.martin-riedl.de/redirect/latest/macos/%s/snapshot/%s.zip", arch, t),
			}
		}
		return []toolDef{
			{"ffmpeg", "변환 엔진", "zip", mr("ffmpeg"), "ffmpeg"},
			{"ffprobe", "분석 도구", "zip", mr("ffprobe"), "ffprobe"},
		}
	case "windows":
		ff := []string{
			"https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.zip",
			"https://github.com/yt-dlp/FFmpeg-Builds/releases/download/latest/ffmpeg-master-latest-win64-gpl.zip",
		}
		return []toolDef{
			{"ffmpeg", "변환 엔진", "zip", ff, "ffmpeg.exe"},
			{"ffprobe", "분석 도구", "zip", ff, "ffprobe.exe"},
		}
	default: // linux — 개발 · 시험용
		ffdir := os.Getenv("CVTROAD_FFMPEG_DIR")
		if ffdir == "" {
			ffdir = "/usr/bin"
		}
		return []toolDef{
			{"ffmpeg", "변환 엔진", "copy", []string{filepath.Join(ffdir, "ffmpeg")}, ""},
			{"ffprobe", "분석 도구", "copy", []string{filepath.Join(ffdir, "ffprobe")}, ""},
		}
	}
}

// 다른 맥 앱(YTROAD · SROAD)이 이미 받아 둔 ffmpeg가 있으면 복사해서 써요 (같은 배포처)
func siblingBins() []string {
	home, _ := os.UserHomeDir()
	out := []string{}
	for _, n := range []string{"YTROAD", "SROAD", "YTDownloader"} {
		out = append(out, filepath.Join(defaultSupportDir(home, n), "bin"))
	}
	if p := os.Getenv("CVTROAD_SIBLING_BIN"); p != "" {
		out = append([]string{p}, out...)
	}
	return out
}

type ToolItem struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	State string `json:"state"` // wait | down | copy | done | error
	Done  int64  `json:"done"`
	Total int64  `json:"total"`
}

type Tools struct {
	mu      sync.Mutex
	support string
	bin     string
	running bool
	err     string
	items   []*ToolItem
}

func NewTools(support, bin string) *Tools {
	t := &Tools{support: support, bin: bin}
	t.resetItems()
	return t
}

func (t *Tools) Path(name string) string { return filepath.Join(t.bin, exe(name)) }

func (t *Tools) resetItems() {
	t.items = nil
	for _, d := range toolDefs() {
		st := "wait"
		if isExecutable(t.Path(d.Name)) {
			st = "done"
		}
		t.items = append(t.items, &ToolItem{Name: d.Name, Label: d.Label, State: st})
	}
}

func (t *Tools) Ready() bool {
	for _, d := range toolDefs() {
		if !isExecutable(t.Path(d.Name)) {
			return false
		}
	}
	return true
}

func (t *Tools) Running() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.running
}

func (t *Tools) State() map[string]any {
	t.mu.Lock()
	defer t.mu.Unlock()
	items := []ToolItem{}
	var done, total int64
	for _, it := range t.items {
		items = append(items, *it)
		done += it.Done
		total += it.Total
	}
	return map[string]any{"ready": t.Ready(), "running": t.running, "error": t.err, "items": items, "done": done, "total": total}
}

// EnsureLater: 앱을 처음 열었을 때 뒤에서 미리 받아 둬요
func (t *Tools) EnsureLater() {
	time.Sleep(3 * time.Second)
	if !t.Ready() {
		t.Start()
	}
}

func (t *Tools) Start() {
	t.mu.Lock()
	if t.running {
		t.mu.Unlock()
		return
	}
	t.running, t.err = true, ""
	t.resetItems()
	t.mu.Unlock()
	go func() {
		err := t.install(context.Background(), nil)
		t.mu.Lock()
		t.running = false
		if err != nil {
			t.err = err.Error()
			log.Printf("tools: %v", err)
		}
		t.mu.Unlock()
	}()
}

// EnsureNow installs synchronously (도우미 · 빠른 동작에서 처음 쓸 때).
func (t *Tools) EnsureNow(ctx context.Context, prog func(float64)) error {
	if t.Ready() {
		return nil
	}
	t.mu.Lock()
	t.running = true
	t.mu.Unlock()
	err := t.install(ctx, prog)
	t.mu.Lock()
	t.running = false
	if err != nil {
		t.err = err.Error()
	}
	t.mu.Unlock()
	return err
}

func (t *Tools) set(name string, fn func(*ToolItem)) {
	t.mu.Lock()
	for _, it := range t.items {
		if it.Name == name {
			fn(it)
		}
	}
	t.mu.Unlock()
}

// lock: 앱과 도우미가 동시에 받지 않도록 (10분 넘은 잠금은 무시)
func (t *Tools) lock(ctx context.Context) (func(), error) {
	p := filepath.Join(t.support, "tools.lock")
	for i := 0; ; i++ {
		f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			fmt.Fprint(f, os.Getpid())
			f.Close()
			return func() { os.Remove(p) }, nil
		}
		if st, err := os.Stat(p); err == nil && time.Since(st.ModTime()) > 10*time.Minute {
			os.Remove(p)
			continue
		}
		if t.Ready() {
			return func() {}, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
		if i > 900 {
			return nil, errors.New("다른 곳에서 준비 중이에요. 잠시 뒤 다시 해 보세요")
		}
	}
}

func (t *Tools) install(ctx context.Context, prog func(float64)) error {
	unlock, err := t.lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	if t.Ready() {
		t.mu.Lock()
		t.resetItems()
		t.mu.Unlock()
		return nil
	}
	os.MkdirAll(t.bin, 0o755)
	tmp := filepath.Join(t.support, "setup-tmp-"+strconv.Itoa(os.Getpid()))
	os.RemoveAll(tmp)
	os.MkdirAll(tmp, 0o755)
	defer os.RemoveAll(tmp)
	defs := toolDefs()
	for i, d := range defs {
		dest := t.Path(d.Name)
		if isExecutable(dest) {
			t.set(d.Name, func(it *ToolItem) { it.State = "done" })
			continue
		}
		copied := false
		for _, sb := range siblingBins() {
			if old := filepath.Join(sb, exe(d.Name)); isExecutable(old) && d.Kind != "copy" {
				t.set(d.Name, func(it *ToolItem) { it.State = "copy" })
				if copyExecutable(old, dest) == nil && toolWorks(dest) {
					copied = true
					break
				}
				os.Remove(dest)
			}
		}
		if copied {
			t.set(d.Name, func(it *ToolItem) { it.State = "done" })
			continue
		}
		t.set(d.Name, func(it *ToolItem) { it.State = "down"; it.Done, it.Total = 0, 0 })
		var lastErr error
		for _, u := range d.URLs {
			lastErr = t.fetch(ctx, d, u, tmp, dest, func(done, total int64) {
				t.set(d.Name, func(it *ToolItem) { it.Done, it.Total = done, total })
				if prog != nil && total > 0 {
					prog((float64(i) + float64(done)/float64(total)) / float64(len(defs)))
				}
			})
			if lastErr == nil {
				break
			}
			log.Printf("tool %s from %s: %v", d.Name, u, lastErr)
		}
		if lastErr != nil {
			t.set(d.Name, func(it *ToolItem) { it.State = "error" })
			return fmt.Errorf("%s을(를) 받지 못했어요 — %v", d.Label, lastErr)
		}
		t.set(d.Name, func(it *ToolItem) { it.State = "done" })
	}
	return nil
}

func toolWorks(p string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, p, "-version")
	hideWindow(cmd)
	return cmd.Run() == nil
}

func (t *Tools) fetch(ctx context.Context, d toolDef, url, tmp, dest string, progress func(done, total int64)) error {
	if d.Kind == "copy" {
		return copyExecutable(url, dest)
	}
	zipPath := filepath.Join(tmp, "dl-"+sha256Hex(url)[:12]+".zip")
	if !fileExists(zipPath) {
		if err := download(ctx, url, zipPath, progress); err != nil {
			return err
		}
	}
	out := filepath.Join(tmp, d.Name+".out")
	if err := extractZipFile(zipPath, d.Member, out); err != nil {
		os.Remove(zipPath)
		return err
	}
	return finishExecutable(out, dest)
}

func finishExecutable(src, dest string) error {
	os.Chmod(src, 0o755)
	if runtime.GOOS == "darwin" {
		exec.Command("/usr/bin/xattr", "-c", src).Run()
	}
	return os.Rename(src, dest)
}

func copyExecutable(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	part := dest + ".part"
	out, err := os.OpenFile(part, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(part)
		return err
	}
	out.Close()
	return finishExecutable(part, dest)
}

var _ = strings.TrimSpace
