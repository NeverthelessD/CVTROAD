package main

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// 앱 창에서 시작한 변환 작업 대기열 (동시에 2개까지)

type Job struct {
	mu       sync.Mutex
	ID       string
	Req      Request
	Title    string
	State    string // wait | run | done | error | cancel
	Progress float64
	Label    string
	Started  int64
	Finished int64
	Created  int64
	Result   *Result
	cancel   context.CancelFunc
	eta      float64
}

type JobSnap struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Op       string  `json:"op"`
	To       string  `json:"to"`
	Kind     string  `json:"kind"`
	Count    int     `json:"count"`
	State    string  `json:"state"`
	Progress float64 `json:"progress"`
	Label    string  `json:"label"`
	ETA      float64 `json:"eta"`
	Created  int64   `json:"created"`
	Finished int64   `json:"finished"`
	Result   *Result `json:"result,omitempty"`
}

type JobManager struct {
	mu    sync.Mutex
	jobs  []*Job
	limit int
}

func NewJobManager() *JobManager { return &JobManager{limit: 2} }

func (m *JobManager) Add(req Request) *Job {
	j := &Job{ID: randID(5), Req: req, Title: req.Title(), State: "wait", Created: time.Now().UnixMilli()}
	m.mu.Lock()
	m.jobs = append(m.jobs, j)
	m.mu.Unlock()
	m.pump()
	return j
}

func (m *JobManager) pump() {
	m.mu.Lock()
	defer m.mu.Unlock()
	running := 0
	for _, j := range m.jobs {
		if j.State == "run" {
			running++
		}
	}
	for _, j := range m.jobs {
		if running >= m.limit {
			break
		}
		if j.State == "wait" {
			running++
			j.State = "run"
			j.Started = time.Now().UnixMilli()
			go m.run(j)
		}
	}
}

func (m *JobManager) run(j *Job) {
	ctx, cancel := context.WithCancel(context.Background())
	j.mu.Lock()
	j.cancel = cancel
	j.mu.Unlock()
	start := time.Now()
	res := Run(ctx, j.Req, func(p float64, label string) {
		j.mu.Lock()
		j.Progress, j.Label = p, label
		if p > 0.03 {
			el := time.Since(start).Seconds()
			est := el/p - el
			if j.eta == 0 {
				j.eta = est
			} else {
				j.eta = j.eta*0.8 + est*0.2
			}
		}
		j.mu.Unlock()
	})
	cancel()
	j.mu.Lock()
	j.Result = &res
	j.Finished = time.Now().UnixMilli()
	j.Progress = 1
	switch {
	case ctx.Err() != nil && j.State == "cancel":
	case len(res.Outputs) == 0 && res.Text == "" && len(res.Errors) > 0:
		j.State = "error"
	default:
		j.State = "done"
	}
	state := j.State
	j.mu.Unlock()
	if state == "done" {
		s := app.Settings()
		long := j.Finished-j.Started > 15000 // 오래 걸린 작업만 알려요 (창을 보고 있을 때는 화면에 나와요)
		if j.Req.Notify || (s.Notify && long) {
			if len(res.Outputs) > 0 || res.Text != "" {
				notify("CVTROAD", res.Message)
			}
		}
		if s.Reveal && len(res.Outputs) > 0 {
			revealPath(res.Outputs[0])
		}
	}
	m.pump()
}

func (m *JobManager) Cancel(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range m.jobs {
		if j.ID == id {
			j.mu.Lock()
			if j.State == "wait" {
				j.State = "cancel"
				j.Finished = time.Now().UnixMilli()
			} else if j.State == "run" {
				j.State = "cancel"
				if j.cancel != nil {
					j.cancel()
				}
			}
			j.mu.Unlock()
		}
	}
}

func (m *JobManager) CancelAll() {
	m.mu.Lock()
	ids := []string{}
	for _, j := range m.jobs {
		ids = append(ids, j.ID)
	}
	m.mu.Unlock()
	for _, id := range ids {
		m.Cancel(id)
	}
}

func (m *JobManager) ClearFinished() {
	m.mu.Lock()
	defer m.mu.Unlock()
	keep := []*Job{}
	for _, j := range m.jobs {
		j.mu.Lock()
		if j.State == "wait" || j.State == "run" {
			keep = append(keep, j)
		}
		j.mu.Unlock()
	}
	m.jobs = keep
}

func (m *JobManager) Busy() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range m.jobs {
		if j.State == "wait" || j.State == "run" {
			return true
		}
	}
	return false
}

func (m *JobManager) Snapshot() []JobSnap {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []JobSnap{}
	for _, j := range m.jobs {
		j.mu.Lock()
		out = append(out, JobSnap{ID: j.ID, Title: j.Title, Op: j.Req.Op, To: j.Req.To, Kind: mainKind(j.Req.Files), Count: len(j.Req.Files),
			State: j.State, Progress: j.Progress, Label: j.Label, ETA: j.eta, Created: j.Created, Finished: j.Finished, Result: j.Result})
		j.mu.Unlock()
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Created > out[b].Created })
	if len(out) > 60 {
		out = out[:60]
	}
	return out
}

func (a *App) handleJobStart(w http.ResponseWriter, r *http.Request) {
	var req Request
	if err := readJSON(r, &req); err != nil {
		jsonError(w, 400, "요청을 읽지 못했어요")
		return
	}
	if len(req.Files) == 0 {
		jsonError(w, 400, "바꿀 파일을 골라 주세요")
		return
	}
	for _, f := range req.Files {
		if !a.isAllowed(f) {
			jsonError(w, 403, "고르지 않은 파일이 들어 있어요")
			return
		}
	}
	if req.OutDir != "" {
		if st, err := os.Stat(req.OutDir); err != nil || !st.IsDir() {
			jsonError(w, 400, "저장할 폴더를 찾을 수 없어요")
			return
		}
		req.OutDir = filepath.Clean(req.OutDir)
	}
	req.From = "app"
	j := a.jobs.Add(req)
	writeJSON(w, map[string]any{"ok": true, "id": j.ID})
}
