package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// 사진 속 정보(EXIF: 찍은 위치 · 기기 · 시간 등)를 그림은 그대로 둔 채 빼요.

// jpegOrientation reads the EXIF orientation (1 = 바로, 0 = 정보 없음).
func jpegOrientation(p string) int {
	f, err := os.Open(p)
	if err != nil {
		return 0
	}
	defer f.Close()
	buf := make([]byte, 256<<10)
	n, _ := io.ReadFull(f, buf)
	b := buf[:n]
	if len(b) < 4 || b[0] != 0xFF || b[1] != 0xD8 {
		return 0
	}
	i := 2
	for i+4 <= len(b) {
		if b[i] != 0xFF {
			return 0
		}
		m := b[i+1]
		if m == 0xDA || m == 0xD9 {
			return 0
		}
		l := int(b[i+2])<<8 | int(b[i+3])
		if l < 2 || i+2+l > len(b) {
			return 0
		}
		seg := b[i+4 : i+2+l]
		if m == 0xE1 && len(seg) > 14 && string(seg[:6]) == "Exif\x00\x00" {
			return exifOrientation(seg[6:])
		}
		i += 2 + l
	}
	return 0
}

func exifOrientation(t []byte) int {
	if len(t) < 8 {
		return 0
	}
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 0
	}
	off := int(bo.Uint32(t[4:8]))
	if off+2 > len(t) {
		return 0
	}
	cnt := int(bo.Uint16(t[off : off+2]))
	for k := 0; k < cnt; k++ {
		e := off + 2 + k*12
		if e+12 > len(t) {
			return 0
		}
		if bo.Uint16(t[e:e+2]) == 0x0112 {
			v := int(bo.Uint16(t[e+8 : e+10]))
			if v >= 1 && v <= 8 {
				return v
			}
			return 0
		}
	}
	return 0
}

// stripJPEG copies the JPEG without APP1(EXIF/XMP) · APP13(IPTC) · COM, keeping the colour profile (APP2 ICC).
func stripJPEG(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if len(b) < 4 || b[0] != 0xFF || b[1] != 0xD8 {
		return userErr{"JPG 파일이 아니거나 손상됐어요"}
	}
	var out bytes.Buffer
	out.Write(b[:2])
	i := 2
	for i+4 <= len(b) {
		if b[i] != 0xFF {
			return userErr{"JPG 파일을 읽지 못했어요"}
		}
		m := b[i+1]
		if m == 0xFF { // padding
			i++
			continue
		}
		if m == 0xDA { // 그림 데이터 시작: 나머지는 그대로
			out.Write(b[i:])
			return os.WriteFile(dst, out.Bytes(), 0o644)
		}
		if m == 0xD8 || (m >= 0xD0 && m <= 0xD7) || m == 0x01 {
			out.Write(b[i : i+2])
			i += 2
			continue
		}
		l := int(b[i+2])<<8 | int(b[i+3])
		if l < 2 || i+2+l > len(b) {
			return userErr{"JPG 파일을 읽지 못했어요"}
		}
		drop := m == 0xE1 || m == 0xED || m == 0xFE || (m >= 0xE3 && m <= 0xEF && m != 0xEE)
		if !drop {
			out.Write(b[i : i+2+l])
		}
		i += 2 + l
	}
	return userErr{"JPG 파일 끝이 잘려 있어요"}
}

// stripPNG drops text · time · EXIF chunks (tEXt, zTXt, iTXt, tIME, eXIf).
func stripPNG(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	sig := []byte("\x89PNG\r\n\x1a\n")
	if !bytes.HasPrefix(b, sig) {
		return userErr{"PNG 파일이 아니거나 손상됐어요"}
	}
	var out bytes.Buffer
	out.Write(sig)
	i := 8
	for i+12 <= len(b) {
		l := int(binary.BigEndian.Uint32(b[i : i+4]))
		if l < 0 || i+12+l > len(b) {
			return userErr{"PNG 파일을 읽지 못했어요"}
		}
		typ := string(b[i+4 : i+8])
		switch typ {
		case "tEXt", "zTXt", "iTXt", "tIME", "eXIf":
		default:
			out.Write(b[i : i+12+l])
		}
		i += 12 + l
		if typ == "IEND" {
			return os.WriteFile(dst, out.Bytes(), 0o644)
		}
	}
	return errors.New("png: no IEND")
}

// copyText puts text on the clipboard (QR 코드 내용 등).
func copyText(s string) {
	if s == "" {
		return
	}
	if runtime.GOOS == "darwin" {
		cmd := exec.Command("/usr/bin/pbcopy")
		cmd.Env = append(os.Environ(), "LANG=en_US.UTF-8", "LC_CTYPE=UTF-8")
		cmd.Stdin = strings.NewReader(s)
		cmd.Run()
		return
	}
	if p := os.Getenv("CVTROAD_TEST_CLIPBOARD"); p != "" {
		os.WriteFile(p, []byte(s), 0o644)
	}
}
