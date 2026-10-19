package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// PDF: 나누기 · 합치기 · 회전 · 페이지 정리 · 정보 지우기는 pdfcpu(이 앱 안에 들어 있음)로,
// 쪽 그림 만들기 · 글자 꺼내기 · 용량 줄이기는 맥의 PDFKit(helper.js)으로 해요.

func pdfConf() *model.Configuration {
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	return conf
}

func init() {
	api.DisableConfigDir()
}

func pdfErr(err error) error {
	s := strings.ToLower(err.Error())
	switch {
	case strings.Contains(s, "encrypt") || strings.Contains(s, "password"):
		return uerr("암호가 걸린 PDF는 바꿀 수 없어요")
	case strings.Contains(s, "eof") || strings.Contains(s, "corrupt") || strings.Contains(s, "xref"):
		return uerr("PDF 파일이 손상됐어요")
	}
	return uerr("PDF를 처리하지 못했어요 (%s)", lastLine(err.Error()))
}

func pageCount(p string) (int, error) {
	n, err := api.PageCountFile(p)
	if err != nil {
		return 0, pdfErr(err)
	}
	return n, nil
}

func mergePDFs(files []string, dst string) error {
	if err := api.MergeCreateFile(files, dst, false, pdfConf()); err != nil {
		return pdfErr(err)
	}
	return nil
}

func pdfConvert(ctx context.Context, req Request, f string, o Opts, prog func(float64)) ([]string, error) {
	switch req.To {
	case "png", "jpg", "tiff":
		return pdfToImages(ctx, req, f, req.To, o.PDFDpi, prog)
	case "txt", "docx", "rtf":
		text, err := pdfText(f)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(text) == "" {
			return nil, uerr("글자가 없는 PDF예요 (스캔한 문서일 수 있어요)")
		}
		out := outPath(req, f, "", req.To)
		p, err := withPart(out, func(part string) error { return writeTextAs(text, req.To, part) })
		return one(p), err
	}
	return nil, uerr("이 형식으로는 바꿀 수 없어요")
}

func pdfText(f string) (string, error) {
	if !helperAvailable() {
		return "", uerr("이 컴퓨터에서는 PDF 글자를 꺼낼 수 없어요")
	}
	var text string
	var herr string
	err := runHelper("pdftext", map[string]any{"in": f}, 2*time.Minute, func(h helperLine) {
		if h.Text != "" {
			text = h.Text
		}
		if h.Error != "" {
			herr = h.Error
		}
		if h.Encrypted {
			herr = "암호가 걸린 PDF는 바꿀 수 없어요"
		}
	})
	if herr != "" {
		return "", uerr("%s", herr)
	}
	if err != nil && text == "" {
		return "", uerr("PDF 글자를 꺼내지 못했어요")
	}
	return text, nil
}

func pdfToImages(ctx context.Context, req Request, f, fmtID string, dpi int, prog func(float64)) ([]string, error) {
	if !helperAvailable() {
		return nil, uerr("이 컴퓨터에서는 PDF를 그림으로 바꿀 수 없어요")
	}
	n, err := pageCount(f)
	if err != nil {
		return nil, err
	}
	tmp := tempDir("pdfimg-")
	defer os.RemoveAll(tmp)
	var herr string
	err = runHelper("pdfimg", map[string]any{"in": f, "outDir": tmp, "fmt": fmtID, "dpi": dpi, "prefix": stem(f)}, 2*time.Minute, func(h helperLine) {
		if h.Tick > 0 && h.Pages > 0 {
			prog(float64(h.Page) / float64(h.Pages))
		}
		if h.Error != "" {
			herr = h.Error
		}
	})
	if herr != "" {
		return nil, uerr("%s", herr)
	}
	if err != nil {
		return nil, uerr("PDF를 그림으로 바꾸지 못했어요")
	}
	made, _ := filepath.Glob(filepath.Join(tmp, "*."+fmtID))
	sort.Strings(made)
	if len(made) == 0 {
		return nil, uerr("PDF를 그림으로 바꾸지 못했어요")
	}
	if len(made) == 1 && n <= 1 {
		out := outPath(req, f, "", fmtID)
		if err := os.Rename(made[0], out); err != nil {
			if err := copyFile(made[0], out); err != nil {
				return nil, err
			}
		}
		return []string{out}, nil
	}
	// 여러 쪽: "<이름> (PNG)" 폴더에 001, 002 …
	dir := uniquePath(outDirFor(req, f), stem(f)+" ("+strings.ToUpper(fmtID)+")")
	part := partPath(dir)
	if err := os.MkdirAll(part, 0o755); err != nil {
		return nil, err
	}
	for _, m := range made {
		if err := os.Rename(m, filepath.Join(part, filepath.Base(m))); err != nil {
			copyFile(m, filepath.Join(part, filepath.Base(m)))
		}
	}
	if err := commit(part, dir); err != nil {
		return nil, err
	}
	return []string{dir}, nil
}

func pdfTool(ctx context.Context, req Request, f string, o Opts, prog func(float64)) ([]string, error) {
	switch req.To {
	case "compress":
		out := outPath(req, f, " (압축)", "pdf")
		p, err := withPart(out, func(part string) error { return compressPDF(f, part) })
		if err == nil && fileSize(p) >= fileSize(f)*97/100 {
			removeQuiet(p)
			return nil, skipErr{"이미 충분히 작아서 더 줄일 수 없었어요"}
		}
		return one(p), err
	case "split":
		n, err := pageCount(f)
		if err != nil {
			return nil, err
		}
		if n < 2 {
			return nil, skipErr{"한 쪽짜리 PDF라 나눌 게 없어요"}
		}
		tmp := tempDir("split-")
		defer os.RemoveAll(tmp)
		if err := api.SplitFile(f, tmp, 1, pdfConf()); err != nil {
			return nil, pdfErr(err)
		}
		made, _ := filepath.Glob(filepath.Join(tmp, "*.pdf"))
		sort.Slice(made, func(i, j int) bool { return trailingNum(made[i]) < trailingNum(made[j]) })
		dir := uniquePath(outDirFor(req, f), stem(f)+" (쪽마다)")
		part := partPath(dir)
		os.MkdirAll(part, 0o755)
		w := len(strconv.Itoa(len(made)))
		for i, m := range made {
			name := fmt.Sprintf("%s %0*d쪽.pdf", stem(f), w, i+1)
			if err := os.Rename(m, filepath.Join(part, name)); err != nil {
				copyFile(m, filepath.Join(part, name))
			}
			prog(float64(i+1) / float64(len(made)))
		}
		if err := commit(part, dir); err != nil {
			return nil, err
		}
		return []string{dir}, nil
	case "rotate":
		out := outPath(req, f, " (회전)", "pdf")
		p, err := withPart(out, func(part string) error {
			if err := api.RotateFile(f, part, 90, nil, pdfConf()); err != nil {
				return pdfErr(err)
			}
			return nil
		})
		return one(p), err
	case "strip":
		out := outPath(req, f, " (정보 삭제)", "pdf")
		p, err := withPart(out, func(part string) error { return stripPDF(f, part) })
		return one(p), err
	case "pages":
		if len(req.Pages) == 0 {
			return nil, uerr("남길 쪽이 하나도 없어요")
		}
		out := outPath(req, f, " (정리)", "pdf")
		p, err := withPart(out, func(part string) error { return editPages(f, part, req.Pages) })
		return one(p), err
	}
	return nil, uerr("이 PDF에는 쓸 수 없는 도구예요")
}

func trailingNum(p string) int {
	s := strings.TrimSuffix(filepath.Base(p), ".pdf")
	i := len(s)
	for i > 0 && s[i-1] >= '0' && s[i-1] <= '9' {
		i--
	}
	n, _ := strconv.Atoi(s[i:])
	return n
}

// editPages: 새 순서대로 모으고(collect), 쪽마다 회전
func editPages(src, dst string, pages []PageEdit) error {
	n, err := pageCount(src)
	if err != nil {
		return err
	}
	sel := []string{}
	for _, pe := range pages {
		if pe.Page < 1 || pe.Page > n {
			return uerr("%d쪽이 없어요", pe.Page)
		}
		sel = append(sel, strconv.Itoa(pe.Page))
	}
	tmp := dst + ".collect.pdf"
	defer os.Remove(tmp)
	if err := api.CollectFile(src, tmp, sel, pdfConf()); err != nil {
		return pdfErr(err)
	}
	byRot := map[int][]string{}
	for i, pe := range pages {
		r := ((pe.Rotate % 360) + 360) % 360
		if r != 0 {
			byRot[r] = append(byRot[r], strconv.Itoa(i+1))
		}
	}
	cur := tmp
	for _, r := range []int{90, 180, 270} {
		if len(byRot[r]) == 0 {
			continue
		}
		next := fmt.Sprintf("%s.r%d.pdf", dst, r)
		defer os.Remove(next)
		if err := api.RotateFile(cur, next, r, byRot[r], pdfConf()); err != nil {
			return pdfErr(err)
		}
		cur = next
	}
	return copyFile(cur, dst)
}

// stripPDF: 문서 정보(작성자 · 만든 프로그램 · 날짜)와 XMP 정보 지우기
func stripPDF(src, dst string) error {
	ctx, err := api.ReadContextFile(src)
	if err != nil {
		return pdfErr(err)
	}
	if ctx.XRefTable.Info != nil {
		if d, err := ctx.DereferenceDict(*ctx.XRefTable.Info); err == nil && d != nil {
			for k := range d {
				delete(d, k)
			}
		}
		ctx.XRefTable.Info = nil
	}
	if root, err := ctx.Catalog(); err == nil && root != nil {
		delete(root, "Metadata")
	}
	ctx.XRefTable.Title, ctx.XRefTable.Author, ctx.XRefTable.Subject = "", "", ""
	ctx.XRefTable.Creator, ctx.XRefTable.Producer = "", ""
	ctx.XRefTable.Keywords = ""
	ctx.XRefTable.CreationDate, ctx.XRefTable.ModDate = "", ""
	if err := api.WriteContextFile(ctx, dst); err != nil {
		return pdfErr(err)
	}
	return nil
}

// compressPDF: 맥에서는 PDFKit으로 그림을 다시 압축하고, 결과가 더 작은 쪽을 남겨요
func compressPDF(src, dst string) error {
	best := ""
	tmp1 := dst + ".kit.pdf"
	defer os.Remove(tmp1)
	if helperAvailable() {
		var herr string
		err := runHelper("pdfcompress", map[string]any{"in": src, "out": tmp1}, 5*time.Minute, func(h helperLine) {
			if h.Error != "" {
				herr = h.Error
			}
		})
		if err == nil && herr == "" && fileSize(tmp1) > 0 {
			best = tmp1
		}
	}
	tmp2 := dst + ".opt.pdf"
	defer os.Remove(tmp2)
	if err := api.OptimizeFile(src, tmp2, pdfConf()); err == nil && fileSize(tmp2) > 0 {
		if best == "" || fileSize(tmp2) < fileSize(best) {
			best = tmp2
		}
	} else if best == "" {
		return pdfErr(err)
	}
	return copyFile(best, dst)
}

// ---------------------------------------------------------------- 페이지 정리 화면용 정보

func (a *App) handlePDFPages(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Path string `json:"path"`
	}
	readJSON(r, &in)
	if !a.isAllowed(in.Path) {
		jsonError(w, 403, "열 수 없는 파일이에요")
		return
	}
	dims, err := api.PageDimsFile(in.Path)
	if err != nil {
		jsonError(w, 400, friendly(pdfErr(err)))
		return
	}
	pages := []map[string]float64{}
	for _, d := range dims {
		pages = append(pages, map[string]float64{"w": d.Width, "h": d.Height})
	}
	writeJSON(w, map[string]any{"ok": true, "pages": pages})
}
