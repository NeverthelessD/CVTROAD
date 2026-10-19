package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// 압축: ZIP · TAR.GZ 만들기와 풀기. 이름은 Windows에서도 깨지지 않게 NFC(한글 완성형)로 저장해요.

func skipName(name string) bool {
	return name == ".DS_Store" || name == "__MACOSX" || strings.HasPrefix(name, "._") || strings.HasSuffix(name, ".cvtpart")
}

type packItem struct {
	abs  string
	name string // 압축 안 이름 (슬래시)
	info fs.FileInfo
}

func collectItems(files []string) ([]packItem, int64) {
	items := []packItem{}
	var total int64
	for _, f := range files {
		base := filepath.Dir(f)
		filepath.Walk(f, func(p string, info fs.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if skipName(info.Name()) {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return nil
			}
			rel, _ := filepath.Rel(base, p)
			name := norm.NFC.String(filepath.ToSlash(rel))
			items = append(items, packItem{p, name, info})
			if !info.IsDir() {
				total += info.Size()
			}
			return nil
		})
	}
	return items, total
}

type progressReader struct {
	r     io.Reader
	done  *int64
	total int64
	prog  func(float64)
	last  time.Time
	ctx   context.Context
}

func (pr *progressReader) Read(b []byte) (int, error) {
	if pr.ctx != nil && pr.ctx.Err() != nil {
		return 0, pr.ctx.Err()
	}
	n, err := pr.r.Read(b)
	*pr.done += int64(n)
	if pr.prog != nil && pr.total > 0 && time.Since(pr.last) > 120*time.Millisecond {
		pr.prog(float64(*pr.done) / float64(pr.total))
		pr.last = time.Now()
	}
	return n, err
}

func zipFiles(ctx context.Context, files []string, dst string, prog func(float64)) error {
	items, total := collectItems(files)
	if len(items) == 0 {
		return uerr("묶을 파일이 없어요")
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)
	var done int64
	for _, it := range items {
		if ctx.Err() != nil {
			zw.Close()
			f.Close()
			return ctx.Err()
		}
		h, err := zip.FileInfoHeader(it.info)
		if err != nil {
			continue
		}
		h.Name = it.name
		if it.info.IsDir() {
			h.Name += "/"
			h.Method = zip.Store
		} else {
			h.Method = zip.Deflate
			switch strings.ToLower(path.Ext(it.name)) {
			case ".jpg", ".jpeg", ".png", ".heic", ".mp4", ".mov", ".mp3", ".m4a", ".zip", ".gz", ".webp", ".avif", ".mkv", ".webm":
				h.Method = zip.Store // 이미 압축된 파일은 그대로 (빠르게)
			}
		}
		w, err := zw.CreateHeader(h)
		if err != nil {
			zw.Close()
			f.Close()
			return err
		}
		if it.info.IsDir() {
			continue
		}
		in, err := os.Open(it.abs)
		if err != nil {
			continue
		}
		_, err = io.Copy(w, &progressReader{r: in, done: &done, total: total, prog: prog, ctx: ctx})
		in.Close()
		if err != nil {
			zw.Close()
			f.Close()
			return err
		}
	}
	if err := zw.Close(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func tarGzFiles(ctx context.Context, files []string, dst string, prog func(float64)) error {
	items, total := collectItems(files)
	if len(items) == 0 {
		return uerr("묶을 파일이 없어요")
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	var done int64
	closeAll := func() error {
		tw.Close()
		gz.Close()
		return f.Close()
	}
	for _, it := range items {
		if ctx.Err() != nil {
			closeAll()
			return ctx.Err()
		}
		h, err := tar.FileInfoHeader(it.info, "")
		if err != nil {
			continue
		}
		h.Name = it.name
		if it.info.IsDir() {
			h.Name += "/"
		}
		h.Uname, h.Gname = "", ""
		h.Format = tar.FormatPAX
		if err := tw.WriteHeader(h); err != nil {
			closeAll()
			return err
		}
		if it.info.IsDir() {
			continue
		}
		in, err := os.Open(it.abs)
		if err != nil {
			continue
		}
		_, err = io.Copy(tw, &progressReader{r: in, done: &done, total: total, prog: prog, ctx: ctx})
		in.Close()
		if err != nil {
			closeAll()
			return err
		}
	}
	if err := tw.Close(); err != nil {
		f.Close()
		return err
	}
	if err := gz.Close(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// ---------------------------------------------------------------- 풀기

// zipName: UTF-8 표시가 없고 UTF-8도 아닌 이름은 Windows 한국어(CP949)로 읽어요
func zipName(f *zip.File) string {
	n := f.Name
	if f.NonUTF8 || !utf8.ValidString(n) {
		if s, _, err := transform.String(korean.EUCKR.NewDecoder(), n); err == nil && utf8.ValidString(s) {
			n = s
		}
	}
	return norm.NFC.String(n)
}

func safeRel(name string) (string, bool) {
	name = strings.ReplaceAll(name, `\`, "/")
	c := path.Clean("/" + name)[1:]
	if c == "" || c == "." || strings.HasPrefix(c, "../") {
		return "", false
	}
	for _, part := range strings.Split(c, "/") {
		if skipName(part) {
			return "", false
		}
	}
	return c, true
}

type unpackEntry struct {
	name string
	dir  bool
	mode os.FileMode
	mt   time.Time
	open func() (io.ReadCloser, error)
	size int64
}

func listArchive(p string) ([]unpackEntry, func(), error) {
	ext := extOf(p)
	switch ext {
	case "zip":
		zr, err := zip.OpenReader(p)
		if err != nil {
			return nil, nil, uerr("압축 파일이 손상됐거나 ZIP이 아니에요")
		}
		out := []unpackEntry{}
		for _, f := range zr.File {
			if f.Flags&0x1 != 0 {
				zr.Close()
				return nil, nil, uerr("암호가 걸린 압축 파일은 풀 수 없어요")
			}
			name, ok := safeRel(zipName(f))
			if !ok {
				continue
			}
			f := f
			mode := f.Mode()
			if mode&os.ModeSymlink != 0 {
				continue
			}
			out = append(out, unpackEntry{name: name, dir: f.FileInfo().IsDir() || strings.HasSuffix(f.Name, "/"), mode: mode.Perm(), mt: f.Modified,
				open: func() (io.ReadCloser, error) { return f.Open() }, size: int64(f.UncompressedSize64)})
		}
		return out, func() { zr.Close() }, nil
	case "tar", "tgz", "gz":
		fh, err := os.Open(p)
		if err != nil {
			return nil, nil, err
		}
		var r io.Reader = fh
		if ext != "tar" {
			gz, err := gzip.NewReader(fh)
			if err != nil {
				fh.Close()
				return nil, nil, uerr("압축 파일이 손상됐어요")
			}
			r = gz
		}
		if ext == "gz" && !strings.HasSuffix(strings.ToLower(p), ".tar.gz") {
			// 파일 하나를 gzip으로 묶은 것 (예: log.txt.gz)
			name := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
			data := r
			return []unpackEntry{{name: name, mode: 0o644, mt: time.Now(), open: func() (io.ReadCloser, error) { return io.NopCloser(data), nil }}},
				func() { fh.Close() }, nil
		}
		// tar는 앞에서부터 차례로 읽어야 해서 임시 폴더에 먼저 풀어요
		tmp := tempDir("untar-")
		tr := tar.NewReader(r)
		out := []unpackEntry{}
		for {
			h, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				fh.Close()
				os.RemoveAll(tmp)
				return nil, nil, uerr("압축 파일이 손상됐어요")
			}
			name, ok := safeRel(norm.NFC.String(h.Name))
			if !ok {
				continue
			}
			switch h.Typeflag {
			case tar.TypeDir:
				out = append(out, unpackEntry{name: name, dir: true, mode: 0o755, mt: h.ModTime})
			case tar.TypeReg, tar.TypeRegA:
				tp := filepath.Join(tmp, randID(6))
				w, err := os.Create(tp)
				if err != nil {
					continue
				}
				io.Copy(w, tr)
				w.Close()
				out = append(out, unpackEntry{name: name, mode: os.FileMode(h.Mode).Perm(), mt: h.ModTime, size: h.Size,
					open: func() (io.ReadCloser, error) { return os.Open(tp) }})
			}
		}
		return out, func() { fh.Close(); os.RemoveAll(tmp) }, nil
	}
	return nil, nil, uerr("풀 수 없는 압축 형식이에요")
}

// unpackTo extracts entries into dir (dir must exist).
func unpackTo(ctx context.Context, entries []unpackEntry, dir string, prog func(float64)) error {
	var total, done int64
	for _, e := range entries {
		total += e.size
	}
	for _, e := range entries {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		target := filepath.Join(dir, filepath.FromSlash(e.name))
		if e.dir {
			os.MkdirAll(target, 0o755)
			continue
		}
		os.MkdirAll(filepath.Dir(target), 0o755)
		rc, err := e.open()
		if err != nil {
			return uerr("압축 파일 안의 %s을(를) 읽지 못했어요", path.Base(e.name))
		}
		mode := e.mode
		if mode == 0 {
			mode = 0o644
		}
		w, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode|0o600)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(w, &progressReader{r: rc, done: &done, total: total, prog: prog, ctx: ctx})
		rc.Close()
		w.Close()
		if err != nil {
			return uerr("압축 파일이 손상됐어요")
		}
		if !e.mt.IsZero() {
			os.Chtimes(target, e.mt, e.mt)
		}
	}
	return nil
}

// topFolder: 모든 항목이 폴더 하나 안에 있으면 그 이름
func topFolder(entries []unpackEntry) string {
	top := ""
	for _, e := range entries {
		first := strings.SplitN(e.name, "/", 2)[0]
		if !strings.Contains(e.name, "/") && !e.dir {
			return ""
		}
		if top == "" {
			top = first
		} else if top != first {
			return ""
		}
	}
	return top
}

func archiveConvert(ctx context.Context, req Request, f string, prog func(float64)) ([]string, error) {
	entries, done, err := listArchive(f)
	if err != nil {
		return nil, err
	}
	defer done()
	if len(entries) == 0 {
		return nil, skipErr{"압축 파일이 비어 있어요"}
	}
	outDir := outDirFor(req, f)
	switch req.To {
	case "unzip":
		if top := topFolder(entries); top != "" {
			// 폴더 하나가 통째로 들어 있으면 그 폴더를 그대로 꺼내요
			dest := uniquePath(outDir, top)
			part := partPath(dest)
			os.MkdirAll(part, 0o755)
			if err := unpackTo(ctx, entries, part, prog); err != nil {
				os.RemoveAll(part)
				return nil, err
			}
			inner := filepath.Join(part, top)
			if err := os.Rename(inner, dest); err != nil {
				os.RemoveAll(part)
				return nil, err
			}
			os.RemoveAll(part)
			return []string{dest}, nil
		}
		if len(entries) == 1 && !entries[0].dir {
			dest := uniquePath(outDir, path.Base(entries[0].name))
			p, err := withPart(dest, func(part string) error {
				tmp := tempDir("one-")
				defer os.RemoveAll(tmp)
				if err := unpackTo(ctx, entries, tmp, prog); err != nil {
					return err
				}
				return moveFile(filepath.Join(tmp, filepath.FromSlash(entries[0].name)), part)
			})
			return one(p), err
		}
		dest := uniquePath(outDir, stem(f))
		p, err := withPart(dest, func(part string) error {
			os.MkdirAll(part, 0o755)
			return unpackTo(ctx, entries, part, prog)
		})
		return one(p), err
	case "zip", "tgz":
		if (req.To == "zip" && extOf(f) == "zip") || (req.To == "tgz" && extOf(f) == "tgz") {
			return nil, skipErr{"이미 " + targetLabel("archive", req.To) + " 파일이에요"}
		}
		tmp := tempDir("rearc-")
		defer os.RemoveAll(tmp)
		root := filepath.Join(tmp, stem(f))
		os.MkdirAll(root, 0o755)
		if err := unpackTo(ctx, entries, root, func(p float64) { prog(p * 0.5) }); err != nil {
			return nil, err
		}
		kids, _ := os.ReadDir(root)
		items := []string{}
		for _, k := range kids {
			items = append(items, filepath.Join(root, k.Name()))
		}
		ext := "zip"
		if req.To == "tgz" {
			ext = "tar.gz"
		}
		out := uniquePath(outDir, stem(f)+"."+ext)
		p, err := withPart(out, func(part string) error {
			sub := func(p float64) { prog(0.5 + p*0.5) }
			if req.To == "tgz" {
				return tarGzFiles(ctx, items, part, sub)
			}
			return zipFiles(ctx, items, part, sub)
		})
		return one(p), err
	}
	return nil, uerr("이 형식으로는 바꿀 수 없어요")
}

func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := copyFile(src, dst); err != nil {
		return err
	}
	return os.Remove(src)
}
