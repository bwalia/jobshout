package career

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"
)

const maxCVUploadBytes = 5 << 20

// ExtractCVMarkdown turns an uploaded PDF CV into text. Other types are rejected.
func ExtractCVMarkdown(filename, contentType string, data []byte) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("career: empty file")
	}
	if len(data) > maxCVUploadBytes {
		return "", fmt.Errorf("career: file is larger than 5MB")
	}
	ext := strings.ToLower(filepath.Ext(filename))
	ct := strings.ToLower(contentType)
	head := data
	if len(head) > 8 {
		head = head[:8]
	}
	if ext != ".pdf" && !strings.Contains(ct, "application/pdf") && !bytes.Contains(head, []byte("%PDF")) {
		return "", fmt.Errorf("career: upload a PDF")
	}
	return extractPDF(data)
}

func extractPDF(data []byte) (string, error) {
	if !bytes.Contains(data, []byte("%PDF")) {
		return "", fmt.Errorf("career: not a PDF")
	}
	// Prefer pdftotext when available: it understands ToUnicode CMaps and keeps
	// layout. Many designer-exported CVs (Google Docs, Word, Affinity) store
	// glyphs as hex strings that the naive stream scrape cannot read.
	if t := strings.TrimSpace(extractViaPdftotext(data)); t != "" {
		return t, nil
	}
	// Pure-Go reader: works in containers without poppler and on hosts where
	// Homebrew is not on PATH (launchd, minimal CI images).
	if t := strings.TrimSpace(extractViaPDFLib(data)); t != "" {
		return t, nil
	}
	if t := strings.TrimSpace(extractPDFContentStreams(data)); t != "" {
		return t, nil
	}
	return "", fmt.Errorf("career: could not read text from that PDF")
}

func extractViaPdftotext(data []byte) string {
	exe := findPdftotext()
	if exe == "" {
		return ""
	}
	dir, err := os.MkdirTemp("", "career-cv-")
	if err != nil {
		return ""
	}
	defer os.RemoveAll(dir)
	in := filepath.Join(dir, "cv.pdf")
	if err := os.WriteFile(in, data, 0o600); err != nil {
		return ""
	}
	out, err := exec.Command(exe, "-layout", "-enc", "UTF-8", "-nopgbrk", in, "-").Output()
	if err != nil || !utf8.Valid(out) {
		return ""
	}
	return string(out)
}

func findPdftotext() string {
	if exe, err := exec.LookPath("pdftotext"); err == nil {
		return exe
	}
	// launchd and some container entrypoints ship a PATH with no Homebrew.
	for _, p := range []string{
		"/opt/homebrew/bin/pdftotext",
		"/usr/local/bin/pdftotext",
		"/usr/bin/pdftotext",
	} {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

func extractViaPDFLib(data []byte) string {
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return ""
	}
	var b strings.Builder
	for i := 1; i <= r.NumPage(); i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		b.WriteString(text)
		if text != "" && !strings.HasSuffix(text, "\n") {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

var pdfLengthRe = regexp.MustCompile(`/Length\s+(\d+)`)

func extractPDFContentStreams(data []byte) string {
	var parts []string
	i := 0
	for i < len(data) {
		rel := bytes.Index(data[i:], []byte("stream"))
		if rel < 0 {
			break
		}
		abs := i + rel
		if !pdfKeywordAt(data, abs, len("stream")) {
			i = abs + 6
			continue
		}
		after := abs + 6
		if after >= len(data) {
			break
		}
		if after < len(data) && data[after] == '\r' {
			after++
		}
		if after < len(data) && data[after] == '\n' {
			after++
		}
		windowStart := abs - 500
		if windowStart < 0 {
			windowStart = 0
		}
		dict := data[windowStart:abs]
		m := pdfLengthRe.FindSubmatch(dict)
		if m == nil {
			i = abs + 6
			continue
		}
		n, err := strconv.Atoi(string(m[1]))
		if err != nil || n <= 0 || after+n > len(data) {
			i = abs + 6
			continue
		}
		payload := data[after : after+n]
		if bytes.Contains(dict, []byte("/FlateDecode")) {
			zr, zerr := zlib.NewReader(bytes.NewReader(payload))
			if zerr != nil {
				i = abs + 6
				continue
			}
			out, rerr := io.ReadAll(zr)
			_ = zr.Close()
			if rerr != nil {
				i = abs + 6
				continue
			}
			payload = out
		}
		if t := pdfTextOps(payload); t != "" {
			parts = append(parts, t)
		}
		i = after + n
	}
	return strings.Join(parts, "\n")
}

func pdfKeywordAt(data []byte, abs, keyLen int) bool {
	if abs > 0 {
		c := data[abs-1]
		if c != ' ' && c != '\n' && c != '\r' && c != '\t' && c != '>' {
			return false
		}
	}
	end := abs + keyLen
	if end < len(data) {
		c := data[end]
		if c != ' ' && c != '\n' && c != '\r' && c != '\t' && c != '/' && c != '<' {
			return false
		}
	}
	return true
}

var (
	pdfTjRe      = regexp.MustCompile(`\(((?:\\.|[^\\)])*)\)\s*Tj`)
	pdfTJBlockRe = regexp.MustCompile(`\[(.*?)\]\s*TJ`)
	pdfTJTokRe   = regexp.MustCompile(`\(((?:\\.|[^\\)])*)\)|(-?\d+(?:\.\d+)?)`)
)

func pdfTextOps(payload []byte) string {
	var b strings.Builder
	for _, m := range pdfTjRe.FindAllSubmatch(payload, -1) {
		if s := pdfUnescape(m[1]); s != "" {
			b.WriteString(s)
			b.WriteByte('\n')
		}
	}
	for _, m := range pdfTJBlockRe.FindAllSubmatch(payload, -1) {
		if s := pdfTJText(m[1]); s != "" {
			b.WriteString(s)
			b.WriteByte('\n')
		}
	}
	return strings.TrimSpace(b.String())
}

func pdfTJText(inner []byte) string {
	var b strings.Builder
	needSpace := false
	sawLit := false
	for _, m := range pdfTJTokRe.FindAllSubmatch(inner, -1) {
		if len(m[1]) > 0 || (m[0][0] == '(') {
			if sawLit && needSpace {
				b.WriteByte(' ')
			}
			b.WriteString(pdfUnescape(m[1]))
			sawLit = true
			needSpace = false
			continue
		}
		if len(m[2]) == 0 {
			continue
		}
		adj, err := strconv.ParseFloat(string(m[2]), 64)
		if err == nil && adj <= -80 {
			needSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

func pdfUnescape(raw []byte) string {
	s := string(raw)
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = strings.ReplaceAll(s, `\r`, "")
	s = strings.ReplaceAll(s, `\t`, " ")
	s = strings.ReplaceAll(s, `\(`, "(")
	s = strings.ReplaceAll(s, `\)`, ")")
	s = strings.ReplaceAll(s, `\\`, `\`)
	return s
}
