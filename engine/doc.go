package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"os"
	"regexp"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// 문서: 맥에 들어 있는 textutil(텍스트 편집기와 같은 엔진)로 docx · doc · rtf · odt · html · txt를 서로 바꾸고,
// PDF는 helper.js(맥의 인쇄 기능)로 만들어요. 맥이 아니면 글자만 옮기는 방식으로 바꿔요.

func hasTextutil() bool {
	return runtime.GOOS == "darwin" && os.Getenv("CVTROAD_NO_TEXTUTIL") == "" && isExecutable("/usr/bin/textutil")
}

var textutilOut = map[string]string{"txt": "txt", "html": "html", "rtf": "rtf", "doc": "doc", "docx": "docx", "odt": "odt"}

func convertDoc(ctx context.Context, src, to, dst string) error {
	ext := extOf(src)
	if to == "pdf" {
		if !helperAvailable() {
			return uerr("이 컴퓨터에서는 문서를 PDF로 바꿀 수 없어요")
		}
		in := src
		if ext == "md" || ext == "txt" {
			// 한국어 옛 인코딩(CP949) 글도 깨지지 않게 UTF-8로 맞춰서
			t, err := readPlainText(src)
			if err != nil {
				return err
			}
			tmp := tempDir("doc-")
			defer os.RemoveAll(tmp)
			in = tmp + "/doc.txt"
			os.WriteFile(in, []byte(t), 0o644)
		}
		var herr string
		err := runHelper("docpdf", map[string]any{"in": in, "out": dst}, 3*time.Minute, func(h helperLine) {
			if h.Error != "" {
				herr = h.Error
			}
		})
		if herr != "" {
			return uerr("%s", herr)
		}
		if err != nil {
			return uerr("PDF로 바꾸지 못했어요")
		}
		return nil
	}
	if hasTextutil() {
		f := textutilOut[to]
		if f == "" {
			return uerr("이 형식으로는 바꿀 수 없어요")
		}
		in := src
		args := []string{"-convert", f, "-output", dst}
		if ext == "md" || ext == "txt" {
			t, err := readPlainText(src)
			if err != nil {
				return err
			}
			tmp := tempDir("doc-")
			defer os.RemoveAll(tmp)
			in = tmp + "/doc.txt"
			os.WriteFile(in, []byte(t), 0o644)
			args = append(args, "-inputencoding", "UTF-8", "-format", "txt")
		}
		if to == "txt" || to == "html" {
			args = append(args, "-encoding", "UTF-8")
		}
		args = append(args, in)
		if err := runCmd(ctx, 2*time.Minute, "/usr/bin/textutil", args...); err != nil {
			return uerr("문서를 바꾸지 못했어요 (%s)", err.Error())
		}
		return nil
	}
	// 맥이 아닐 때: 글자만 옮겨요
	text, err := readDocText(src)
	if err != nil {
		return err
	}
	return writeTextAs(text, to, dst)
}

// readPlainText reads a text file as UTF-8 (UTF-16 · CP949도 알아서).
func readPlainText(p string) (string, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	return decodeText(b), nil
}

func decodeText(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}):
		return string(b[3:])
	case bytes.HasPrefix(b, []byte{0xFF, 0xFE}):
		s, _, _ := transform.Bytes(unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder(), b)
		return string(s)
	case bytes.HasPrefix(b, []byte{0xFE, 0xFF}):
		s, _, _ := transform.Bytes(unicode.UTF16(unicode.BigEndian, unicode.UseBOM).NewDecoder(), b)
		return string(s)
	}
	if utf8.Valid(b) {
		return string(b)
	}
	if s, _, err := transform.Bytes(korean.EUCKR.NewDecoder(), b); err == nil {
		return string(s)
	}
	return strings.ToValidUTF8(string(b), "?")
}

var tagRe = regexp.MustCompile(`(?s)<[^>]*>`)
var scriptRe = regexp.MustCompile(`(?is)<(script|style)[^>]*>.*?</(script|style)>`)
var blockRe = regexp.MustCompile(`(?i)<(br|/p|/div|/h[1-6]|/li|/tr)[^>]*>`)

func readDocText(p string) (string, error) {
	switch extOf(p) {
	case "txt", "md":
		return readPlainText(p)
	case "html", "htm":
		t, _ := readPlainText(p)
		t = scriptRe.ReplaceAllString(t, "")
		t = blockRe.ReplaceAllString(t, "\n")
		return html.UnescapeString(tagRe.ReplaceAllString(t, "")), nil
	case "docx", "odt":
		zr, err := zip.OpenReader(p)
		if err != nil {
			return "", uerr("문서를 열지 못했어요")
		}
		defer zr.Close()
		name := "word/document.xml"
		para := "p"
		if extOf(p) == "odt" {
			name, para = "content.xml", "p"
		}
		for _, f := range zr.File {
			if f.Name == name {
				rc, _ := f.Open()
				defer rc.Close()
				return xmlText(rc, para), nil
			}
		}
		return "", uerr("문서 내용을 찾지 못했어요")
	case "rtf":
		b, _ := os.ReadFile(p)
		return rtfText(string(b)), nil
	}
	return "", uerr("이 컴퓨터에서는 %s 문서를 바꿀 수 없어요", strings.ToUpper(extOf(p)))
}

func xmlText(r io.Reader, para string) string {
	d := xml.NewDecoder(r)
	var sb strings.Builder
	for {
		tok, err := d.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.CharData:
			sb.Write(t)
		case xml.StartElement:
			if t.Name.Local == "tab" {
				sb.WriteString("\t")
			} else if t.Name.Local == "br" || t.Name.Local == "line-break" {
				sb.WriteString("\n")
			}
		case xml.EndElement:
			if t.Name.Local == para || t.Name.Local == "h" {
				sb.WriteString("\n")
			}
		}
	}
	return sb.String()
}

var rtfCtl = regexp.MustCompile(`\\u(-?\d+)\??|\\([a-z]+)(-?\d+)? ?|\\'([0-9a-f]{2})|[{}]|\\[^a-z]`)

// rtfSkipGroups removes header groups (글꼴표 · 색상표 · 문서 정보 등) that are not text.
func rtfSkipGroups(s string) string {
	var sb strings.Builder
	depth, skipAt := 0, -1
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' && i+1 < len(s) && (s[i+1] == '{' || s[i+1] == '}' || s[i+1] == '\\') {
			if skipAt < 0 {
				sb.WriteString(s[i : i+2])
			}
			i++
			continue
		}
		if c == '{' {
			depth++
			if skipAt < 0 {
				rest := s[i+1:]
				for _, d := range []string{`\fonttbl`, `\colortbl`, `\stylesheet`, `\info`, `\*`, `\pict`, `\expandedcolortbl`, `\listtable`} {
					if strings.HasPrefix(rest, d) {
						skipAt = depth
						break
					}
				}
			}
		}
		if skipAt < 0 {
			sb.WriteByte(c)
		}
		if c == '}' {
			if skipAt == depth {
				skipAt = -1
			}
			depth--
		}
	}
	return sb.String()
}

func rtfText(s string) string {
	s = rtfSkipGroups(s)
	var sb strings.Builder
	last := 0
	for _, m := range rtfCtl.FindAllStringSubmatchIndex(s, -1) {
		sb.WriteString(s[last:m[0]])
		last = m[1]
		tok := s[m[0]:m[1]]
		switch {
		case m[2] >= 0:
			var n int
			fmt.Sscan(s[m[2]:m[3]], &n)
			if n < 0 {
				n += 65536
			}
			sb.WriteRune(rune(n))
		case m[4] >= 0:
			w := s[m[4]:m[5]]
			if w == "par" || w == "line" {
				sb.WriteString("\n")
			} else if w == "tab" {
				sb.WriteString("\t")
			}
		case strings.HasPrefix(tok, `\\`) || tok == `\{` || tok == `\}`:
			sb.WriteString(tok[1:])
		}
	}
	sb.WriteString(s[last:])
	return strings.TrimSpace(sb.String())
}

// writeTextAs writes plain text as txt · docx · rtf · html (글자만).
func writeTextAs(text, to, dst string) error {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	switch to {
	case "txt":
		return os.WriteFile(dst, []byte(text), 0o644)
	case "html":
		var sb strings.Builder
		sb.WriteString("<!doctype html>\n<meta charset=\"utf-8\">\n<body>\n")
		for _, p := range strings.Split(text, "\n") {
			sb.WriteString("<p>" + html.EscapeString(p) + "</p>\n")
		}
		sb.WriteString("</body>\n")
		return os.WriteFile(dst, []byte(sb.String()), 0o644)
	case "rtf":
		var sb strings.Builder
		sb.WriteString("{\\rtf1\\ansi\\ansicpg949\\deff0{\\fonttbl{\\f0 Apple SD Gothic Neo;}}\\f0\\fs24\n")
		for _, r := range text {
			switch {
			case r == '\n':
				sb.WriteString("\\par\n")
			case r == '\\' || r == '{' || r == '}':
				sb.WriteString("\\" + string(r))
			case r == '\t':
				sb.WriteString("\\tab ")
			case r < 128:
				sb.WriteRune(r)
			case r < 65536:
				n := int(r)
				if n > 32767 {
					n -= 65536
				}
				fmt.Fprintf(&sb, "\\u%d?", n)
			default:
				// 이모지 등: UTF-16 짝으로
				r -= 0x10000
				hi, lo := 0xD800+(r>>10), 0xDC00+(r&0x3FF)
				fmt.Fprintf(&sb, "\\u%d?\\u%d?", int(hi)-65536, int(lo)-65536)
			}
		}
		sb.WriteString("}\n")
		return os.WriteFile(dst, []byte(sb.String()), 0o644)
	case "docx", "doc", "odt":
		if to != "docx" {
			return uerr("이 컴퓨터에서는 %s로 바꿀 수 없어요", strings.ToUpper(to))
		}
		return writeDocx(text, dst)
	}
	return uerr("이 형식으로는 바꿀 수 없어요")
}

func writeDocx(text, dst string) error {
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)
	add := func(name, body string) {
		w, _ := zw.Create(name)
		io.WriteString(w, body)
	}
	add("[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`)
	add("_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`)
	var body strings.Builder
	for _, p := range strings.Split(text, "\n") {
		var esc bytes.Buffer
		xml.EscapeText(&esc, []byte(p))
		body.WriteString(`<w:p><w:r><w:t xml:space="preserve">` + esc.String() + `</w:t></w:r></w:p>`)
	}
	add("word/document.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`+body.String()+`<w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440"/></w:sectPr></w:body></w:document>`)
	if err := zw.Close(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
