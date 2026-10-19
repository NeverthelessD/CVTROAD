package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// 파일 종류 · 바꿀 수 있는 형식 · 도구 목록
// 이 표 하나를 앱 화면(/api/formats)과 백그라운드 도우미(menu.json)가 함께 써요.

type Target struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Sub   string `json:"sub"`
}

type Tool struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Sub   string `json:"sub"`
	Icon  string `json:"icon"`
	Multi bool   `json:"multi,omitempty"` // 여러 파일을 하나로 (합치기 등)
	App   bool   `json:"app,omitempty"`   // 앱 창에서 이어서 해요 (페이지 정리)
}

type Kind struct {
	ID      string   `json:"id"`
	Label   string   `json:"label"`
	Color   string   `json:"color"`
	Exts    []string `json:"exts"`
	Targets []Target `json:"targets"`
	Tools   []Tool   `json:"tools"`
}

var zipTool = Tool{"zip", "ZIP으로 묶기", "압축 파일", "zip", true, false}
var stripTool = Tool{"strip", "정보 지우기", "위치·기기 정보", "eraser", false, false}

var kinds = []Kind{
	{
		ID: "image", Label: "이미지", Color: "#f28c28",
		Exts: []string{"jpg", "jpeg", "png", "heic", "heif", "webp", "avif", "tif", "tiff", "bmp", "gif", "ico", "psd", "tga",
			"jp2", "dng", "cr2", "cr3", "nef", "arw", "raf", "orf", "rw2"},
		Targets: []Target{
			{"jpg", "JPG", "사진"}, {"png", "PNG", "투명 배경"}, {"heic", "HEIC", "아이폰"}, {"webp", "WEBP", "웹용"},
			{"avif", "AVIF", "고효율"}, {"tiff", "TIFF", "인쇄용"}, {"gif", "GIF", "움짤"}, {"bmp", "BMP", "비트맵"},
			{"pdf", "PDF", "문서"}, {"ico", "ICO", "아이콘"},
		},
		Tools: []Tool{
			{"half", "크기 반으로", "가로·세로 50%", "shrink", false, false},
			{"fit1920", "1920px 맞춤", "긴 쪽 1920", "frame", false, false},
			{"compress", "용량 줄이기", "화질 조금 낮춤", "compress", false, false},
			stripTool,
			{"rotate", "오른쪽으로 회전", "90°", "rotate", false, false},
			{"pdfmerge", "하나의 PDF로", "고른 순서대로", "stack", true, false},
			{"qr", "QR 코드 읽기", "글자 복사", "qr", false, false},
			zipTool,
		},
	},
	{
		ID: "video", Label: "영상", Color: "#7b5cd6",
		Exts: []string{"mp4", "mov", "m4v", "mkv", "webm", "avi", "wmv", "flv", "mpg", "mpeg", "3gp", "ts", "mts", "m2ts"},
		Targets: []Target{
			{"mp4", "MP4", "어디서나"}, {"mov", "MOV", "애플"}, {"webm", "WEBM", "웹용"}, {"mkv", "MKV", "보관용"},
			{"gif", "GIF", "움짤"}, {"mp3", "MP3", "소리만"}, {"m4a", "M4A", "소리만"}, {"wav", "WAV", "소리만"},
		},
		Tools: []Tool{
			{"compress", "용량 줄이기", "보내기 좋게", "compress", false, false},
			{"720p", "720p로 줄이기", "HD 크기", "frame", false, false},
			{"mute", "소리 없애기", "영상만 남김", "mute", false, false},
			{"audio", "소리만 꺼내기", "원래 음질", "wave", false, false},
			stripTool,
			zipTool,
		},
	},
	{
		ID: "audio", Label: "오디오", Color: "#1aa898",
		Exts: []string{"mp3", "m4a", "aac", "wav", "aif", "aiff", "flac", "ogg", "oga", "opus", "wma", "caf", "m4r", "alac", "amr"},
		Targets: []Target{
			{"mp3", "MP3", "어디서나"}, {"m4a", "M4A", "애플"}, {"wav", "WAV", "무손실"}, {"flac", "FLAC", "무손실 압축"},
			{"aiff", "AIFF", "무손실"}, {"ogg", "OGG", "오픈 형식"}, {"opus", "OPUS", "작은 용량"}, {"m4r", "M4R", "벨소리"},
		},
		Tools: []Tool{
			{"compress", "용량 줄이기", "MP3 128kbps", "compress", false, false},
			{"loud", "음량 맞추기", "고르게", "level", false, false},
			{"trim", "앞뒤 무음 자르기", "조용한 부분", "trim", false, false},
			stripTool,
			zipTool,
		},
	},
	{
		ID: "pdf", Label: "PDF", Color: "#d23b3b",
		Exts: []string{"pdf"},
		Targets: []Target{
			{"png", "PNG", "쪽마다 그림"}, {"jpg", "JPG", "쪽마다 그림"}, {"tiff", "TIFF", "쪽마다 그림"},
			{"txt", "TXT", "글자만"}, {"docx", "DOCX", "글자만"}, {"rtf", "RTF", "글자만"},
		},
		Tools: []Tool{
			{"compress", "용량 줄이기", "그림 화질 낮춤", "compress", false, false},
			{"split", "쪽마다 나누기", "한 쪽씩", "split", false, false},
			{"merge", "하나로 합치기", "고른 순서대로", "stack", true, false},
			{"pages", "페이지 정리", "순서·회전·삭제", "pages", false, true},
			{"rotate", "오른쪽으로 회전", "모든 쪽 90°", "rotate", false, false},
			stripTool,
			zipTool,
		},
	},
	{
		ID: "doc", Label: "문서", Color: "#2563c9",
		Exts: []string{"docx", "doc", "rtf", "rtfd", "txt", "md", "html", "htm", "odt", "webarchive"},
		Targets: []Target{
			{"pdf", "PDF", "보내기 좋게"}, {"docx", "DOCX", "워드"}, {"doc", "DOC", "예전 워드"}, {"rtf", "RTF", "서식 있는 글"},
			{"odt", "ODT", "오픈 문서"}, {"html", "HTML", "웹 페이지"}, {"txt", "TXT", "글자만"},
		},
		Tools: []Tool{zipTool},
	},
	{
		ID: "archive", Label: "압축 파일", Color: "#8b5e34",
		Exts: []string{"zip", "tar", "tgz", "gz"},
		Targets: []Target{
			{"unzip", "풀기", "같은 폴더에"}, {"zip", "ZIP", "다시 묶기"}, {"tgz", "TAR.GZ", "다시 묶기"},
		},
		Tools: []Tool{{"unzip", "압축 풀기", "같은 폴더에", "unzip", false, false}},
	},
	{
		ID: "folder", Label: "폴더", Color: "#8a94a3",
		Targets: []Target{{"zip", "ZIP", "폴더째 압축"}, {"tgz", "TAR.GZ", "폴더째 압축"}},
		Tools:   []Tool{zipTool},
	},
	{
		ID: "other", Label: "기타 파일", Color: "#8a94a3",
		Targets: []Target{{"zip", "ZIP", "압축 파일"}},
		Tools:   []Tool{zipTool},
	},
}

var extKind = map[string]string{}

func init() {
	for _, k := range kinds {
		for _, e := range k.Exts {
			extKind[e] = k.ID
		}
	}
}

func kindByID(id string) *Kind {
	for i := range kinds {
		if kinds[i].ID == id {
			return &kinds[i]
		}
	}
	return &kinds[len(kinds)-1]
}

func extOf(p string) string {
	n := strings.ToLower(filepath.Base(p))
	if strings.HasSuffix(n, ".tar.gz") {
		return "tgz"
	}
	return strings.TrimPrefix(filepath.Ext(n), ".")
}

func kindOf(p string) string {
	if st, err := os.Stat(p); err == nil && st.IsDir() {
		// .app · .rtfd 같은 꾸러미는 폴더지만 하나의 파일처럼 다뤄요
		if strings.HasSuffix(strings.ToLower(p), ".rtfd") {
			return "doc"
		}
		return "folder"
	}
	if k, ok := extKind[extOf(p)]; ok {
		return k
	}
	return "other"
}

// mainKind: 여러 파일을 골랐을 때 가장 많은 종류 (같으면 표 순서)
func mainKind(paths []string) string {
	count := map[string]int{}
	for _, p := range paths {
		count[kindOf(p)]++
	}
	best, bestN := "other", -1
	for _, k := range kinds {
		if n := count[k.ID]; n > bestN && n > 0 {
			best, bestN = k.ID, n
		}
	}
	return best
}

func targetLabel(kind, id string) string {
	for _, t := range kindByID(kind).Targets {
		if t.ID == id {
			return t.Label
		}
	}
	return strings.ToUpper(id)
}

func toolByID(kind, id string) *Tool {
	for _, t := range kindByID(kind).Tools {
		if t.ID == id {
			t := t
			return &t
		}
	}
	return nil
}

// allTargetIDs: 화면의 "모두 바꾸기"에 쓰는 형식 목록
func sortedExts() []string {
	out := []string{}
	for e := range extKind {
		out = append(out, e)
	}
	sort.Strings(out)
	return out
}
