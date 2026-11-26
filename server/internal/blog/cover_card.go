package blog

import (
	"bytes"
	_ "embed"
	"fmt"
	"image/color"
	"image/png"
	"math"
	"strconv"
	"strings"
	"sync"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
)

// cHeadline is the colour of the line set biggest on a card.
var cHeadline = rgb{0xfa, 0xcc, 0x15}

// coverCardAsk is what a headline card needs from the plan: the line it sets
// biggest, and the numbers its chart draws. The other parts (stat, eyebrow,
// labels) are asked for with the diagram.
const coverCardAsk = `a "headline" of 2-6 words stating the article's point, which reads on from the stat when there is one (stat "$35 billion", headline "dollar baby"; stat "100%", headline "of the rent, paid gross"), and a "trend" only when the article reports how one number changed over time: 4-12 numbers in order, with "trend_from" and "trend_to" of 2-5 words saying what the first and last point are (otherwise [] and "")`

// drawCard renders a cover as a headline card: the article's figure and point
// set large, its title under them, and on the right the trend it reports or
// its steps. Every word is typeset here, so it is spelt right and needs no
// image model: ERNIE lettered the tax article's slip and calendar, the picture
// was thrown away, and the cover went out as bare boxes.
func drawCard(scene coverScene, title, accent string) ([]byte, error) {
	if err := coverFonts(); err != nil {
		return nil, fmt.Errorf("cover fonts: %w", err)
	}
	dc := gg.NewContext(int(designW), int(designH))
	p := &pen{dc: dc, accent: coverAccentColour(accent), faces: map[string]font.Face{}}
	all := strings.Join(append([]string{scene.Eyebrow, scene.Stat, scene.Headline, title, scene.TrendFrom, scene.TrendTo}, scene.Labels...), "")
	if err := cardFonts(); err == nil && hasGlyphs(cardBold, all) && hasGlyphs(cardMedium, title) {
		p.bold, p.medium = cardBold, cardMedium
	}
	dc.SetLineJoin(gg.LineJoinRound)
	dc.SetLineCap(gg.LineCapRound)

	dc.DrawRectangle(0, 0, designW, designH)
	p.fill(cInk.a(1))
	glow := gg.NewRadialGradient(1230, 470, 0, 1230, 470, 720)
	glow.AddColorStop(0, p.accent.a(0.1))
	glow.AddColorStop(1, p.accent.a(0))
	dc.SetFillStyle(glow)
	dc.DrawRectangle(0, 0, designW, designH)
	dc.Fill()
	dc.DrawRectangle(0, 0, designW, 10)
	p.fill(p.accent.a(1))
	if scene.Eyebrow != "" {
		p.spaced(scene.Eyebrow, 88, 104, 24, 4, p.accent.a(1))
	}

	// The left column, biggest first: the stat, then the headline that reads
	// on from it, then the title. With no stat the headline leads; with
	// neither, the title does.
	type part struct {
		text           string
		max, min       float64
		lines          int
		bold           bool
		c              color.Color
		lead, spaceTop float64
	}
	var parts []part
	add := func(s string, max, min float64, lines int, bold bool, c color.Color, lead, top float64) {
		if s = strings.TrimSpace(s); s != "" {
			parts = append(parts, part{s, max, min, lines, bold, c, lead, top})
		}
	}
	lead, rest, leadLines := scene.Stat, scene.Headline, 1
	if lead == "" {
		lead, rest, leadLines = scene.Headline, "", 2
	}
	if lead == "" {
		add(title, 84, 44, 4, true, cText.a(1), 1.12, 0)
	} else {
		add(lead, 150, 64, leadLines, true, cHeadline.a(1), 1.02, 0)
		add(rest, 92, 44, 2, true, cText.a(1), 1.08, 14)
		add(title, 36, 26, 3, false, cMuted.mix(cText, 0.4).a(1), 1.3, 34)
	}
	const colX, colW = 88.0, 770.0
	sizes := make([]float64, len(parts))
	wrapped := make([][]string, len(parts))
	height := 0.0
	for i, pt := range parts {
		sizes[i], wrapped[i] = p.fitLines(pt.text, colW, pt.max, pt.min, pt.lines, pt.bold)
		height += pt.spaceTop + float64(len(wrapped[i]))*sizes[i]*pt.lead
	}
	y := math.Max(170, 170+(630-height)/2)
	for i, pt := range parts {
		y += pt.spaceTop
		p.face(pt.bold, sizes[i])
		p.dc.SetColor(pt.c)
		for _, l := range wrapped[i] {
			p.dc.DrawString(l, colX, y+sizes[i]*0.8)
			y += sizes[i] * pt.lead
		}
	}

	if len(scene.Trend) >= minTrendPoints {
		p.trend(scene)
	} else {
		p.ladder(scene.Labels)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, dc.Image()); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Poppins, jobshout.com's face, sets the card: the Go fonts slash their zero,
// so "100%" read as code. These are its Latin cuts (fonts/OFL.txt).
var (
	//go:embed fonts/Poppins-Bold.ttf
	poppinsBold []byte
	//go:embed fonts/Poppins-Medium.ttf
	poppinsMedium []byte

	cardFontsOnce        sync.Once
	cardBold, cardMedium *truetype.Font
	cardFontsErr         error
)

func cardFonts() error {
	cardFontsOnce.Do(func() {
		if cardBold, cardFontsErr = truetype.Parse(poppinsBold); cardFontsErr == nil {
			cardMedium, cardFontsErr = truetype.Parse(poppinsMedium)
		}
	})
	return cardFontsErr
}

// hasGlyphs reports whether f has a glyph for every letter of s. A card whose
// words reach past Poppins' Latin cut is set in the Go fonts instead, which
// cover far more, rather than showing boxes.
func hasGlyphs(f *truetype.Font, s string) bool {
	for _, r := range s {
		if r > ' ' && f.Index(r) == 0 {
			return false
		}
	}
	return true
}

// fitLines is the largest size from max down to min at which s wraps into at
// most n lines of width w; at min it is cut to n lines.
func (p *pen) fitLines(s string, w, max, min float64, n int, bold bool) (float64, []string) {
	size := max
	for ; size > min; size -= 4 {
		p.face(bold, size)
		if lines := p.dc.WordWrap(s, w); len(lines) <= n && p.widest(lines) <= w {
			return size, lines
		}
	}
	p.face(bold, min)
	lines := p.dc.WordWrap(s, w)
	if len(lines) > n {
		lines = lines[:n]
		lines[n-1] = strings.TrimRight(lines[n-1], " ,;:") + "…"
	}
	return min, lines
}

// The chart's frame on the right of a card.
const (
	chartL, chartR = 960.0, 1512.0
	chartT, chartB = 230.0, 690.0
	minTrendPoints = 3
	maxTrendPoints = 24
)

// trend draws the article's numbers as a line rising or falling to its last
// point, which is the accent; the first and last are named.
func (p *pen) trend(s coverScene) {
	dc := p.dc
	lo, hi := s.Trend[0], s.Trend[0]
	for _, v := range s.Trend {
		lo, hi = math.Min(lo, v), math.Max(hi, v)
	}
	span := hi - lo
	if span == 0 {
		span = 1
		lo -= 0.5
	}
	n := len(s.Trend)
	pt := func(i int) (float64, float64) {
		x := chartL + (chartR-chartL)*float64(i)/float64(n-1)
		return x, chartB - (chartB-chartT)*(s.Trend[i]-lo)/span
	}

	area := gg.NewLinearGradient(0, chartT, 0, chartB)
	area.AddColorStop(0, p.accent.a(0.35))
	area.AddColorStop(1, p.accent.a(0))
	dc.MoveTo(chartL, chartB)
	for i := range s.Trend {
		dc.LineTo(pt(i))
	}
	dc.LineTo(chartR, chartB)
	dc.ClosePath()
	dc.SetFillStyle(area)
	dc.Fill()

	dc.MoveTo(chartL, chartB)
	dc.LineTo(chartR, chartB)
	p.stroke(cSlate.a(1), 2)
	for i := 0; i < n-1; i++ {
		dc.LineTo(pt(i))
	}
	p.stroke(cText.mix(cMuted, 0.3).a(1), 6)
	x0, y0 := pt(n - 2)
	x1, y1 := pt(n - 1)
	dc.MoveTo(x0, y0)
	dc.LineTo(x1, y1)
	p.stroke(p.accent.a(1), 11)

	sx, sy := pt(0)
	p.disc(sx, sy, 11, cText.a(1), cInk.a(1), 4)
	p.disc(x1, y1, 15, p.accent.a(1), cInk.a(1), 5)
	if s.TrendFrom != "" {
		ly := sy - 34
		if sy < chartT+60 {
			ly = sy + 44
		}
		p.text(s.TrendFrom, sx, ly, 26, cText.a(1), true, 0)
	}
	if s.TrendTo != "" {
		ly := y1 - 38
		if y1 < chartT+60 {
			ly = y1 + 48
		}
		p.text(s.TrendTo, chartR-24, ly, 28, p.accent.a(1), true, 1)
	}
	p.text("illustrative", chartR, chartB+36, 20, cMuted.a(1), false, 1)
}

// ladder draws the article's steps or parts down the right of a card,
// numbered, with the last one in the accent.
func (p *pen) ladder(labels []string) {
	if len(labels) > 6 {
		labels = labels[:6]
	}
	n := len(labels)
	if n == 0 {
		return
	}
	const cx, textX, textW = 990.0, 1050.0, 470.0
	gap := 120.0
	if n > 1 {
		gap = math.Min(gap, (chartB-chartT)/float64(n-1))
	}
	top := (chartT+chartB)/2 - gap*float64(n-1)/2
	if n > 1 {
		p.dc.MoveTo(cx, top)
		p.dc.LineTo(cx, top+gap*float64(n-1))
		p.stroke(cSlate2.a(1), 4)
	}
	for i, l := range labels {
		y := top + gap*float64(i)
		fillC, line, num, txt := cPanel.a(1), cSlate2.a(1), cMuted.a(1), cText.a(1)
		if i == n-1 {
			fillC, line, num, txt = p.accent.a(1), cInk.a(1), cInk.a(1), p.accent.a(1)
		}
		p.disc(cx, y, 30, fillC, line, 4)
		p.text(strconv.Itoa(i+1), cx, y, 28, num, true, 0.5)
		size, lines := p.fitLines(l, textW, 40, 24, 2, true)
		p.face(true, size)
		p.dc.SetColor(txt)
		p.dc.DrawStringWrapped(strings.Join(lines, " "), textX, y, 0, 0.5, textW, 1.15, gg.AlignLeft)
	}
}
