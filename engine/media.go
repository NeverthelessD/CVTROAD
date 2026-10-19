package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// ---------------------------------------------------------------- ffprobe

type probeStream struct {
	Index       int               `json:"index"`
	CodecType   string            `json:"codec_type"`
	CodecName   string            `json:"codec_name"`
	Width       int               `json:"width"`
	Height      int               `json:"height"`
	PixFmt      string            `json:"pix_fmt"`
	SampleFmt   string            `json:"sample_fmt"`
	BitsRaw     string            `json:"bits_per_raw_sample"`
	BitRate     string            `json:"bit_rate"`
	Channels    int               `json:"channels"`
	Disposition map[string]int    `json:"disposition"`
	Tags        map[string]string `json:"tags"`
	SideData    []map[string]any  `json:"side_data_list"`
	NbFrames    string            `json:"nb_frames"`
}

type probeInfo struct {
	Streams []probeStream `json:"streams"`
	Format  struct {
		Duration   string `json:"duration"`
		BitRate    string `json:"bit_rate"`
		FormatName string `json:"format_name"`
	} `json:"format"`
}

func probe(ctx context.Context, p string) (*probeInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, app.tools.Path("ffprobe"), "-v", "error", "-print_format", "json", "-show_streams", "-show_format", p)
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil, uerr("파일을 읽지 못했어요. 손상됐거나 지원하지 않는 형식이에요")
	}
	var pi probeInfo
	if err := json.Unmarshal(out, &pi); err != nil {
		return nil, uerr("파일 정보를 읽지 못했어요")
	}
	return &pi, nil
}

func (pi *probeInfo) duration() float64 {
	d, _ := strconv.ParseFloat(pi.Format.Duration, 64)
	return d
}

// video: 진짜 영상 (앨범 표지 같은 그림 한 장은 빼요)
func (pi *probeInfo) video() *probeStream {
	for i := range pi.Streams {
		s := &pi.Streams[i]
		if s.CodecType == "video" && s.Disposition["attached_pic"] == 0 {
			return s
		}
	}
	return nil
}

func (pi *probeInfo) cover() *probeStream {
	for i := range pi.Streams {
		s := &pi.Streams[i]
		if s.CodecType == "video" && s.Disposition["attached_pic"] == 1 {
			return s
		}
	}
	return nil
}

func (pi *probeInfo) audio() *probeStream {
	for i := range pi.Streams {
		if pi.Streams[i].CodecType == "audio" {
			return &pi.Streams[i]
		}
	}
	return nil
}

func (s *probeStream) rotation() int {
	if s == nil {
		return 0
	}
	if r, ok := s.Tags["rotate"]; ok {
		n, _ := strconv.Atoi(r)
		return n
	}
	for _, sd := range s.SideData {
		if v, ok := sd["rotation"]; ok {
			switch x := v.(type) {
			case float64:
				return int(x)
			case string:
				n, _ := strconv.Atoi(x)
				return n
			}
		}
	}
	return 0
}

// displaySize: 회전까지 생각한 화면 크기
func (s *probeStream) displaySize() (int, int) {
	if s == nil {
		return 0, 0
	}
	r := s.rotation()
	if r < 0 {
		r = -r
	}
	if r%180 == 90 {
		return s.Height, s.Width
	}
	return s.Width, s.Height
}

func (s *probeStream) bits() int {
	if s == nil {
		return 16
	}
	if n, _ := strconv.Atoi(s.BitsRaw); n > 0 {
		return n
	}
	if strings.HasPrefix(s.SampleFmt, "s32") || strings.HasPrefix(s.SampleFmt, "flt") || strings.HasPrefix(s.SampleFmt, "dbl") {
		if strings.Contains(s.CodecName, "24") {
			return 24
		}
		if strings.HasPrefix(s.CodecName, "pcm_s32") || strings.HasPrefix(s.CodecName, "pcm_f") {
			return 24
		}
	}
	if strings.Contains(s.CodecName, "24") {
		return 24
	}
	return 16
}

// ---------------------------------------------------------------- ffmpeg 실행 (진행률)

func ffmpegRun(ctx context.Context, args []string, dur float64, prog func(float64)) error {
	full := append([]string{"-hide_banner", "-nostdin", "-y", "-v", "error", "-progress", "pipe:1", "-nostats"}, args...)
	cmd := exec.CommandContext(ctx, app.tools.Path("ffmpeg"), full...)
	hideWindow(cmd)
	setProcessGroup(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var errBuf strings.Builder
	cmd.Stderr = &limitedWriter{w: &errBuf, n: 8000}
	if err := cmd.Start(); err != nil {
		return uerr("변환 도구를 실행하지 못했어요")
	}
	stop := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			killProcessGroup(cmd)
		case <-stop:
		}
	}()
	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		line := sc.Text()
		if prog == nil || dur <= 0 {
			continue
		}
		if strings.HasPrefix(line, "out_time_us=") || strings.HasPrefix(line, "out_time_ms=") {
			v, err := strconv.ParseFloat(line[strings.Index(line, "=")+1:], 64)
			if err == nil && v > 0 {
				prog(v / 1e6 / dur)
			}
		}
	}
	err = cmd.Wait()
	close(stop)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		msg := strings.TrimSpace(errBuf.String())
		return fmt.Errorf("%s", lastLine(msg))
	}
	return nil
}

// ---------------------------------------------------------------- 영상

func crfFor(q string) (x264 int, vp9 int, preset string) {
	switch q {
	case "high":
		return 19, 28, "faster"
	case "small":
		return 28, 38, "medium"
	}
	return 23, 33, "faster"
}

// scaleFilter: 짧은 쪽을 h로 (이미 작으면 그대로)
func scaleFilter(v *probeStream, h int) string {
	if v == nil || h <= 0 {
		return ""
	}
	w0, h0 := v.displaySize()
	short := h0
	if w0 < h0 {
		short = w0
	}
	if short <= h {
		return ""
	}
	if w0 >= h0 {
		return fmt.Sprintf("scale=-2:%d", h)
	}
	return fmt.Sprintf("scale=%d:-2", h)
}

func sizeFor(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func x264Args(o Opts, v *probeStream, maxH int, crfOverride int) []string {
	crf, _, preset := crfFor(o.VideoQuality)
	if crfOverride > 0 {
		crf = crfOverride
	}
	args := []string{"-c:v", "libx264", "-crf", strconv.Itoa(crf), "-preset", preset, "-pix_fmt", "yuv420p"}
	if f := scaleFilter(v, maxH); f != "" {
		args = append(args, "-vf", f)
	}
	return args
}

func convertVideo(ctx context.Context, src, to, dst string, o Opts, prog func(float64)) (string, error) {
	pi, err := probe(ctx, src)
	if err != nil {
		return "", err
	}
	dur := pi.duration()
	v := pi.video()
	a := pi.audio()
	switch to {
	case "mp3", "m4a", "wav":
		if a == nil {
			return "", uerr("이 영상에는 소리가 없어요")
		}
		return convertAudio(ctx, src, to, dst, o, prog)
	}
	if v == nil {
		return "", uerr("영상이 들어 있지 않은 파일이에요")
	}
	maxH := sizeFor(o.VideoSize)
	resize := scaleFilter(v, maxH) != ""
	args := []string{"-i", src}
	switch to {
	case "mp4", "mov", "m4v":
		vOK := v.CodecName == "h264" || v.CodecName == "hevc"
		aOK := a == nil || a.CodecName == "aac" || a.CodecName == "mp3" || a.CodecName == "alac" || (to == "mov" && strings.HasPrefix(a.CodecName, "pcm_"))
		args = append(args, "-map", "0:v:0", "-map", "0:a:0?")
		if vOK && aOK && !resize {
			// 다시 압축하지 않고 그릇만 바꿔요 (빠르고 화질 그대로)
			args = append(args, "-c", "copy")
			if v.CodecName == "hevc" {
				args = append(args, "-tag:v", "hvc1")
			}
		} else {
			args = append(args, x264Args(o, v, maxH, 0)...)
			if a != nil {
				if aOK && a.CodecName == "aac" {
					args = append(args, "-c:a", "copy")
				} else {
					args = append(args, "-c:a", "aac", "-b:a", "192k")
				}
			}
		}
		args = append(args, "-movflags", "+faststart", "-map_metadata", "0", dst)
	case "mkv":
		if resize {
			args = append(args, "-map", "0:v:0", "-map", "0:a?", "-map", "0:s?")
			args = append(args, x264Args(o, v, maxH, 0)...)
			args = append(args, "-c:a", "copy", "-c:s", "copy", dst)
		} else {
			args = append(args, "-map", "0", "-c", "copy", dst)
		}
	case "webm":
		_, crf, _ := crfFor(o.VideoQuality)
		args = append(args, "-map", "0:v:0", "-map", "0:a:0?", "-c:v", "libvpx-vp9", "-crf", strconv.Itoa(crf), "-b:v", "0",
			"-row-mt", "1", "-deadline", "good", "-cpu-used", "4", "-pix_fmt", "yuv420p")
		if f := scaleFilter(v, maxH); f != "" {
			args = append(args, "-vf", f)
		}
		args = append(args, "-c:a", "libopus", "-b:a", "128k", dst)
	case "gif":
		note := ""
		if dur > 30 {
			args = []string{"-t", "30", "-i", src}
			dur = 30
			note = "앞 30초만 GIF로 만들었어요"
		}
		w0, _ := v.displaySize()
		width := 640
		if w0 > 0 && w0 < width {
			width = w0
		}
		args = append(args, "-filter_complex",
			fmt.Sprintf("[0:v:0]fps=12,scale=%d:-1:flags=lanczos,split[a][b];[a]palettegen=stats_mode=diff[p];[b][p]paletteuse=dither=bayer:bayer_scale=4", width),
			"-loop", "0", dst)
		return note, ffmpegRun(ctx, args, dur, prog)
	default:
		return "", uerr("이 형식으로는 바꿀 수 없어요")
	}
	return "", ffmpegRun(ctx, args, dur, prog)
}

func videoTool(ctx context.Context, req Request, f string, o Opts, prog func(float64)) ([]string, error) {
	pi, err := probe(ctx, f)
	if err != nil {
		return nil, err
	}
	v, a := pi.video(), pi.audio()
	dur := pi.duration()
	ext := extOf(f)
	if ext == "m4v" || ext == "mov" || ext == "mp4" {
		// 그대로
	}
	switch req.To {
	case "compress", "720p":
		if v == nil {
			return nil, uerr("영상이 들어 있지 않은 파일이에요")
		}
		h, crf, suffix := 1080, 28, " (압축)"
		if req.To == "720p" {
			h, crf, suffix = 720, 23, " (720p)"
			if scaleFilter(v, 720) == "" {
				return nil, skipErr{"이미 720p보다 작은 영상이에요"}
			}
		}
		out := outPath(req, f, suffix, "mp4")
		p, err := withPart(out, func(part string) error {
			args := []string{"-i", f, "-map", "0:v:0", "-map", "0:a:0?"}
			o2 := o
			o2.VideoQuality = "small"
			if req.To == "720p" {
				o2.VideoQuality = "normal"
			}
			args = append(args, x264Args(o2, v, h, crf)...)
			if a != nil {
				args = append(args, "-c:a", "aac", "-b:a", "128k")
			}
			args = append(args, "-movflags", "+faststart", part)
			return ffmpegRun(ctx, args, dur, prog)
		})
		if err == nil && req.To == "compress" && fileSize(p) >= fileSize(f) {
			removeQuiet(p)
			return nil, skipErr{"이미 충분히 작아서 더 줄일 수 없었어요"}
		}
		return one(p), err
	case "mute":
		if v == nil {
			return nil, uerr("영상이 들어 있지 않은 파일이에요")
		}
		out := outPath(req, f, " (소리 없음)", ext)
		p, err := withPart(out, func(part string) error {
			return ffmpegRun(ctx, []string{"-i", f, "-map", "0:v", "-c", "copy", "-an", part}, dur, prog)
		})
		return one(p), err
	case "audio":
		if a == nil {
			return nil, uerr("이 영상에는 소리가 없어요")
		}
		aext, copyOK := "m4a", a.CodecName == "aac" || a.CodecName == "alac"
		switch a.CodecName {
		case "mp3":
			aext, copyOK = "mp3", true
		case "opus":
			aext, copyOK = "opus", true
		case "vorbis":
			aext, copyOK = "ogg", true
		case "flac":
			aext, copyOK = "flac", true
		}
		out := outPath(req, f, "", aext)
		p, err := withPart(out, func(part string) error {
			args := []string{"-i", f, "-map", "0:a:0", "-vn"}
			if copyOK {
				args = append(args, "-c:a", "copy")
			} else {
				args = append(args, "-c:a", aacEncoder(), "-b:a", "256k")
			}
			return ffmpegRun(ctx, append(args, part), dur, prog)
		})
		return one(p), err
	case "strip":
		out := outPath(req, f, " (정보 삭제)", ext)
		p, err := withPart(out, func(part string) error {
			args := []string{"-i", f, "-map", "0:v?", "-map", "0:a?", "-c", "copy",
				"-map_metadata", "-1", "-map_metadata:s:v", "-1", "-map_metadata:s:a", "-1", "-map_chapters", "-1", "-fflags", "+bitexact"}
			if ext == "mp4" || ext == "mov" || ext == "m4v" {
				args = append(args, "-movflags", "+faststart")
			}
			return ffmpegRun(ctx, append(args, part), dur, prog)
		})
		return one(p), err
	}
	return nil, uerr("이 영상에는 쓸 수 없는 도구예요")
}

// ---------------------------------------------------------------- 오디오

func aacEncoder() string {
	if runtime.GOOS == "darwin" {
		return "aac_at" // 맥에 들어 있는 고음질 AAC
	}
	return "aac"
}

func kbps(q string) int {
	switch q {
	case "128":
		return 128
	case "192":
		return 192
	}
	return 320
}

// audioCodecArgs: 형식별 인코더 설정
func audioCodecArgs(to string, q string, a *probeStream) []string {
	b := kbps(q)
	switch to {
	case "mp3":
		return []string{"-c:a", "libmp3lame", "-b:a", fmt.Sprintf("%dk", b)}
	case "m4a":
		if q == "lossless" {
			return []string{"-c:a", "alac"}
		}
		return []string{"-c:a", aacEncoder(), "-b:a", fmt.Sprintf("%dk", b)}
	case "m4r":
		return []string{"-c:a", aacEncoder(), "-b:a", "256k"}
	case "wav":
		if a.bits() >= 24 {
			return []string{"-c:a", "pcm_s24le"}
		}
		return []string{"-c:a", "pcm_s16le"}
	case "aiff":
		if a.bits() >= 24 {
			return []string{"-c:a", "pcm_s24be"}
		}
		return []string{"-c:a", "pcm_s16be"}
	case "flac":
		args := []string{"-c:a", "flac", "-compression_level", "8"}
		if a.bits() >= 24 {
			args = append(args, "-sample_fmt", "s32")
		} else {
			args = append(args, "-sample_fmt", "s16")
		}
		return args
	case "ogg":
		qv := map[int]string{128: "4", 192: "6", 320: "9"}[b]
		if q == "lossless" {
			qv = "10"
		}
		return []string{"-c:a", "libvorbis", "-q:a", qv}
	case "opus":
		ob := map[int]int{128: 128, 192: 160, 320: 256}[b]
		if q == "lossless" {
			ob = 320
		}
		return []string{"-c:a", "libopus", "-b:a", fmt.Sprintf("%dk", ob)}
	}
	return []string{"-c:a", aacEncoder(), "-b:a", fmt.Sprintf("%dk", b)}
}

func coverOK(to string) bool { return to == "mp3" || to == "m4a" || to == "flac" }

func convertAudio(ctx context.Context, src, to, dst string, o Opts, prog func(float64)) (string, error) {
	pi, err := probe(ctx, src)
	if err != nil {
		return "", err
	}
	a := pi.audio()
	if a == nil {
		return "", uerr("소리가 들어 있지 않은 파일이에요")
	}
	dur := pi.duration()
	note := ""
	build := func(withCover bool) []string {
		args := []string{}
		if to == "m4r" && dur > 40 {
			args = append(args, "-t", "40")
		}
		args = append(args, "-i", src, "-map", "0:a:0")
		cv := pi.cover()
		if withCover && cv != nil && coverOK(to) && kindOf(src) == "audio" {
			args = append(args, "-map", fmt.Sprintf("0:%d", cv.Index), "-c:v", "copy", "-disposition:v:0", "attached_pic")
			if to == "mp3" {
				args = append(args, "-id3v2_version", "3")
			}
		} else {
			args = append(args, "-vn")
		}
		args = append(args, audioCodecArgs(to, o.AudioQuality, a)...)
		args = append(args, "-map_metadata", "0")
		if to == "m4r" {
			args = append(args, "-f", "ipod")
		}
		return append(args, dst)
	}
	if to == "m4r" && dur > 40 {
		note = "아이폰 벨소리는 40초까지라서 앞 40초만 담았어요"
		dur = 40
	}
	if to == "mp3" && o.AudioQuality == "lossless" {
		note = "MP3에는 무손실이 없어서 가장 좋은 음질(320kbps)로 만들었어요"
	}
	err = ffmpegRun(ctx, build(true), dur, prog)
	if err != nil && ctx.Err() == nil && pi.cover() != nil {
		err = ffmpegRun(ctx, build(false), dur, prog) // 앨범 표지 때문에 실패했다면 표지 없이
	}
	return note, err
}

// 같은 형식으로 다시 저장할 때 쓰는 형식 (지원하지 않는 형식이면 m4a)
func sameAudioTarget(f string) string {
	switch e := extOf(f); e {
	case "mp3", "m4a", "wav", "flac", "ogg", "opus":
		return e
	case "aif", "aiff":
		return "aiff"
	}
	return "m4a"
}

func audioTool(ctx context.Context, req Request, f string, o Opts, prog func(float64)) ([]string, error) {
	pi, err := probe(ctx, f)
	if err != nil {
		return nil, err
	}
	a := pi.audio()
	if a == nil {
		return nil, uerr("소리가 들어 있지 않은 파일이에요")
	}
	dur := pi.duration()
	to := sameAudioTarget(f)
	q := o.AudioQuality
	if a.CodecName == "alac" || a.CodecName == "flac" || strings.HasPrefix(a.CodecName, "pcm_") {
		q = "lossless"
	} else if br, _ := strconv.Atoi(a.BitRate); br > 0 {
		// 원래 음질보다 높게 만들 필요는 없어요
		switch {
		case br <= 140000:
			q = "128"
		case br <= 210000:
			q = "192"
		default:
			q = "320"
		}
	}
	reencode := func(suffix, filter string) ([]string, error) {
		out := outPath(req, f, suffix, extFor(to))
		p, err := withPart(out, func(part string) error {
			args := []string{"-i", f, "-map", "0:a:0", "-vn", "-af", filter}
			args = append(args, audioCodecArgs(to, q, a)...)
			args = append(args, "-map_metadata", "0", part)
			return ffmpegRun(ctx, args, dur, prog)
		})
		return one(p), err
	}
	switch req.To {
	case "compress":
		out := outPath(req, f, " (압축)", "mp3")
		p, err := withPart(out, func(part string) error {
			args := []string{"-i", f, "-map", "0:a:0", "-vn", "-c:a", "libmp3lame", "-b:a", "128k", "-map_metadata", "0", part}
			return ffmpegRun(ctx, args, dur, prog)
		})
		if err == nil && fileSize(p) >= fileSize(f) {
			removeQuiet(p)
			return nil, skipErr{"이미 충분히 작아서 더 줄일 수 없었어요"}
		}
		return one(p), err
	case "loud":
		return reencode(" (음량 맞춤)", "loudnorm=I=-14:TP=-1.5:LRA=11")
	case "trim":
		return reencode(" (무음 자름)", "silenceremove=start_periods=1:start_threshold=-50dB:start_silence=0.05,areverse,silenceremove=start_periods=1:start_threshold=-50dB:start_silence=0.05,areverse")
	case "strip":
		ext := extOf(f)
		out := outPath(req, f, " (정보 삭제)", ext)
		p, err := withPart(out, func(part string) error {
			args := []string{"-i", f, "-map", "0:a", "-c", "copy", "-map_metadata", "-1", "-map_metadata:s:a", "-1", "-fflags", "+bitexact"}
			if ext == "m4r" {
				args = append(args, "-f", "ipod")
			}
			return ffmpegRun(ctx, append(args, part), dur, prog)
		})
		return one(p), err
	}
	return nil, uerr("이 파일에는 쓸 수 없는 도구예요")
}

func removeQuiet(p string) {
	if p != "" {
		os.RemoveAll(p)
	}
}
