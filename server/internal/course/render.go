package course

import (
	"bytes"
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"

	"github.com/jobshout/server/internal/model"
)

// lessonConverter renders chapter markdown to lesson HTML.
//
// goldmark is used WITHOUT html.WithUnsafe: raw HTML in the model's markdown
// is dropped and javascript:/vbscript:/data: link targets are neutralised.
// Workstation Academy sanitises lesson HTML too, and OpsAPI does not, so this
// is the layer that must not let markup through.
var lessonConverter = goldmark.New(goldmark.WithExtensions(extension.GFM))

var leadingH1 = regexp.MustCompile(`\A\s*#\s[^\n]*\n+`)

// renderLesson converts chapter markdown plus its images to lesson HTML. The
// leading H1 is dropped because the Academy shows the lesson title itself.
// The quiz is deliberately never rendered here: answers stay in JobShout.
func renderLesson(markdown string, images []model.CourseImage) (string, error) {
	var buf bytes.Buffer
	if err := lessonConverter.Convert([]byte(leadingH1.ReplaceAllString(markdown, "")), &buf); err != nil {
		return "", fmt.Errorf("course: render markdown: %w", err)
	}
	for _, img := range images {
		if !safeImageURL(img.URL) {
			continue
		}
		buf.WriteString(`<figure><img src="`)
		buf.WriteString(html.EscapeString(img.URL))
		buf.WriteString(`" alt="`)
		buf.WriteString(html.EscapeString(img.Alt))
		buf.WriteString(`" />`)
		if img.Caption != "" {
			buf.WriteString(`<figcaption>`)
			buf.WriteString(html.EscapeString(img.Caption))
			buf.WriteString(`</figcaption>`)
		}
		buf.WriteString("</figure>\n")
	}
	return strings.TrimSpace(buf.String()), nil
}

// safeImageURL allows only https URLs and the image service's own relative
// path. Anything else (data:, javascript:, http:) is not embedded.
func safeImageURL(u string) bool {
	u = strings.TrimSpace(u)
	return strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "/api/v1/images/file/")
}
