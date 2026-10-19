package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// 이미지: 맥에 들어 있는 sips(사진 앱과 같은 엔진)를 먼저 쓰고, WEBP · AVIF · ICO는 ffmpeg로 만들어요.

func hasSips() bool {
	return runtime.GOOS == "darwin" && os.Getenv("CVTROAD_NO_SIPS") == "" && isExecutable("/usr/bin/sips")
}

var sipsFormat = map[string]string{"jpg": "jpeg", "png": "png", "heic": "heic", "tiff": "tiff", "bmp": "bmp", "gif": "gif"}

// ffmpeg가 그대로 읽을 수 있는 이미지 (HEIC · RAW · PSD는 sips로 먼저 풀어요)
func ffmpegReadable(ext string) bool {
	switch ext {
	case "jpg", "jpeg", "png", "webp", "bmp", "gif", "tif", "tiff", "tga", "ico", "jp2", "avif":
		return true
	}
	return false
}

func runCmd(ctx context.Context, timeout time.Duration, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return uerr("시간이 너무 오래 걸려서 멈췄어요")
	}
	if err != nil {
		return fmt.Errorf("%s", lastLine(string(out)))
	}
	return nil
}

func sips(ctx context.Context, args ...string) error {
	return runCmd(ctx, 3*time.Minute, "/usr/bin/sips", args...)
}

func ffImage(ctx context.Context, args ...string) error {
	full := append([]string{"-hide_banner", "-nostdin", "-y", "-v", "error"}, args...)
	return runCmd(ctx, 5*time.Minute, app.tools.Path("ffmpeg"), full...)
}

// decodedSource: ffmpeg가 읽을 수 있는 파일 (필요하면 sips로 PNG를 만들어 줘요)
func decodedSource(ctx context.Context, src, tmp string) (string, error) {
	if ffmpegReadable(extOf(src)) {
		return src, nil
	}
	if !hasSips() {
		return "", uerr("이 컴퓨터에서는 %s 파일을 읽을 수 없어요", strings.ToUpper(extOf(src)))
	}
	out := filepath.Join(tmp, "src.png")
	if err := sips(ctx, "-s", "format", "png", src, "--out", out); err != nil {
		return "", uerr("이미지를 읽지 못했어요")
	}
	return out, nil
}

// hasAlpha: 투명한 부분이 있을 수 있는 이미지인지 (JPG로 바꿀 때 흰 바탕을 깔아요)
func hasAlpha(ctx context.Context, p string) bool {
	switch extOf(p) {
	case "png", "webp", "gif", "tif", "tiff", "ico", "avif", "heic", "heif", "psd", "tga":
	default:
		return false
	}
	if !isExecutable(app.tools.Path("ffprobe")) || !ffmpegReadable(extOf(p)) {
		return extOf(p) == "png" || extOf(p) == "gif"
	}
	pi, err := probe(ctx, p)
	if err != nil {
		return false
	}
	v := pi.video()
	if v == nil {
		return false
	}
	pf := v.PixFmt
	return strings.Contains(pf, "a") && (strings.HasPrefix(pf, "rgba") || strings.HasPrefix(pf, "bgra") || strings.HasPrefix(pf, "argb") ||
		strings.HasPrefix(pf, "abgr") || strings.HasPrefix(pf, "ya") || strings.HasPrefix(pf, "yuva") || strings.HasPrefix(pf, "gbrap") || pf == "pal8")
}

func jpegQ(q int) string {
	// ffmpeg -q:v 2(가장 좋음) ~ 31
	v := (100-q)/5 + 1
	if v < 2 {
		v = 2
	}
	if v > 31 {
		v = 31
	}
	return strconv.Itoa(v)
}

func avifCRF(q int) string {
	c := int(63 - float64(q)*0.43 + 0.5)
	if c < 10 {
		c = 10
	}
	if c > 55 {
		c = 55
	}
	return strconv.Itoa(c)
}

// uprightSource: 돌려 찍은 사진(EXIF 방향)을 실제로 세운 사본 (방향 정보를 담지 못하는 형식으로 바꿀 때)
func uprightSource(ctx context.Context, src, tmp string) string {
	ext := extOf(src)
	if ext == "jpg" || ext == "jpeg" {
		if jpegOrientation(src) > 1 && isExecutable(app.tools.Path("ffmpeg")) {
			up := filepath.Join(tmp, "upright.jpg")
			if uprightJPEG(ctx, src, up) == nil {
				return up
			}
		}
		return src
	}
	if hasSips() && helperAvailable() {
		out, err := exec.CommandContext(ctx, "/usr/bin/sips", "-g", "orientation", src).Output()
		if err == nil {
			for _, l := range strings.Split(string(out), "\n") {
				l = strings.TrimSpace(l)
				if strings.HasPrefix(l, "orientation:") {
					if n, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(l, "orientation:"))); n > 1 {
						up := filepath.Join(tmp, "upright.png")
						if helperSimple("upright", map[string]any{"in": src, "out": up}, time.Minute) == nil && fileSize(up) > 0 {
							return up
						}
					}
				}
			}
		}
	}
	return src
}

func convertImage(ctx context.Context, src, to, dst string, o Opts) error {
	tmp := tempDir("img-")
	defer os.RemoveAll(tmp)
	q := o.ImageQuality
	if to != "jpg" && to != "heic" {
		src = uprightSource(ctx, src, tmp)
	}
	alphaToFlat := (to == "jpg" || to == "bmp") && hasAlpha(ctx, src)

	// 1) sips (맥)
	if f, ok := sipsFormat[to]; ok && hasSips() && !alphaToFlat {
		args := []string{"-s", "format", f}
		if to == "jpg" || to == "heic" {
			args = append(args, "-s", "formatOptions", strconv.Itoa(q))
		}
		args = append(args, src, "--out", dst)
		if err := sips(ctx, args...); err == nil && fileSize(dst) > 0 {
			return nil
		} else if to == "heic" {
			os.Remove(dst)
			if err := sips(ctx, "-s", "format", "heic", src, "--out", dst); err == nil && fileSize(dst) > 0 {
				return nil
			}
			return uerr("HEIC로 저장하지 못했어요")
		}
		os.Remove(dst)
	}
	if to == "heic" {
		return uerr("이 컴퓨터에서는 HEIC로 저장할 수 없어요")
	}

	// 2) ffmpeg
	in, err := decodedSource(ctx, src, tmp)
	if err != nil {
		return err
	}
	args := []string{"-i", in}
	switch to {
	case "jpg":
		if alphaToFlat {
			args = append(args, "-filter_complex", "[0:v]split[a][b];[a]drawbox=c=white:t=fill[bg];[bg][b]overlay=format=auto,format=yuvj444p")
		}
		args = append(args, "-frames:v", "1", "-q:v", jpegQ(q), "-update", "1", dst)
	case "png":
		args = append(args, "-frames:v", "1", "-update", "1", dst)
	case "bmp":
		if alphaToFlat {
			args = append(args, "-filter_complex", "[0:v]split[a][b];[a]drawbox=c=white:t=fill[bg];[bg][b]overlay=format=auto,format=bgr24")
		}
		args = append(args, "-frames:v", "1", "-update", "1", dst)
	case "tiff":
		args = append(args, "-frames:v", "1", "-compression_algo", "lzw", "-update", "1", dst)
	case "gif":
		if extOf(src) == "gif" || extOf(src) == "webp" {
			args = append(args, "-filter_complex", "split[a][b];[a]palettegen[p];[b][p]paletteuse", "-loop", "0", dst)
		} else {
			args = append(args, "-filter_complex", "split[a][b];[a]palettegen[p];[b][p]paletteuse", "-frames:v", "1", dst)
		}
	case "webp":
		args = append(args, "-c:v", "libwebp", "-quality", strconv.Itoa(q), "-compression_level", "6")
		if extOf(src) == "gif" {
			args = append(args, "-loop", "0")
		} else {
			args = append(args, "-frames:v", "1")
		}
		args = append(args, dst)
	case "avif":
		args = append(args, "-frames:v", "1", "-c:v", "libaom-av1", "-still-picture", "1", "-crf", avifCRF(q), "-cpu-used", "6",
			"-pix_fmt", "yuv420p", "-f", "avif", dst)
	case "ico":
		args = append(args, "-frames:v", "1", "-vf", "scale=256:256:force_original_aspect_ratio=decrease", "-f", "ico", dst)
	default:
		return uerr("이 형식으로는 바꿀 수 없어요")
	}
	if err := ffImage(ctx, args...); err != nil {
		return uerr("이미지를 바꾸지 못했어요 (%s)", err.Error())
	}
	return nil
}

// ---------------------------------------------------------------- 이미지 → PDF

// pdfReady: PDF에 그대로 넣을 수 있는 JPG/PNG (돌려 찍은 사진은 바로 세워서)
func pdfReady(ctx context.Context, src, tmp string, i int) (string, error) {
	ext := extOf(src)
	if ext == "jpg" || ext == "jpeg" {
		if jpegOrientation(src) <= 1 {
			return src, nil
		}
		out := filepath.Join(tmp, fmt.Sprintf("p%03d.jpg", i))
		if err := uprightJPEG(ctx, src, out); err != nil {
			return src, nil
		}
		return out, nil
	}
	if ext == "png" {
		return src, nil
	}
	// 그 밖의 형식은 JPG(투명하면 PNG)로 바꿔서 넣어요
	toPNG := hasAlpha(ctx, src)
	ext2 := "jpg"
	if toPNG {
		ext2 = "png"
	}
	out := filepath.Join(tmp, fmt.Sprintf("p%03d.%s", i, ext2))
	if err := convertImage(ctx, src, ext2, out, Opts{ImageQuality: 92}); err != nil {
		return "", err
	}
	if ext2 == "jpg" && jpegOrientation(out) > 1 {
		up := filepath.Join(tmp, fmt.Sprintf("p%03d-up.jpg", i))
		if uprightJPEG(ctx, out, up) == nil {
			return up, nil
		}
	}
	return out, nil
}

func imagesToPDF(ctx context.Context, files []string, dst string) error {
	tmp := tempDir("pdf-")
	defer os.RemoveAll(tmp)
	ready := []string{}
	for i, f := range files {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		r, err := pdfReady(ctx, f, tmp, i)
		if err != nil {
			return uerr("%s을(를) PDF에 넣지 못했어요", filepath.Base(f))
		}
		ready = append(ready, r)
	}
	imp, err := api.Import("pos:full", types.POINTS)
	if err != nil {
		return err
	}
	if err := api.ImportImagesFile(ready, dst, imp, pdfConf()); err != nil {
		return uerr("PDF를 만들지 못했어요 (%s)", lastLine(err.Error()))
	}
	return nil
}

// ---------------------------------------------------------------- 이미지 도구

func imageTool(ctx context.Context, req Request, f string, o Opts, prog func(float64), res *Result) ([]string, error) {
	ext := extOf(f)
	outExt := ext
	if ext == "jpeg" {
		outExt = "jpg"
	}
	tmp := tempDir("imgt-")
	defer os.RemoveAll(tmp)
	switch req.To {
	case "half", "fit1920":
		w, h := imageSize(ctx, f)
		if w == 0 || h == 0 {
			return nil, uerr("이미지 크기를 읽지 못했어요")
		}
		nw, nh := w/2, h/2
		suffix := " (반 크기)"
		if req.To == "fit1920" {
			if w <= 1920 && h <= 1920 {
				return nil, skipErr{"이미 1920px보다 작은 이미지예요"}
			}
			if w >= h {
				nw, nh = 1920, h*1920/w
			} else {
				nw, nh = w*1920/h, 1920
			}
			suffix = " (1920px)"
		}
		if !hasSips() && (extOf(f) == "jpg" || extOf(f) == "jpeg") && jpegOrientation(f) >= 5 {
			nw, nh = nh, nw // ffmpeg는 사진을 먼저 세운 뒤 크기를 바꿔요
		}
		if nw < 1 || nh < 1 {
			return nil, skipErr{"너무 작은 이미지예요"}
		}
		out := outPath(req, f, suffix, outExt)
		p, err := withPart(out, func(part string) error { return resizeImage(ctx, f, part, nw, nh, o) })
		return one(p), err
	case "compress":
		out := outPath(req, f, " (압축)", outExt)
		p, err := withPart(out, func(part string) error { return compressImage(ctx, f, part, o) })
		if err == nil && fileSize(p) >= fileSize(f) {
			removeQuiet(p)
			return nil, skipErr{"이미 충분히 작아서 더 줄일 수 없었어요"}
		}
		if err == nil {
			res.Notes = append(res.Notes, fmt.Sprintf("%s: %s → %s", filepath.Base(f), humanSize(fileSize(f)), humanSize(fileSize(p))))
		}
		return one(p), err
	case "strip":
		out := outPath(req, f, " (정보 삭제)", outExt)
		p, err := withPart(out, func(part string) error { return stripImage(ctx, f, part, tmp) })
		return one(p), err
	case "rotate":
		out := outPath(req, f, " (회전)", outExt)
		p, err := withPart(out, func(part string) error {
			if hasSips() && sipsFormat[outExt] != "" {
				return sips(ctx, "-r", "90", f, "--out", part)
			}
			in, err := decodedSource(ctx, f, tmp)
			if err != nil {
				return err
			}
			if outExt == "heic" {
				return uerr("이 컴퓨터에서는 HEIC를 저장할 수 없어요")
			}
			args := []string{"-i", in, "-vf", "transpose=1", "-frames:v", "1"}
			if outExt == "jpg" {
				args = append(args, "-q:v", "2")
			}
			return ffImage(ctx, append(args, part)...)
		})
		return one(p), err
	case "qr":
		text, err := readQR(f)
		if err != nil {
			return nil, err
		}
		if res.Text != "" {
			res.Text += "\n"
		}
		res.Text += text
		copyText(res.Text)
		return nil, nil
	}
	return nil, uerr("이 이미지에는 쓸 수 없는 도구예요")
}

func imageSize(ctx context.Context, p string) (int, int) {
	if hasSips() {
		out, err := exec.CommandContext(ctx, "/usr/bin/sips", "-g", "pixelWidth", "-g", "pixelHeight", p).Output()
		if err == nil {
			var w, h int
			for _, l := range strings.Split(string(out), "\n") {
				l = strings.TrimSpace(l)
				if strings.HasPrefix(l, "pixelWidth:") {
					w, _ = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(l, "pixelWidth:")))
				}
				if strings.HasPrefix(l, "pixelHeight:") {
					h, _ = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(l, "pixelHeight:")))
				}
			}
			if w > 0 && h > 0 {
				return w, h
			}
		}
	}
	if ffmpegReadable(extOf(p)) && isExecutable(app.tools.Path("ffprobe")) {
		if pi, err := probe(ctx, p); err == nil && pi.video() != nil {
			return pi.video().Width, pi.video().Height
		}
	}
	return 0, 0
}

func resizeImage(ctx context.Context, src, dst string, w, h int, o Opts) error {
	ext := extOf(dst)
	if hasSips() && sipsFormat[ext] != "" {
		args := []string{"-z", strconv.Itoa(h), strconv.Itoa(w)}
		if ext == "jpg" || ext == "heic" {
			args = append(args, "-s", "formatOptions", strconv.Itoa(max(o.ImageQuality, 85)))
		}
		return sips(ctx, append(args, src, "--out", dst)...)
	}
	if ext == "heic" {
		return uerr("이 컴퓨터에서는 HEIC를 저장할 수 없어요")
	}
	tmp := tempDir("rs-")
	defer os.RemoveAll(tmp)
	in, err := decodedSource(ctx, src, tmp)
	if err != nil {
		return err
	}
	args := []string{"-i", in, "-vf", fmt.Sprintf("scale=%d:%d:flags=lanczos", w, h)}
	switch ext {
	case "jpg":
		args = append(args, "-frames:v", "1", "-q:v", jpegQ(max(o.ImageQuality, 85)))
	case "webp":
		args = append(args, "-frames:v", "1", "-c:v", "libwebp", "-quality", strconv.Itoa(o.ImageQuality))
	case "gif":
		args[3] = fmt.Sprintf("scale=%d:%d:flags=lanczos,split[a][b];[a]palettegen[p];[b][p]paletteuse", w, h)
		args[2] = "-filter_complex"
	default:
		args = append(args, "-frames:v", "1")
	}
	return ffImage(ctx, append(args, dst)...)
}

func compressImage(ctx context.Context, src, dst string, o Opts) error {
	ext := extOf(dst)
	q := min(o.ImageQuality, 72)
	switch ext {
	case "jpg":
		if hasSips() {
			return sips(ctx, "-s", "format", "jpeg", "-s", "formatOptions", strconv.Itoa(q), src, "--out", dst)
		}
		return ffImage(ctx, "-i", src, "-frames:v", "1", "-q:v", jpegQ(q), dst)
	case "heic":
		if hasSips() {
			return sips(ctx, "-s", "format", "heic", "-s", "formatOptions", strconv.Itoa(q-10), src, "--out", dst)
		}
	case "png":
		// 색을 256가지로 줄이는 방식 (pngquant와 비슷) — 눈으로는 거의 같아요
		return ffImage(ctx, "-i", src, "-filter_complex",
			"[0:v]split[a][b];[a]palettegen=max_colors=256:reserve_transparent=1:stats_mode=full[p];[b][p]paletteuse=dither=sierra2_4a:alpha_threshold=128",
			"-frames:v", "1", "-pix_fmt", "pal8", dst)
	case "webp":
		return ffImage(ctx, "-i", src, "-frames:v", "1", "-c:v", "libwebp", "-quality", strconv.Itoa(q), "-compression_level", "6", dst)
	case "tiff", "tif":
		if hasSips() {
			return sips(ctx, "-s", "format", "tiff", "-s", "formatOptions", "lzw", src, "--out", dst)
		}
		return ffImage(ctx, "-i", src, "-frames:v", "1", "-compression_algo", "deflate", dst)
	case "gif":
		return ffImage(ctx, "-i", src, "-filter_complex", "split[a][b];[a]palettegen=max_colors=128[p];[b][p]paletteuse=dither=bayer", "-loop", "0", dst)
	}
	return skipErr{strings.ToUpper(ext) + " 파일은 JPG나 PNG로 바꾸면 용량이 줄어요"}
}

func stripImage(ctx context.Context, src, dst, tmp string) error {
	switch extOf(src) {
	case "jpg", "jpeg":
		in := src
		if jpegOrientation(src) > 1 {
			// 돌려 찍은 사진은 먼저 바로 세워요 (방향 정보까지 지우니까)
			up := filepath.Join(tmp, "up.jpg")
			if err := uprightJPEG(ctx, src, up); err == nil {
				in = up
			}
		}
		return stripJPEG(in, dst)
	case "png":
		return stripPNG(src, dst)
	case "heic", "heif":
		if !helperAvailable() {
			return uerr("이 컴퓨터에서는 HEIC 정보를 지울 수 없어요")
		}
		return helperSimple("heicstrip", map[string]any{"in": src, "out": dst}, 2*time.Minute)
	}
	// 그 밖의 형식: 다시 저장하면서 정보를 빼요
	in, err := decodedSource(ctx, src, tmp)
	if err != nil {
		return err
	}
	args := []string{"-i", in, "-map_metadata", "-1", "-frames:v", "1"}
	switch extOf(dst) {
	case "webp":
		args = append(args, "-c:v", "libwebp", "-lossless", "1")
	case "tif", "tiff":
		args = append(args, "-compression_algo", "lzw")
	}
	return ffImage(ctx, append(args, dst)...)
}

// uprightJPEG: EXIF 방향대로 실제 그림을 돌려서 저장
func uprightJPEG(ctx context.Context, src, dst string) error {
	vf := map[int]string{2: "hflip", 3: "hflip,vflip", 4: "vflip", 5: "transpose=0", 6: "transpose=1", 7: "transpose=3", 8: "transpose=2"}[jpegOrientation(src)]
	if vf == "" {
		return copyFile(src, dst)
	}
	if !isExecutable(app.tools.Path("ffmpeg")) {
		return uerr("변환 도구가 아직 없어요")
	}
	// -noautorotate: ffmpeg가 따로 돌리지 않게
	return ffImage(ctx, "-noautorotate", "-i", src, "-vf", vf, "-frames:v", "1", "-q:v", "2", "-map_metadata", "-1", dst)
}

func readQR(f string) (string, error) {
	if !helperAvailable() {
		return "", uerr("이 컴퓨터에서는 QR 코드를 읽을 수 없어요")
	}
	var text []string
	err := runHelper("qr", map[string]any{"in": f}, 60*time.Second, func(h helperLine) {
		if h.Text != "" {
			text = append(text, h.Text)
		}
	})
	if err != nil && len(text) == 0 {
		return "", uerr("QR 코드를 읽지 못했어요")
	}
	if len(text) == 0 {
		return "", skipErr{"QR 코드를 찾지 못했어요"}
	}
	return strings.Join(text, "\n"), nil
}
