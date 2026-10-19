package main

// 진단 — 엔진이 갑자기 멈췄을 때 이유를 남겨요.
//
//   - Go가 멈추면서 남기는 오류(크래시)를 Support/crash.txt 에 따로 적어요 (debug.SetCrashOutput)
//   - 종료 신호(TERM·INT·HUP)를 받으면 engine.log 에 적어요. HUP은 무시하고 계속 돌아요.
//   - 켜질 때 Support/running 이 남아 있고 그 프로세스가 없으면 → 지난번에 갑자기 멈춘 것.
//     crash.txt · engine.out · engine.log 끝부분을 모아 기록 폴더(맥: ~/Library/Logs/CVTROAD)의
//     last-crash.txt 로 저장하고, 앱 화면에 알려요.
//   - 백그라운드 작업(goroutine)에서 난 오류는 guard()로 잡아서 엔진 전체가 꺼지지 않게 해요.

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"
)

type crashInfo struct {
	At      int64  `json:"at"`
	Summary string `json:"summary"`
	Saved   string `json:"saved,omitempty"` // 진단 파일 위치
}

var (
	diagMu    sync.Mutex
	lastCrash *crashInfo
)

func (a *App) runningPath() string { return filepath.Join(a.support, "running") }

// startDiag: runEngine 맨 앞에서 불러요.
func (a *App) startDiag() {
	a.checkPreviousRun()
	writeFileAtomic(a.runningPath(), []byte(fmt.Sprintf("%d %d %s", os.Getpid(), time.Now().Unix(), build)), 0o644)

	if f, err := os.OpenFile(filepath.Join(a.logDir, "crash.txt"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644); err == nil {
		fmt.Fprintf(f, "CVTROAD %s · pid %d · %s 시작\n", build, os.Getpid(), time.Now().Format("2006-01-02 15:04:05"))
		if err := debug.SetCrashOutput(f, debug.CrashOptions{}); err != nil {
			log.Printf("diag: crash output: %v", err)
		}
		f.Close() // SetCrashOutput이 따로 복사해 둬요
	}

	ch := make(chan os.Signal, 4)
	signal.Notify(ch, diagSignals()...)
	go func() {
		for s := range ch {
			log.Printf("diag: 신호를 받았어요: %v (보낸 쪽 pid를 알 수 없어요)", s)
			if isHangup(s) {
				continue // 창이나 실행기가 닫혀도 계속 돌아요
			}
			a.shutdown()
		}
	}()
}

// clearRunning: 정상 종료할 때 (shutdown)
func (a *App) clearRunning() {
	if b, err := os.ReadFile(a.runningPath()); err == nil && strings.HasPrefix(string(b), strconv.Itoa(os.Getpid())+" ") {
		os.Remove(a.runningPath())
	}
}

func (a *App) checkPreviousRun() {
	b, err := os.ReadFile(a.runningPath())
	if err != nil {
		return
	}
	f := strings.Fields(string(b))
	if len(f) < 2 {
		return
	}
	pid, _ := strconv.Atoi(f[0])
	started, _ := strconv.ParseInt(f[1], 10, 64)
	prevBuild := ""
	if len(f) > 2 {
		prevBuild = f[2]
	}
	if pid > 0 && pid != os.Getpid() && processAlive(pid) {
		return // 아직 살아 있어요 (다른 엔진이 돌고 있음)
	}

	crash := strings.TrimSpace(readTail(filepath.Join(a.logDir, "crash.txt"), 24<<10))
	out := strings.TrimSpace(readTail(filepath.Join(a.logDir, "engine.out.prev"), 16<<10))
	lg := strings.TrimSpace(readTail(filepath.Join(a.logDir, "engine.log"), 12<<10))

	summary := "엔진이 갑자기 멈췄어요 (남은 오류 기록 없음 — 시스템이 강제로 끝냈을 수 있어요)"
	if i := strings.Index(crash, "\n"); i >= 0 && strings.TrimSpace(crash[i:]) != "" {
		first := strings.TrimSpace(crash[i:])
		if j := strings.Index(first, "\n"); j >= 0 {
			first = first[:j]
		}
		summary = "엔진 오류: " + first
	} else if strings.Contains(lg, "diag: 신호를 받았어요") {
		summary = "엔진이 종료 신호를 받고 꺼졌어요"
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "CVTROAD 진단 정보\n만든 때: %s\n지금 버전: %s · 멈춘 버전: %s\n", time.Now().Format("2006-01-02 15:04:05"), build, prevBuild)
	if started > 0 {
		fmt.Fprintf(&sb, "멈춘 엔진이 켜진 때: %s (pid %d)\n", time.Unix(started, 0).Format("2006-01-02 15:04:05"), pid)
	}
	fmt.Fprintf(&sb, "요약: %s\n\n", summary)
	fmt.Fprintf(&sb, "── crash.txt ──\n%s\n\n── engine.out (지난번) ──\n%s\n\n── engine.log (끝부분) ──\n%s\n", crash, out, lg)
	report := sb.String()
	writeFileAtomic(filepath.Join(a.logDir, "last-crash.txt"), []byte(report), 0o644)

	ci := &crashInfo{At: time.Now().UnixMilli(), Summary: summary, Saved: filepath.Join(a.logDir, "last-crash.txt")}
	log.Printf("diag: 지난번 엔진이 갑자기 멈췄어요 — %s", summary)
	diagMu.Lock()
	lastCrash = ci
	diagMu.Unlock()
}

func lastCrashInfo() *crashInfo {
	diagMu.Lock()
	defer diagMu.Unlock()
	return lastCrash
}

// guard: go 루틴 맨 앞에 defer guard("이름") — 오류가 나도 엔진은 계속 돌아요.
func guard(name string) {
	if r := recover(); r != nil {
		log.Printf("diag: %s 에서 오류가 났지만 계속 돌아요: %v\n%s", name, r, debug.Stack())
	}
}

func readTail(p string, max int64) string {
	f, err := os.Open(p)
	if err != nil {
		return ""
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return ""
	}
	off := st.Size() - max
	if off < 0 {
		off = 0
	}
	buf := make([]byte, st.Size()-off)
	n, _ := f.ReadAt(buf, off)
	return string(buf[:n])
}
