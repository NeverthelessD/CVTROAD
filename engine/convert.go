package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// 변환 요청 하나 = 같은 일을 여러 파일에 (또는 여러 파일을 하나로)
type Request struct {
	Op     string     `json:"op"` // convert | tool
	To     string     `json:"to"` // 바꿀 형식 id 또는 도구 id
	Files  []string   `json:"files"`
	OutDir string     `json:"outDir,omitempty"` // 비우면 설정대로 (원본 옆 / 기본 폴더)
	Pages  []PageEdit `json:"pages,omitempty"`  // PDF 페이지 정리
	Opts   *Opts      `json:"opts,omitempty"`
	Notify bool       `json:"notify,omitempty"`
	From   string     `json:"from,omitempty"` // app | ring | quick (기록용)
}

type Opts struct {
	ImageQuality int    `json:"imageQuality,omitempty"`
	AudioQuality string `json:"audioQuality,omitempty"`
	VideoQuality string `json:"videoQuality,omitempty"`
	VideoSize    string `json:"videoSize,omitempty"`
	PDFDpi       int    `json:"pdfDpi,omitempty"`
}

type PageEdit struct {
	Page   int `json:"page"`   // 원래 쪽 번호 (1부터)
	Rotate int `json:"rotate"` // 0 · 90 · 180 · 270
}

type Result struct {
	Outputs []string `json:"outputs"`
	Errors  []string `json:"errors"`
	Skipped []string `json:"skipped"`
	Notes   []string `json:"notes"`
	Text    string   `json:"text,omitempty"` // QR 코드 내용
	Title   string   `json:"title"`
	Message string   `json:"message"`
}

// Progress: 전체 진행률(0~1)과 지금 하는 일
type Progress func(p float64, label string)

// 사용자에게 보여 줄 오류 (그대로 보여 줘요)
type userErr struct{ msg string }

func (e userErr) Error() string { return e.msg }
func uerr(f string, a ...any) error {
	return userErr{fmt.Sprintf(f, a...)}
}

// "건너뜀"으로 표시할 오류 (실패가 아니라 할 일이 없을 때)
type skipErr struct{ msg string }

func (e skipErr) Error() string { return e.msg }

func (r Request) opts() Opts {
	s := app.Settings()
	o := Opts{ImageQuality: s.ImageQuality, AudioQuality: s.AudioQuality, VideoQuality: s.VideoQuality, VideoSize: s.VideoSize, PDFDpi: s.PDFDpi}
	if r.Opts != nil {
		if r.Opts.ImageQuality >= 40 && r.Opts.ImageQuality <= 100 {
			o.ImageQuality = r.Opts.ImageQuality
		}
		if r.Opts.AudioQuality != "" {
			o.AudioQuality = r.Opts.AudioQuality
		}
		if r.Opts.VideoQuality != "" {
			o.VideoQuality = r.Opts.VideoQuality
		}
		if r.Opts.VideoSize != "" {
			o.VideoSize = r.Opts.VideoSize
		}
		if r.Opts.PDFDpi >= 72 && r.Opts.PDFDpi <= 600 {
			o.PDFDpi = r.Opts.PDFDpi
		}
	}
	return fixOpts(o)
}

func fixOpts(o Opts) Opts {
	s := fixSettings(Settings{ImageQuality: o.ImageQuality, AudioQuality: o.AudioQuality, VideoQuality: o.VideoQuality, VideoSize: o.VideoSize, PDFDpi: o.PDFDpi})
	return Opts{ImageQuality: s.ImageQuality, AudioQuality: s.AudioQuality, VideoQuality: s.VideoQuality, VideoSize: s.VideoSize, PDFDpi: s.PDFDpi}
}

// Title: 화면 · 알림에 쓰는 한 줄 설명
func (r Request) Title() string {
	k := mainKind(r.Files)
	n := len(r.Files)
	what := filepath.Base(firstOr(r.Files, ""))
	if n > 1 {
		what = fmt.Sprintf("%s 외 %d개", what, n-1)
	}
	if r.Op == "tool" {
		if t := toolByID(k, r.To); t != nil {
			return what + " · " + t.Label
		}
		return what + " · " + r.To
	}
	if r.To == "unzip" {
		return what + " · 압축 풀기"
	}
	return what + " → " + targetLabel(k, r.To)
}

func firstOr(a []string, d string) string {
	if len(a) > 0 {
		return a[0]
	}
	return d
}

// Run does the whole request. It never touches the originals.
func Run(ctx context.Context, req Request, prog Progress) Result {
	res := Result{Outputs: []string{}, Errors: []string{}, Skipped: []string{}, Notes: []string{}, Title: req.Title()}
	if prog == nil {
		prog = func(float64, string) {}
	}
	files := []string{}
	for _, f := range req.Files {
		if f = strings.TrimSpace(f); f != "" {
			files = append(files, filepath.Clean(f))
		}
	}
	if len(files) == 0 {
		res.Errors = append(res.Errors, "바꿀 파일이 없어요")
		res.Message = res.Errors[0]
		return res
	}
	for _, f := range files {
		if !fileExists(f) {
			res.Errors = append(res.Errors, filepath.Base(f)+": 파일을 찾을 수 없어요 (옮기거나 지웠을 수 있어요)")
		}
	}
	files = existing(files)
	kind := mainKind(files)
	o := req.opts()

	// ffmpeg가 필요한 일이면 먼저 준비 (처음 한 번)
	if needsFFmpeg(kind, req) && !app.tools.Ready() {
		prog(0, "변환 도구를 준비하고 있어요 (처음 한 번)")
		if err := app.tools.EnsureNow(ctx, func(p float64) { prog(p*0.3, "변환 도구를 받고 있어요 (처음 한 번)") }); err != nil {
			res.Errors = append(res.Errors, "변환 도구를 준비하지 못했어요 — "+err.Error())
			res.Message = res.Errors[len(res.Errors)-1]
			return res
		}
	}

	// 여러 파일을 하나로 만드는 일
	multi := req.Op == "tool" && (req.To == "zip" || req.To == "merge" || req.To == "pdfmerge")
	if multi {
		use := files
		if req.To == "merge" {
			use = onlyKind(files, "pdf", &res)
		} else if req.To == "pdfmerge" {
			use = onlyKind(files, "image", &res)
		}
		if len(use) > 0 {
			out, err := runMulti(ctx, req, use, o, func(p float64) { prog(p, filepath.Base(use[0])) })
			record(&res, firstOr(use, ""), out, err)
		}
		finish(&res, req)
		return res
	}

	n := len(files)
	for i, f := range files {
		if ctx.Err() != nil {
			res.Errors = append(res.Errors, "취소했어요")
			break
		}
		base := filepath.Base(f)
		fk := kindOf(f)
		if fk != kind && !(kind == "other" || req.To == "zip" || req.To == "tgz") {
			res.Skipped = append(res.Skipped, fmt.Sprintf("%s: %s 파일이 아니라서 건너뛰었어요", base, kindByID(kind).Label))
			continue
		}
		sub := func(p float64) {
			if p < 0 {
				p = 0
			}
			if p > 1 {
				p = 1
			}
			prog((float64(i)+p)/float64(n), base)
		}
		sub(0)
		var outs []string
		var err error
		if req.Op == "tool" {
			outs, err = runTool(ctx, req, f, fk, o, sub, &res)
		} else {
			outs, err = runConvert(ctx, req, f, fk, o, sub, &res)
		}
		for _, out := range outs {
			app.allow(out)
		}
		recordMany(&res, f, outs, err)
		sub(1)
	}
	finish(&res, req)
	return res
}

func existing(in []string) []string {
	out := []string{}
	for _, f := range in {
		if fileExists(f) {
			out = append(out, f)
		}
	}
	return out
}

func onlyKind(files []string, kind string, res *Result) []string {
	out := []string{}
	for _, f := range files {
		if kindOf(f) == kind {
			out = append(out, f)
		} else {
			res.Skipped = append(res.Skipped, fmt.Sprintf("%s: %s 파일이 아니라서 건너뛰었어요", filepath.Base(f), kindByID(kind).Label))
		}
	}
	return out
}

func record(res *Result, src, out string, err error) {
	if out != "" {
		recordMany(res, src, []string{out}, err)
	} else {
		recordMany(res, src, nil, err)
	}
}

func recordMany(res *Result, src string, outs []string, err error) {
	if err != nil {
		var se skipErr
		if errors.As(err, &se) {
			res.Skipped = append(res.Skipped, filepath.Base(src)+": "+se.msg)
			return
		}
		res.Errors = append(res.Errors, filepath.Base(src)+": "+friendly(err))
		return
	}
	res.Outputs = append(res.Outputs, outs...)
	for _, o := range outs {
		app.allow(o)
	}
}

func friendly(err error) string {
	var ue userErr
	if errors.As(err, &ue) {
		return ue.msg
	}
	if errors.Is(err, context.Canceled) {
		return "취소했어요"
	}
	s := err.Error()
	switch {
	case strings.Contains(s, "No space left") || strings.Contains(s, "no space left"):
		return "저장 공간이 부족해요"
	case strings.Contains(s, "permission denied") || strings.Contains(s, "Operation not permitted"):
		return "그 폴더에 저장할 권한이 없어요. 설정에서 ‘기본 폴더에 저장’으로 바꿔 보세요"
	case strings.Contains(s, "Invalid data found") || strings.Contains(s, "moov atom not found"):
		return "파일이 손상됐거나 읽을 수 없는 형식이에요"
	}
	return "바꾸지 못했어요 (" + lastLine(s) + ")"
}

func finish(res *Result, req Request) {
	ok := len(res.Outputs)
	switch {
	case res.Text != "":
		res.Message = "QR 코드를 읽었어요 — 복사했어요"
	case ok > 0 && len(res.Errors) == 0:
		if ok == 1 {
			res.Message = filepath.Base(res.Outputs[0]) + " 만들었어요"
		} else {
			res.Message = fmt.Sprintf("%d개 만들었어요", ok)
		}
		if len(res.Skipped) > 0 {
			res.Message += fmt.Sprintf(" (%d개는 건너뜀)", len(res.Skipped))
		}
	case ok > 0:
		res.Message = fmt.Sprintf("%d개 만들고 %d개는 실패했어요", ok, len(res.Errors))
	case len(res.Errors) > 0:
		res.Message = res.Errors[0]
	case len(res.Skipped) > 0:
		res.Message = res.Skipped[0]
	default:
		res.Message = "할 일이 없었어요"
	}
}

// ---------------------------------------------------------------- 저장 위치 · 이름

func uploadsDir() string { return filepath.Join(app.support, "uploads") }

func isUploaded(p string) bool {
	return strings.HasPrefix(p, uploadsDir()+string(os.PathSeparator))
}

// outDir: 결과를 둘 폴더
func outDirFor(req Request, src string) string {
	s := app.Settings()
	dir := req.OutDir
	if dir == "" {
		if s.OutMode == "folder" || isUploaded(src) {
			dir = s.OutDir
		} else {
			dir = filepath.Dir(src)
		}
	}
	if !dirWritable(dir) {
		dir = s.OutDir
		if !dirWritable(dir) {
			// 기본 폴더가 지워졌거나 아직 없으면 만들어서 써요
			if dir != "" && os.MkdirAll(dir, 0o755) == nil && dirWritable(dir) {
				return dir
			}
			dir = downloadsFolder()
			os.MkdirAll(dir, 0o755)
		}
	}
	return dir
}

func dirWritable(dir string) bool {
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		return false
	}
	f, err := os.CreateTemp(dir, ".cvtroad-w-")
	if err != nil {
		return false
	}
	f.Close()
	os.Remove(f.Name())
	return true
}

// stem: 확장자를 뺀 이름 (".tar.gz"도)
func stem(p string) string {
	b := filepath.Base(p)
	lb := strings.ToLower(b)
	if strings.HasSuffix(lb, ".tar.gz") {
		return b[:len(b)-7]
	}
	if fi, err := os.Stat(p); err == nil && fi.IsDir() && !strings.HasSuffix(lb, ".rtfd") {
		return b
	}
	return strings.TrimSuffix(b, filepath.Ext(b))
}

// outPath: dir/<stem><suffix>.<ext> (같은 이름이 있으면 " (2)")
func outPath(req Request, src, suffix, ext string) string {
	return uniquePath(outDirFor(req, src), stem(src)+suffix+"."+ext)
}

// part file: 다 만든 다음에 진짜 이름으로 바꿔요 (중간에 멈춰도 반쯤 된 파일이 남지 않게)
func partPath(final string) string {
	dir := filepath.Dir(final)
	return filepath.Join(dir, "."+randID(3)+"-"+filepath.Base(final)+".cvtpart"+filepath.Ext(final))
}

func commit(part, final string) error {
	st, err := os.Stat(part)
	if err != nil {
		return uerr("결과 파일이 만들어지지 않았어요")
	}
	if !st.IsDir() && st.Size() == 0 {
		os.Remove(part)
		return uerr("결과 파일이 비어 있어요")
	}
	if fileExists(final) {
		final = uniquePath(filepath.Dir(final), filepath.Base(final))
	}
	if err := os.Rename(part, final); err != nil {
		os.RemoveAll(part)
		return err
	}
	now := time.Now()
	os.Chtimes(final, now, now)
	return nil
}

// withPart runs fn writing into a temporary path and moves it into place when it succeeds.
func withPart(final string, fn func(part string) error) (string, error) {
	part := partPath(final)
	if err := fn(part); err != nil {
		os.RemoveAll(part)
		return "", err
	}
	if err := commit(part, final); err != nil {
		return "", err
	}
	return final, nil
}

func tempDir(prefix string) string {
	base := filepath.Join(app.support, "tmp")
	os.MkdirAll(base, 0o755)
	d, err := os.MkdirTemp(base, prefix)
	if err != nil {
		d = filepath.Join(base, prefix+randID(4))
		os.MkdirAll(d, 0o755)
	}
	return d
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func fileSize(p string) int64 {
	if st, err := os.Stat(p); err == nil {
		return st.Size()
	}
	return 0
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1fGB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0fKB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%dB", n)
}

// ---------------------------------------------------------------- 나눠 주기

func needsFFmpeg(kind string, req Request) bool {
	switch kind {
	case "video", "audio":
		return true
	case "image":
		if req.Op == "convert" {
			return req.To == "webp" || req.To == "avif" || req.To == "ico" || !hasSips()
		}
		return req.To == "compress" || req.To == "strip" || !hasSips()
	}
	return false
}

func runConvert(ctx context.Context, req Request, f, kind string, o Opts, prog func(float64), res *Result) ([]string, error) {
	switch kind {
	case "image":
		if req.To == "pdf" {
			out := outPath(req, f, "", "pdf")
			p, err := withPart(out, func(part string) error { return imagesToPDF(ctx, []string{f}, part) })
			return one(p), err
		}
		out := outPath(req, f, "", extFor(req.To))
		if extOf(f) == req.To || (req.To == "jpg" && extOf(f) == "jpeg") || (req.To == "tiff" && extOf(f) == "tif") {
			return nil, skipErr{"이미 " + targetLabel(kind, req.To) + " 파일이에요"}
		}
		p, err := withPart(out, func(part string) error { return convertImage(ctx, f, req.To, part, o) })
		return one(p), err
	case "video":
		if extOf(f) == req.To {
			return nil, skipErr{"이미 " + targetLabel(kind, req.To) + " 파일이에요"}
		}
		out := outPath(req, f, "", extFor(req.To))
		note := ""
		p, err := withPart(out, func(part string) error {
			var e error
			note, e = convertVideo(ctx, f, req.To, part, o, prog)
			return e
		})
		if note != "" && err == nil {
			res.Notes = append(res.Notes, filepath.Base(f)+": "+note)
		}
		return one(p), err
	case "audio":
		if extOf(f) == req.To && req.To != "m4a" {
			return nil, skipErr{"이미 " + targetLabel(kind, req.To) + " 파일이에요"}
		}
		suffix := ""
		if extOf(f) == req.To {
			suffix = " (" + qualityLabel(o.AudioQuality) + ")"
		}
		out := outPath(req, f, suffix, extFor(req.To))
		note := ""
		p, err := withPart(out, func(part string) error {
			var e error
			note, e = convertAudio(ctx, f, req.To, part, o, prog)
			return e
		})
		if note != "" && err == nil {
			res.Notes = append(res.Notes, filepath.Base(f)+": "+note)
		}
		return one(p), err
	case "pdf":
		return pdfConvert(ctx, req, f, o, prog)
	case "doc":
		if extOf(f) == req.To || (req.To == "html" && extOf(f) == "htm") {
			return nil, skipErr{"이미 " + targetLabel(kind, req.To) + " 파일이에요"}
		}
		out := outPath(req, f, "", extFor(req.To))
		p, err := withPart(out, func(part string) error { return convertDoc(ctx, f, req.To, part) })
		return one(p), err
	case "archive":
		return archiveConvert(ctx, req, f, prog)
	case "folder", "other":
		if req.To == "tgz" {
			out := uniquePath(outDirFor(req, f), stem(f)+".tar.gz")
			p, err := withPart(out, func(part string) error { return tarGzFiles(ctx, []string{f}, part, prog) })
			return one(p), err
		}
		out := outPath(req, f, "", "zip")
		p, err := withPart(out, func(part string) error { return zipFiles(ctx, []string{f}, part, prog) })
		return one(p), err
	}
	return nil, uerr("이 파일은 바꿀 수 없어요")
}

func one(p string) []string {
	if p == "" {
		return nil
	}
	return []string{p}
}

func extFor(id string) string {
	switch id {
	case "tgz":
		return "tar.gz"
	}
	return id
}

func qualityLabel(q string) string {
	if q == "lossless" {
		return "무손실"
	}
	return q + "kbps"
}

func runTool(ctx context.Context, req Request, f, kind string, o Opts, prog func(float64), res *Result) ([]string, error) {
	switch kind {
	case "image":
		return imageTool(ctx, req, f, o, prog, res)
	case "video":
		return videoTool(ctx, req, f, o, prog)
	case "audio":
		return audioTool(ctx, req, f, o, prog)
	case "pdf":
		return pdfTool(ctx, req, f, o, prog)
	case "archive":
		if req.To == "unzip" {
			return archiveConvert(ctx, Request{Op: "convert", To: "unzip", OutDir: req.OutDir}, f, prog)
		}
	}
	return nil, uerr("이 파일에는 쓸 수 없는 도구예요")
}

func runMulti(ctx context.Context, req Request, files []string, o Opts, prog func(float64)) (string, error) {
	first := files[0]
	name := stem(first)
	if len(files) > 1 {
		name = fmt.Sprintf("%s 외 %d개", name, len(files)-1)
	}
	switch req.To {
	case "zip":
		out := uniquePath(outDirFor(req, first), name+".zip")
		return withPart(out, func(part string) error { return zipFiles(ctx, files, part, prog) })
	case "merge":
		if len(files) < 2 {
			return "", uerr("합치려면 PDF를 두 개 이상 골라 주세요")
		}
		out := uniquePath(outDirFor(req, first), name+".pdf")
		return withPart(out, func(part string) error { return mergePDFs(files, part) })
	case "pdfmerge":
		if len(files) > 1 {
			name = fmt.Sprintf("%s 외 %d장", stem(first), len(files)-1)
		}
		out := uniquePath(outDirFor(req, first), name+".pdf")
		return withPart(out, func(part string) error { return imagesToPDF(ctx, files, part) })
	}
	return "", uerr("알 수 없는 작업이에요")
}
