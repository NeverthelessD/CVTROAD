package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// helper.js 를 osascript로 실행해요 (맥 전용: PDFKit · Vision · Core Image · 인쇄).
// 시험할 때는 CVTROAD_HELPER_CMD 로 다른 프로그램을 쓸 수 있어요.

type helperLine struct {
	OK        bool   `json:"ok"`
	Text      string `json:"text"`
	Pages     int    `json:"pages"`
	Page      int    `json:"page"`
	Tick      int    `json:"tick"`
	Encrypted bool   `json:"encrypted"`
	Error     string `json:"error"`
	Out       string `json:"out"`
}

func helperAvailable() bool {
	return os.Getenv("CVTROAD_HELPER_CMD") != "" || runtime.GOOS == "darwin"
}

func helperScript() string {
	p := filepath.Join(app.binDir, "helper.js")
	if !fileExists(p) || fileSize(p) != int64(len(helperJS)) {
		writeFileAtomic(p, helperJS, 0o644)
	}
	return p
}

func helperCommand(ctx context.Context, args ...string) *exec.Cmd {
	if c := os.Getenv("CVTROAD_HELPER_CMD"); c != "" {
		parts := strings.Fields(c)
		return exec.CommandContext(ctx, parts[0], append(parts[1:], args...)...)
	}
	return exec.CommandContext(ctx, "/usr/bin/osascript", append([]string{"-l", "JavaScript", helperScript()}, args...)...)
}

// runHelper runs one helper mode with JSON arguments. idle: stop when it prints nothing for this long.
func runHelper(mode string, args any, idle time.Duration, onLine func(helperLine)) error {
	if !helperAvailable() {
		return errors.New("helper not available")
	}
	tmp := filepath.Join(app.support, "tmp")
	os.MkdirAll(tmp, 0o755)
	argFile := filepath.Join(tmp, "args-"+randID(4)+".json")
	b, _ := json.Marshal(args)
	if err := os.WriteFile(argFile, b, 0o600); err != nil {
		return err
	}
	defer os.Remove(argFile)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	cmd := helperCommand(ctx, mode, argFile)
	hideWindow(cmd)
	setProcessGroup(cmd)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var errBuf strings.Builder
	cmd.Stderr = &limitedWriter{w: &errBuf, n: 4000}
	if err := cmd.Start(); err != nil {
		return err
	}
	var mu sync.Mutex
	lastAt := time.Now()
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				mu.Lock()
				quiet := time.Since(lastAt)
				mu.Unlock()
				if quiet > idle {
					killProcessGroup(cmd)
					stop()
					return
				}
			}
		}
	}()
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 1<<20), 32<<20)
	for sc.Scan() {
		mu.Lock()
		lastAt = time.Now()
		mu.Unlock()
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var hl helperLine
		if json.Unmarshal([]byte(line), &hl) == nil {
			onLine(hl)
		}
	}
	err = cmd.Wait()
	close(done)
	if err != nil {
		if s := strings.TrimSpace(errBuf.String()); s != "" {
			log.Printf("helper %s: %v: %s", mode, err, lastLine(s))
		}
		return err
	}
	return nil
}

// helperSimple: 결과 하나만 기다리는 일 (실패하면 helper가 알려 준 이유를 그대로)
func helperSimple(mode string, args any, idle time.Duration) error {
	var herr string
	ok := false
	err := runHelper(mode, args, idle, func(h helperLine) {
		if h.Error != "" {
			herr = h.Error
		}
		if h.OK {
			ok = true
		}
	})
	if herr != "" {
		return uerr("%s", herr)
	}
	if err != nil || !ok {
		return uerr("바꾸지 못했어요")
	}
	return nil
}

type limitedWriter struct {
	w interface{ WriteString(string) (int, error) }
	n int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.n > 0 {
		s := string(p)
		if len(s) > l.n {
			s = s[:l.n]
		}
		l.n -= len(s)
		l.w.WriteString(s)
	}
	return len(p), nil
}
