package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// 앱 창 없이 하는 변환 (백그라운드 도우미 · Finder 빠른 동작 · 단축키)

// runCLI: cvtroad-engine --run <request.json> <progress.jsonl>
// 진행 상황을 한 줄씩 JSON으로 progress 파일에 적어요 (도우미가 읽어서 원형 메뉴에 보여 줘요).
func runCLI(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: --run request.json [progress.jsonl]")
		return 2
	}
	b, err := os.ReadFile(args[0])
	if err != nil {
		return 2
	}
	var req Request
	if err := json.Unmarshal(b, &req); err != nil {
		return 2
	}
	os.Remove(args[0])
	var out *os.File
	if len(args) > 1 {
		out, _ = os.OpenFile(args[1], os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	}
	var mu sync.Mutex
	write := func(v any) {
		if out == nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		line, _ := json.Marshal(v)
		out.Write(append(line, '\n'))
	}
	res := runHeadless(req, func(p float64, label string) {
		write(map[string]any{"p": p, "label": label})
	})
	write(map[string]any{"done": true, "result": res})
	if out != nil {
		out.Close()
	}
	if len(res.Outputs) == 0 && res.Text == "" && len(res.Errors) > 0 {
		return 1
	}
	return 0
}

// runHeadless runs a request with cancel-on-signal, notifications and Finder reveal.
func runHeadless(req Request, prog Progress) Result {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sig
		cancel()
	}()
	var last time.Time
	res := Run(ctx, req, func(p float64, label string) {
		if time.Since(last) > 150*time.Millisecond || p >= 1 {
			last = time.Now()
			prog(p, label)
		}
	})
	s := app.Settings()
	if req.Notify && s.Notify {
		switch {
		case res.Text != "":
			notifyOS("QR 코드를 읽었어요", firstLine(res.Text)+" — 복사했어요")
		case len(res.Outputs) > 0 || len(res.Errors) > 0 || len(res.Skipped) > 0:
			notifyOS("CVTROAD", res.Message)
		}
	}
	if s.Reveal && len(res.Outputs) > 0 {
		revealPath(res.Outputs[0])
	}
	logResult(req, res)
	return res
}

func firstLine(s string) string {
	if i := strings.Index(s, "\n"); i >= 0 {
		s = s[:i]
	}
	if len([]rune(s)) > 80 {
		s = string([]rune(s)[:80]) + "…"
	}
	return s
}

// 최근에 한 일 (앱 화면의 "최근 변환"에 보여 줘요)
func logResult(req Request, res Result) {
	p := filepath.Join(app.support, "recent.jsonl")
	if st, err := os.Stat(p); err == nil && st.Size() > 512<<10 {
		os.Remove(p)
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	line, _ := json.Marshal(map[string]any{"at": time.Now().UnixMilli(), "from": req.From, "title": res.Title, "message": res.Message,
		"outputs": res.Outputs, "errors": len(res.Errors)})
	f.Write(append(line, '\n'))
}

// runQuick: Finder 빠른 동작에서 불러요.
//
//	--quick menu  <파일…>   원형 변환 메뉴 띄우기 (도우미가 마우스 자리에)
//	--quick tools <파일…>   원형 도구 메뉴 띄우기
//	--quick to:jpg <파일…>  바로 바꾸기
func runQuick(args []string) int {
	if len(args) < 2 {
		return 2
	}
	action, files := args[0], []string{}
	for _, f := range args[1:] {
		if f = strings.TrimSpace(f); f != "" {
			files = append(files, strings.TrimSuffix(f, "/"))
		}
	}
	if len(files) == 0 {
		return 2
	}
	switch {
	case action == "menu" || action == "tools":
		if err := app.agent.Ask(action, files); err != nil {
			notifyOS("CVTROAD", err.Error())
			return 1
		}
		return 0
	case strings.HasPrefix(action, "to:"):
		to := strings.TrimPrefix(action, "to:")
		k := mainKind(files)
		ok := false
		for _, t := range kindByID(k).Targets {
			if t.ID == to {
				ok = true
			}
		}
		if !ok {
			notifyOS("CVTROAD", kindByID(k).Label+" 파일은 "+strings.ToUpper(to)+"(으)로 바꿀 수 없어요")
			return 1
		}
		req := Request{Op: "convert", To: to, Files: files, Notify: true, From: "quick"}
		res := runHeadless(req, func(float64, string) {})
		if len(res.Outputs) == 0 && len(res.Errors) > 0 {
			return 1
		}
		return 0
	case strings.HasPrefix(action, "tool:"):
		req := Request{Op: "tool", To: strings.TrimPrefix(action, "tool:"), Files: files, Notify: true, From: "quick"}
		runHeadless(req, func(float64, string) {})
		return 0
	}
	return 2
}

// runAgentCmd: --agent on|off|status|sync (설치 · 시험용)
func runAgentCmd(args []string) int {
	cmd := "status"
	if len(args) > 0 {
		cmd = args[0]
	}
	switch cmd {
	case "on", "off":
		app.mu.Lock()
		app.settings.Agent = cmd == "on"
		app.saveSettings()
		app.mu.Unlock()
		app.agent.Sync(app.Settings(), true)
	case "sync":
		app.agent.Sync(app.Settings(), true)
	}
	b, _ := json.MarshalIndent(app.agent.Status(), "", "  ")
	fmt.Println(string(b))
	return 0
}

// runSet: 도우미(메뉴 막대)에서 켜고 끄는 설정
func runSet(args []string) int {
	if len(args) < 2 {
		return 2
	}
	key, val := args[0], args[1]
	allowed := map[string]bool{"shiftDrag": true, "bubble": true, "bubbleSide": true, "agent": true}
	if !allowed[key] {
		return 2
	}
	app.mu.Lock()
	cur, _ := json.Marshal(app.settings)
	m := map[string]any{}
	json.Unmarshal(cur, &m)
	switch val {
	case "true":
		m[key] = true
	case "false":
		m[key] = false
	default:
		m[key] = val
	}
	merged, _ := json.Marshal(m)
	var s Settings
	json.Unmarshal(merged, &s)
	app.settings = fixSettings(s)
	app.saveSettings()
	now := app.settings
	app.mu.Unlock()
	if key == "agent" {
		app.agent.Sync(now, true)
	} else {
		app.agent.writeMenu(now)
	}
	return 0
}
