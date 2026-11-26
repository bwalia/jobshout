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

// cardScale is a card's pixels per design unit: 3200x1800, so its type stays
// sharp on a retina screen where the cover spans the article column.
const cardScale = 2

// coverCardAsk is what a headline card needs from the plan: the line it sets
// biggest, and the numbers its chart draws. The other parts (stat, eyebrow,
// labels) are asked for with the diagram.
const coverCardAsk = `a "headline" of 2-6 words stating the article's point, which reads on from the stat when there is one (stat "$35 billion", headline "dollar baby"; stat "100%", headline "of the rent, paid gross"), and a "trend" only when the article reports how one number changed over time: 4-12 numbers in order, with "trend_from" and "trend_to" of 2-5 words saying what the first and last point are (otherwise [] and ""), a "dek": one plain sentence of 12-22 words telling a reader what the article explains, and "notes": for each label in order, 3-8 words saying what it means in this article (stat "", labels ["Full monthly rent", "Tax deducted upfront"], notes ["Paid gross once HMRC approves you", "Agent keeps 20% for HMRC"]), and "facts": up to 3 figures a reader should take away, each {"figure": a number, rate, date, amount or form name exactly as the article writes it, "label": 2-5 words saying what it is} (e.g. {"figure": "20%", "label": "basic-rate tax withheld"}), never the stat again`

// drawCard renders a cover as a headline card: the article's figure and point
// set large, its title under them, and on the right the trend it reports or
// its steps. Every word is typeset here, so it is spelt right and needs no
// image model: ERNIE lettered the tax article's slip and calendar, the picture
// was thrown away, and the cover went out as bare boxes.
func drawCard(scene coverScene, title, accent string) ([]byte, error) {
	if err := coverFonts(); err != nil {
		return nil, fmt.Errorf("cover fonts: %w", err)
	}
	dc := gg.NewContext(int(designW*cardScale), int(designH*cardScale))
	dc.Scale(cardScale, cardScale)
	p := &pen{dc: dc, accent: coverAccentColour(accent), faces: map[string]font.Face{}, k: cardScale}
	dek := scene.Dek
	if dek == "" {
		dek = title
	}
	var notes []string
	for _, l := range scene.Labels {
		notes = append(notes, scene.note(l))
	}
	all := strings.Join(append([]string{scene.Eyebrow, scene.Stat, scene.Headline, scene.TrendFrom, scene.TrendTo}, scene.Labels...), "")
	for _, f := range scene.Facts {
		all += f.Figure + f.Label
	}
	if err := cardFonts(); err == nil && hasGlyphs(cardBold, all) && hasGlyphs(cardMedium, dek+strings.Join(notes, "")) {
		p.bold, p.medium = cardBold, cardMedium
	}
	dc.SetLineJoin(gg.LineJoinRound)
	dc.SetLineCap(gg.LineCapRound)

	dc.DrawRectangle(0, 0, designW, designH)
	p.fill(cInk.a(1))
	// Gradients are in pixels, not design units.
	glow := gg.NewRadialGradient(1230*cardScale, 470*cardScale, 0, 1230*cardScale, 470*cardScale, 720*cardScale)
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
	// on from it, then the dek saying what the article explains (its title
	// when the plan gave none). With no stat the headline leads; with
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
	// A foot of facts takes the bottom of the card: the column and the
	// chart move up and the type steps down to make room.
	colTop, colH, big, dekLines := 170.0, 630.0, 1.0, 3
	if len(scene.Facts) > 0 {
		colTop, colH, big, dekLines = 150.0, 540.0, 0.84, 2
		p.lift = 40
	}
	if lead == "" {
		add(title, 84*big, 44, 4, true, cText.a(1), 1.12, 0)
	} else {
		add(lead, 150*big, 64, leadLines, true, cHeadline.a(1), 1.02, 0)
		add(rest, 92*big, 44, 2, true, cText.a(1), 1.08, 14)
		add(dek, 36, 26, dekLines, false, cMuted.mix(cText, 0.4).a(1), 1.3, 34*big)
	}
	const colX, colW = 88.0, 770.0
	sizes := make([]float64, len(parts))
	wrapped := make([][]string, len(parts))
	height := 0.0
	for i, pt := range parts {
		sizes[i], wrapped[i] = p.fitLines(pt.text, colW, pt.max, pt.min, pt.lines, pt.bold)
		height += pt.spaceTop + float64(len(wrapped[i]))*sizes[i]*pt.lead
	}
	y := math.Max(colTop, colTop+(colH-height)/2)
	for i, pt := range parts {
		y += pt.spaceTop
		p.face(pt.bold, sizes[i])
		p.dc.SetColor(pt.c)
		for _, l := range wrapped[i] {
			p.str(l, colX, y+sizes[i]*0.8, 0, 0)
			y += sizes[i] * pt.lead
		}
	}

	switch {
	case len(scene.Trend) >= minTrendPoints:
		p.trend(scene)
	case scene.Layout == layoutVersus && len(scene.Labels) >= 2:
		p.sides(scene.Labels[:2], notes[:2])
	default:
		p.ladder(scene.Labels, notes)
	}
	p.facts(scene.Facts)

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
		if lines := p.wrap(s, w); len(lines) <= n && p.widest(lines) <= w {
			return size, lines
		}
	}
	p.face(bold, min)
	lines := p.wrap(s, w)
	if len(lines) > n {
		lines = lines[:n]
		lines[n-1] = strings.TrimRight(lines[n-1], " ,;:") + "…"
	}
	return min, lines
}

// The chart's frame on the right of a card.
const (
	chartL, chartR        = 960.0, 1512.0
	chartTop, chartBottom = 230.0, 690.0
	minTrendPoints        = 3
	maxTrendPoints        = 24
)

// trend draws the article's numbers as a line rising or falling to its last
// point, which is the accent; the first and last are named.
func (p *pen) trend(s coverScene) {
	chartT, chartB := chartTop-p.lift, chartBottom-p.lift
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

	area := gg.NewLinearGradient(0, chartT*cardScale, 0, chartB*cardScale)
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
// numbered, with the last one in the accent and each one's note under it.
func (p *pen) ladder(labels, notes []string) {
	chartT, chartB := chartTop-p.lift, chartBottom-p.lift
	if len(labels) > 6 {
		labels, notes = labels[:6], notes[:6]
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
		if notes[i] == "" {
			p.lines(l, textX, y, textW, 40, 24, 2, true, txt, 0.5)
			continue
		}
		p.lines(l, textX, y-18, textW, 36, 24, 1, true, txt, 0.5)
		p.lines(notes[i], textX, y+22, textW, 24, 18, 1, false, cMuted.mix(cText, 0.3).a(1), 0.5)
	}
}

// sides draws a versus scene's two sides as panels, one over the other with
// "vs" between: the first plain, the second in the accent, each with what it
// means under it.
func (p *pen) sides(labels, notes []string) {
	chartT, chartB := chartTop-p.lift, chartBottom-p.lift
	const x, w, h, padX = chartL, chartR - chartL, 190.0, 40.0
	mid := (chartT + chartB) / 2
	tops := []float64{mid - 40 - h, mid + 40}
	for i, l := range labels {
		y := tops[i]
		fillC, line, txt := cPanel.a(0.85), cSlate2.a(1), cText.a(1)
		if i == 1 {
			fillC, line, txt = p.accent.a(0.12), p.accent.a(1), p.accent.a(1)
		}
		p.box(x, y, w, h, 22, fillC, line, 3)
		if notes[i] == "" {
			p.lines(l, x+padX, y+h/2, w-2*padX, 48, 28, 2, true, txt, 0.5)
			continue
		}
		p.lines(l, x+padX, y+h/2-26, w-2*padX, 44, 28, 1, true, txt, 0.5)
		p.lines(notes[i], x+padX, y+h/2+30, w-2*padX, 28, 20, 1, false, cMuted.mix(cText, 0.4).a(1), 0.5)
	}
	p.disc(x+w/2, mid, 34, cInk.a(1), p.accent.a(1), 4)
	p.text("vs", x+w/2, mid-2, 28, p.accent.a(1), true, 0.5)
}

// facts sets a card's foot: each figure the article states, in the accent,
// with what it is under it, in equal cells across the card.
func (p *pen) facts(fs []coverFact) {
	if len(fs) == 0 {
		return
	}
	const x0, x1, top = 88.0, chartR, 730.0
	p.dc.MoveTo(x0, top)
	p.dc.LineTo(x1, top)
	p.stroke(cSlate.a(1), 2)
	w := (x1 - x0) / float64(len(fs))
	for i, f := range fs {
		x := x0 + w*float64(i)
		if i > 0 {
			p.dc.MoveTo(x-28, top+30)
			p.dc.LineTo(x-28, top+124)
			p.stroke(cSlate.a(1), 2)
		}
		p.lines(f.Figure, x, top+56, w-64, 52, 30, 1, true, p.accent.a(1), 0.5)
		p.lines(f.Label, x, top+106, w-64, 26, 18, 1, false, cMuted.mix(cText, 0.3).a(1), 0.5)
	}
}

// lines sets s from x in width w at the largest size from max to min that
// fits in n lines, the block centred on y when ay is 0.5.
func (p *pen) lines(s string, x, y, w, max, min float64, n int, bold bool, c color.Color, ay float64) {
	size, ls := p.fitLines(s, w, max, min, n, bold)
	p.face(bold, size)
	p.dc.SetColor(c)
	lead := size * 1.15
	top := y - ay*lead*float64(len(ls))
	for i, l := range ls {
		p.str(l, x, top+lead*float64(i)+lead/2, 0, 0.5)
	}
}
