package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// 백그라운드 도우미 — 앱을 열지 않아도 쓰는 기능
//
//   - Shift를 누른 채 파일을 끌면 커서 주변에 원형 변환 메뉴 (Shift+Option = 도구 메뉴)
//   - 메뉴 막대 아이콘 · 화면에 떠 있는 변환 창(선택)
//   - Finder 빠른 동작(오른쪽 클릭) · 단축키 ⌃⌥C(변환 메뉴) · ⌃⌥T(도구 메뉴)
//
// 도우미는 osascript로 도는 agent.js(JXA)예요. 로그인할 때 launchd가 켜 줘요.
// 실제 변환은 cvtroad-engine --run 이 하고, 진행 상황은 파일로 주고받아요.

//go:embed agent
var agentFiles embed.FS

const (
	agentLabel = "com.nevertheless-d.cvtroad.agent"
	bundleID   = "com.nevertheless-d.cvtroad"
)

type quickAction struct {
	Name   string
	Action string
	Key    string // pbs 단축키 (^ = control, ~ = option, @ = command, $ = shift)
	Label  string // 화면에 보여 줄 단축키
}

var quickActions = []quickAction{
	{"CVTROAD 변환 메뉴", "menu", "^~c", "⌃⌥C"},
	{"CVTROAD 도구 메뉴", "tools", "^~t", "⌃⌥T"},
	{"CVTROAD → JPG로 바꾸기", "to:jpg", "", ""},
	{"CVTROAD → PDF로 바꾸기", "to:pdf", "", ""},
	{"CVTROAD → MP3로 바꾸기", "to:mp3", "", ""},
}

type Agent struct {
	support string
	bin     string
}

func NewAgent(support, bin string) *Agent { return &Agent{support: support, bin: bin} }

func (g *Agent) dir() string      { return filepath.Join(g.bin, "agent") }
func (g *Agent) stateDir() string { return filepath.Join(g.support, "agent") }
func (g *Agent) enginePath() string {
	if p := os.Getenv("CVTROAD_ENGINE_PATH"); p != "" {
		return p
	}
	return filepath.Join(g.bin, exe("cvtroad-engine"))
}

func launchAgentsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents")
}

func servicesDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Services")
}

func (g *Agent) plistPath() string { return filepath.Join(launchAgentsDir(), agentLabel+".plist") }

func isMac() bool { return runtime.GOOS == "darwin" && os.Getenv("CVTROAD_FAKE_LAUNCHD") == "" }

// writeFiles copies agent.js · ring.html · 아이콘 into Support/bin/agent. Returns true when something changed.
func (g *Agent) writeFiles() bool {
	changed := false
	os.MkdirAll(g.dir(), 0o755)
	fs.WalkDir(agentFiles, "agent", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(p, "agent/")
		dst := filepath.Join(g.dir(), filepath.FromSlash(rel))
		b, _ := agentFiles.ReadFile(p)
		if old, err := os.ReadFile(dst); err == nil && bytes.Equal(old, b) {
			return nil
		}
		os.MkdirAll(filepath.Dir(dst), 0o755)
		writeFileAtomic(dst, b, 0o644)
		changed = true
		return nil
	})
	// 원형 메뉴 글꼴 (앱 화면과 같은 글꼴)
	for _, f := range []string{"Overpass-latin.woff2"} {
		b, err := webFiles.ReadFile("web/fonts/" + f)
		if err != nil {
			continue
		}
		dst := filepath.Join(g.dir(), f)
		if old, err := os.ReadFile(dst); err != nil || !bytes.Equal(old, b) {
			writeFileAtomic(dst, b, 0o644)
		}
	}
	return changed
}

// writeMenu: 도우미가 읽는 설정 · 형식 표
func (g *Agent) writeMenu(s Settings) {
	os.MkdirAll(g.stateDir(), 0o755)
	m := map[string]any{
		"build":      build,
		"engine":     g.enginePath(),
		"support":    g.support,
		"bundleID":   bundleID,
		"shiftDrag":  s.ShiftDrag,
		"bubble":     s.Bubble,
		"bubbleSide": s.BubbleSide,
		"appearance": s.Appearance,
		"kinds":      kinds,
		"exts":       extKind,
		"shortcuts":  map[string]string{"menu": quickActions[0].Label, "tools": quickActions[1].Label},
	}
	b, _ := json.MarshalIndent(m, "", " ")
	p := filepath.Join(g.stateDir(), "menu.json")
	if old, err := os.ReadFile(p); err == nil && bytes.Equal(old, b) {
		return
	}
	writeFileAtomic(p, b, 0o644)
}

func (g *Agent) plist() string {
	esc := func(s string) string { return html.EscapeString(s) }
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + agentLabel + `</string>
	<key>ProgramArguments</key>
	<array>
		<string>/usr/bin/osascript</string>
		<string>-l</string>
		<string>JavaScript</string>
		<string>` + esc(filepath.Join(g.dir(), "agent.js")) + `</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<dict>
		<key>SuccessfulExit</key>
		<false/>
	</dict>
	<key>ThrottleInterval</key>
	<integer>15</integer>
	<key>ProcessType</key>
	<string>Interactive</string>
	<key>LimitLoadToSessionType</key>
	<string>Aqua</string>
	<key>AssociatedBundleIdentifiers</key>
	<array>
		<string>` + bundleID + `</string>
	</array>
	<key>EnvironmentVariables</key>
	<dict>
		<key>CVTROAD_SUPPORT</key>
		<string>` + esc(g.support) + `</string>
	</dict>
	<key>StandardOutPath</key>
	<string>` + esc(filepath.Join(g.support, "agent.log")) + `</string>
	<key>StandardErrorPath</key>
	<string>` + esc(filepath.Join(g.support, "agent.log")) + `</string>
</dict>
</plist>
`
}

func uid() string { return strconv.Itoa(os.Getuid()) }

func launchctl(args ...string) (string, error) {
	if !isMac() {
		return "", errors.New("not mac")
	}
	out, err := exec.Command("/bin/launchctl", args...).CombinedOutput()
	return string(out), err
}

func (g *Agent) loaded() bool {
	out, err := launchctl("print", "gui/"+uid()+"/"+agentLabel)
	return err == nil && strings.Contains(out, agentLabel)
}

func (g *Agent) running() bool {
	out, err := launchctl("print", "gui/"+uid()+"/"+agentLabel)
	return err == nil && strings.Contains(out, "state = running")
}

// Sync makes the installed helper match the settings. force: 다시 시작까지.
func (g *Agent) Sync(s Settings, force bool) {
	changed := g.writeFiles()
	g.writeMenu(s)
	if !isMac() {
		return
	}
	if !s.Agent {
		g.uninstall()
		return
	}
	os.MkdirAll(launchAgentsDir(), 0o755)
	want := g.plist()
	old, _ := os.ReadFile(g.plistPath())
	plistChanged := string(old) != want
	if plistChanged {
		writeFileAtomic(g.plistPath(), []byte(want), 0o644)
	}
	g.installServices(false)
	switch {
	case plistChanged && g.loaded():
		launchctl("bootout", "gui/"+uid()+"/"+agentLabel)
		time.Sleep(500 * time.Millisecond)
		g.bootstrap()
	case !g.loaded():
		g.bootstrap()
	case changed || force || !g.running():
		launchctl("kickstart", "-k", "gui/"+uid()+"/"+agentLabel)
	}
}

func (g *Agent) bootstrap() {
	out, err := launchctl("bootstrap", "gui/"+uid(), g.plistPath())
	if err != nil && !strings.Contains(out, "already") {
		// 예전 방식으로 한 번 더
		if out2, err2 := launchctl("load", "-w", g.plistPath()); err2 != nil {
			log.Printf("agent bootstrap: %v %s / %s", err, strings.TrimSpace(out), strings.TrimSpace(out2))
		}
	}
	launchctl("enable", "gui/"+uid()+"/"+agentLabel)
}

func (g *Agent) uninstall() {
	if g.loaded() {
		launchctl("bootout", "gui/"+uid()+"/"+agentLabel)
	}
	os.Remove(g.plistPath())
	g.removeServices()
}

// Ask asks the running helper to show a ring menu for these files (빠른 동작 · 단축키).
func (g *Agent) Ask(mode string, files []string) error {
	inbox := filepath.Join(g.stateDir(), "inbox")
	os.MkdirAll(inbox, 0o755)
	b, _ := json.Marshal(map[string]any{"mode": mode, "files": files, "at": time.Now().UnixMilli()})
	writeFileAtomic(filepath.Join(inbox, fmt.Sprintf("%d-%s.json", time.Now().UnixNano(), randID(2))), b, 0o644)
	if !isMac() {
		return nil
	}
	if g.running() {
		return nil
	}
	if !app.Settings().Agent {
		return errors.New("백그라운드 도우미가 꺼져 있어요. CVTROAD 앱의 ‘도우미’에서 켜 주세요")
	}
	g.writeFiles()
	g.writeMenu(app.Settings())
	if !g.loaded() {
		g.bootstrap()
	} else {
		launchctl("kickstart", "gui/"+uid()+"/"+agentLabel)
	}
	return nil
}

func (g *Agent) Status() map[string]any {
	s := app.Settings()
	st := map[string]any{
		"supported": uiOS() == "darwin",
		"enabled":   s.Agent,
		"shiftDrag": s.ShiftDrag,
		"bubble":    s.Bubble,
		"installed": fileExists(g.plistPath()),
		"running":   g.running(),
		"services":  g.servicesInstalled(),
	}
	sc := []map[string]string{}
	for _, q := range quickActions {
		sc = append(sc, map[string]string{"name": q.Name, "key": q.Label, "action": q.Action})
	}
	st["quickActions"] = sc
	if b, err := os.ReadFile(filepath.Join(g.stateDir(), "alive")); err == nil {
		if t, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64); err == nil {
			st["alive"] = time.Now().UnixMilli()-t < 15000
		}
	}
	return st
}

func (a *App) handleAgent(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Action string `json:"action"` // on | off | restart | services | demo
	}
	readJSON(r, &in)
	switch in.Action {
	case "on", "off":
		a.mu.Lock()
		a.settings.Agent = in.Action == "on"
		a.saveSettings()
		a.mu.Unlock()
		a.agent.Sync(a.Settings(), true)
	case "restart":
		a.agent.Sync(a.Settings(), true)
	case "services":
		a.agent.installServices(true)
	case "demo":
		// 도우미가 화면 가운데에 원형 메뉴를 띄워 보여 줘요 (사용법 확인용)
		a.agent.Ask("demo", []string{})
	}
	time.Sleep(400 * time.Millisecond)
	writeJSON(w, map[string]any{"ok": true, "agent": a.agent.Status()})
}

// ---------------------------------------------------------------- Finder 빠른 동작 (~/Library/Services)

func (g *Agent) serviceStamp() string { return filepath.Join(g.stateDir(), "services-version") }

const servicesVersion = "1"

func (g *Agent) servicesInstalled() bool {
	for _, q := range quickActions {
		if !fileExists(filepath.Join(servicesDir(), q.Name+".workflow", "Contents", "document.wflow")) {
			return false
		}
	}
	return true
}

func (g *Agent) installServices(force bool) {
	stamp, _ := os.ReadFile(g.serviceStamp())
	enginePathNow := g.enginePath()
	want := servicesVersion + "|" + enginePathNow
	if !force && string(stamp) == want && g.servicesInstalled() {
		return
	}
	for _, q := range quickActions {
		dir := filepath.Join(servicesDir(), q.Name+".workflow", "Contents")
		os.MkdirAll(dir, 0o755)
		writeFileAtomic(filepath.Join(dir, "Info.plist"), []byte(serviceInfoPlist(q.Name)), 0o644)
		writeFileAtomic(filepath.Join(dir, "document.wflow"), []byte(serviceWorkflow(enginePathNow, q.Action)), 0o644)
	}
	writeFileAtomic(g.serviceStamp(), []byte(want), 0o644)
	if isMac() {
		for _, q := range quickActions {
			if q.Key == "" {
				continue
			}
			key := `"(null) - ` + q.Name + ` - runWorkflowAsService"`
			val := `{ "enabled_context_menu" = 1; "enabled_services_menu" = 1; "key_equivalent" = "` + q.Key + `"; "presentation_modes" = { ContextMenu = 1; ServicesMenu = 1; }; }`
			if out, err := exec.Command("/usr/bin/defaults", "write", "pbs", "NSServicesStatus", "-dict-add", key, val).CombinedOutput(); err != nil {
				log.Printf("services shortcut %s: %v %s", q.Name, err, out)
			}
		}
		exec.Command("/System/Library/CoreServices/pbs", "-flush").Run()
		exec.Command("/System/Library/CoreServices/pbs", "-update").Run()
	}
}

func (g *Agent) removeServices() {
	for _, q := range quickActions {
		os.RemoveAll(filepath.Join(servicesDir(), q.Name+".workflow"))
	}
	os.Remove(g.serviceStamp())
	if isMac() {
		exec.Command("/System/Library/CoreServices/pbs", "-flush").Run()
	}
}

func serviceInfoPlist(name string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>NSServices</key>
	<array>
		<dict>
			<key>NSBackgroundColorName</key>
			<string>background</string>
			<key>NSIconName</key>
			<string>NSActionTemplate</string>
			<key>NSMenuItem</key>
			<dict>
				<key>default</key>
				<string>` + html.EscapeString(name) + `</string>
			</dict>
			<key>NSMessage</key>
			<string>runWorkflowAsService</string>
			<key>NSRequiredContext</key>
			<dict>
				<key>NSApplicationIdentifier</key>
				<string>com.apple.finder</string>
			</dict>
			<key>NSSendFileTypes</key>
			<array>
				<string>public.item</string>
			</array>
		</dict>
	</array>
</dict>
</plist>
`
}

func serviceWorkflow(engine, action string) string {
	script := "# CVTROAD — made by. Nevertheless_D\nENGINE=" + shq(engine) + "\n" +
		"if [ -x \"$ENGINE\" ]; then\n  exec \"$ENGINE\" --quick " + shq(action) + " \"$@\"\nelse\n" +
		"  osascript -e 'display notification \"CVTROAD 앱을 한 번 열어 주세요\" with title \"CVTROAD\"'\nfi\n"
	esc := html.EscapeString(script)
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>AMApplicationBuild</key>
	<string>523</string>
	<key>AMApplicationVersion</key>
	<string>2.10</string>
	<key>AMDocumentVersion</key>
	<string>2</string>
	<key>actions</key>
	<array>
		<dict>
			<key>action</key>
			<dict>
				<key>AMAccepts</key>
				<dict>
					<key>Container</key>
					<string>List</string>
					<key>Optional</key>
					<true/>
					<key>Types</key>
					<array>
						<string>com.apple.cocoa.string</string>
					</array>
				</dict>
				<key>AMActionVersion</key>
				<string>2.0.3</string>
				<key>AMApplication</key>
				<array>
					<string>Automator</string>
				</array>
				<key>AMParameterProperties</key>
				<dict>
					<key>COMMAND_STRING</key>
					<dict/>
					<key>CheckedForUserDefaultShell</key>
					<dict/>
					<key>inputMethod</key>
					<dict/>
					<key>shell</key>
					<dict/>
					<key>source</key>
					<dict/>
				</dict>
				<key>AMProvides</key>
				<dict>
					<key>Container</key>
					<string>List</string>
					<key>Types</key>
					<array>
						<string>com.apple.cocoa.string</string>
					</array>
				</dict>
				<key>ActionBundlePath</key>
				<string>/System/Library/Automator/Run Shell Script.action</string>
				<key>ActionName</key>
				<string>Run Shell Script</string>
				<key>ActionParameters</key>
				<dict>
					<key>COMMAND_STRING</key>
					<string>` + esc + `</string>
					<key>CheckedForUserDefaultShell</key>
					<true/>
					<key>inputMethod</key>
					<integer>1</integer>
					<key>shell</key>
					<string>/bin/bash</string>
					<key>source</key>
					<string></string>
				</dict>
				<key>BundleIdentifier</key>
				<string>com.apple.RunShellScript</string>
				<key>CFBundleVersion</key>
				<string>2.0.3</string>
				<key>CanShowSelectedItemsWhenRun</key>
				<false/>
				<key>CanShowWhenRun</key>
				<true/>
				<key>Category</key>
				<array>
					<string>AMCategoryUtilities</string>
				</array>
				<key>Class Name</key>
				<string>RunShellScriptAction</string>
				<key>InputUUID</key>
				<string>6B1C2E1A-3C8D-4A3E-9E0B-2F6E1C0A7D11</string>
				<key>Keywords</key>
				<array>
					<string>Shell</string>
					<string>Script</string>
				</array>
				<key>OutputUUID</key>
				<string>0E7B4C3D-1F2A-4B5C-8D9E-3A4B5C6D7E22</string>
				<key>UUID</key>
				<string>A1B2C3D4-E5F6-4711-8899-AABBCCDDEE33</string>
				<key>UnlocalizedApplications</key>
				<array>
					<string>Automator</string>
				</array>
				<key>arguments</key>
				<dict>
					<key>0</key>
					<dict>
						<key>default value</key>
						<integer>0</integer>
						<key>name</key>
						<string>inputMethod</string>
						<key>required</key>
						<string>0</string>
						<key>type</key>
						<string>0</string>
						<key>uuid</key>
						<string>0</string>
					</dict>
					<key>1</key>
					<dict>
						<key>default value</key>
						<string></string>
						<key>name</key>
						<string>source</string>
						<key>required</key>
						<string>0</string>
						<key>type</key>
						<string>0</string>
						<key>uuid</key>
						<string>1</string>
					</dict>
					<key>2</key>
					<dict>
						<key>default value</key>
						<false/>
						<key>name</key>
						<string>CheckedForUserDefaultShell</string>
						<key>required</key>
						<string>0</string>
						<key>type</key>
						<string>0</string>
						<key>uuid</key>
						<string>2</string>
					</dict>
					<key>3</key>
					<dict>
						<key>default value</key>
						<string></string>
						<key>name</key>
						<string>COMMAND_STRING</string>
						<key>required</key>
						<string>0</string>
						<key>type</key>
						<string>0</string>
						<key>uuid</key>
						<string>3</string>
					</dict>
					<key>4</key>
					<dict>
						<key>default value</key>
						<string>/bin/sh</string>
						<key>name</key>
						<string>shell</string>
						<key>required</key>
						<string>0</string>
						<key>type</key>
						<string>0</string>
						<key>uuid</key>
						<string>4</string>
					</dict>
				</dict>
				<key>conversionLabel</key>
				<integer>0</integer>
				<key>isViewVisible</key>
				<true/>
				<key>location</key>
				<string>309.000000:253.000000</string>
				<key>nibPath</key>
				<string>/System/Library/Automator/Run Shell Script.action/Contents/Resources/Base.lproj/main.nib</string>
			</dict>
			<key>isViewVisible</key>
			<true/>
		</dict>
	</array>
	<key>connectors</key>
	<dict/>
	<key>workflowMetaData</key>
	<dict>
		<key>applicationBundleID</key>
		<string>com.apple.finder</string>
		<key>applicationBundleIDsByPath</key>
		<dict>
			<key>/System/Library/CoreServices/Finder.app</key>
			<string>com.apple.finder</string>
		</dict>
		<key>applicationPath</key>
		<string>/System/Library/CoreServices/Finder.app</string>
		<key>applicationPaths</key>
		<array>
			<string>/System/Library/CoreServices/Finder.app</string>
		</array>
		<key>inputTypeIdentifier</key>
		<string>com.apple.Automator.fileSystemObject</string>
		<key>outputTypeIdentifier</key>
		<string>com.apple.Automator.nothing</string>
		<key>presentationMode</key>
		<integer>15</integer>
		<key>processesInput</key>
		<integer>0</integer>
		<key>serviceApplicationBundleID</key>
		<string>com.apple.finder</string>
		<key>serviceApplicationPath</key>
		<string>/System/Library/CoreServices/Finder.app</string>
		<key>serviceInputTypeIdentifier</key>
		<string>com.apple.Automator.fileSystemObject</string>
		<key>serviceOutputTypeIdentifier</key>
		<string>com.apple.Automator.nothing</string>
		<key>serviceProcessesInput</key>
		<integer>0</integer>
		<key>systemImageName</key>
		<string>NSActionTemplate</string>
		<key>useAutomaticInputType</key>
		<integer>0</integer>
		<key>workflowTypeIdentifier</key>
		<string>com.apple.Automator.servicesMenu</string>
	</dict>
</dict>
</plist>
`
}
