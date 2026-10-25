package blog

import (
	"bytes"
	"fmt"
	"hash/fnv"
	"image/color"
	"image/png"
	"math"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomedium"
)

// Diagram covers are 1600×900 — 16:9 like a drawn cover, and drawn at their
// own size because gg resamples text through any scale transform, which
// softens the lettering.
const (
	designW = 1600.0
	designH = 900.0
)

// rgb is a colour that can be drawn at any opacity.
type rgb struct{ r, g, b uint8 }

func (c rgb) a(alpha float64) color.NRGBA {
	return color.NRGBA{c.r, c.g, c.b, uint8(math.Round(alpha * 255))}
}

func (c rgb) mix(o rgb, t float64) rgb {
	l := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t) }
	return rgb{l(c.r, o.r), l(c.g, o.g), l(c.b, o.b)}
}

var (
	cInk    = rgb{0x07, 0x0b, 0x16}
	cNavy   = rgb{0x0d, 0x15, 0x26}
	cPanel  = rgb{0x0f, 0x17, 0x2a}
	cSlate  = rgb{0x33, 0x41, 0x55}
	cSlate2 = rgb{0x47, 0x55, 0x69}
	cMuted  = rgb{0x94, 0xa3, 0xb8}
	cText   = rgb{0xe2, 0xe8, 0xf0}
	cGreen  = rgb{0x34, 0xd3, 0x99}
	cRed    = rgb{0xf8, 0x71, 0x71}
	cAmber  = rgb{0xfb, 0xbf, 0x24}
	cSky    = rgb{0x38, 0xbd, 0xf8}
	cViolet = rgb{0xa7, 0x8b, 0xfa}
	cTeal   = rgb{0x2d, 0xd4, 0xbf}
	cPink   = rgb{0xf4, 0x72, 0xb6}
)

// coverAccentColours maps the plan's one-word accent note onto a colour.
var coverAccentColours = map[string]rgb{
	"amber": cAmber, "gold": cAmber, "copper": {0xea, 0x8c, 0x55}, "orange": {0xfb, 0x92, 0x3c},
	"emerald": cGreen, "green": cGreen, "mint": cGreen,
	"ice": cSky, "cyan": cSky, "blue": cSky, "sky": cSky,
	"violet": cViolet, "purple": cViolet, "indigo": cViolet,
	"teal": cTeal, "coral": cPink, "pink": cPink, "magenta": cPink, "rose": cPink,
	"red": cRed, "crimson": cRed,
}

func coverAccentColour(accent string) rgb {
	for _, w := range strings.Fields(strings.ToLower(accent)) {
		if c, ok := coverAccentColours[strings.Trim(w, ".,;")]; ok {
			return c
		}
	}
	return cTeal
}

var (
	coverFontsOnce sync.Once
	coverBold      *truetype.Font
	coverMedium    *truetype.Font
	coverFontsErr  error
)

func coverFonts() error {
	coverFontsOnce.Do(func() {
		if coverBold, coverFontsErr = truetype.Parse(gobold.TTF); coverFontsErr != nil {
			return
		}
		coverMedium, coverFontsErr = truetype.Parse(gomedium.TTF)
	})
	return coverFontsErr
}

// pen draws one cover. Faces are made per cover rather than shared because a
// font.Face is not safe for concurrent use and runs draw covers in parallel.
type pen struct {
	dc     *gg.Context
	accent rgb
	faces  map[string]font.Face
	rnd    *rand.Rand
}

func (p *pen) face(bold bool, size float64) {
	key := fmt.Sprintf("%t/%.1f", bold, size)
	f, ok := p.faces[key]
	if !ok {
		tt := coverMedium
		if bold {
			tt = coverBold
		}
		f = truetype.NewFace(tt, &truetype.Options{Size: size, Hinting: font.HintingFull})
		p.faces[key] = f
	}
	p.dc.SetFontFace(f)
}

// text draws s centred on (x, y) at the given anchor.
func (p *pen) text(s string, x, y, size float64, c color.Color, bold bool, ax float64) {
	p.face(bold, size)
	p.dc.SetColor(c)
	p.dc.DrawStringAnchored(s, x, y, ax, 0.5)
}

// fitText draws s centred in width w, shrinking the type until it fits on at
// most two lines.
func (p *pen) fitText(s string, x, y, w, size float64, c color.Color) {
	for ; size > 14; size -= 2 {
		p.face(true, size)
		lines := p.dc.WordWrap(s, w)
		if len(lines) <= 2 && p.widest(lines) <= w {
			break
		}
	}
	p.face(true, size)
	p.dc.SetColor(c)
	p.dc.DrawStringWrapped(s, x, y, 0.5, 0.5, w, 1.25, gg.AlignCenter)
}

func (p *pen) widest(lines []string) float64 {
	m := 0.0
	for _, l := range lines {
		if w, _ := p.dc.MeasureString(l); w > m {
			m = w
		}
	}
	return m
}

func (p *pen) measure(s string, size float64, bold bool) float64 {
	p.face(bold, size)
	w, _ := p.dc.MeasureString(s)
	return w
}

// spaced draws tracked capitals for the corner tag.
func (p *pen) spaced(s string, x, y, size, track float64, c color.Color) float64 {
	p.face(true, size)
	p.dc.SetColor(c)
	for _, r := range s {
		ch := string(r)
		p.dc.DrawStringAnchored(ch, x, y, 0, 0.5)
		w, _ := p.dc.MeasureString(ch)
		x += w + track
	}
	return x
}

func (p *pen) fill(c color.Color) {
	p.dc.SetColor(c)
	p.dc.Fill()
}

func (p *pen) stroke(c color.Color, width float64, dash ...float64) {
	p.dc.SetColor(c)
	p.dc.SetLineWidth(width)
	if len(dash) > 0 {
		p.dc.SetDash(dash...)
	}
	p.dc.Stroke()
	p.dc.SetDash()
}

// box is a rounded panel with an outline.
func (p *pen) box(x, y, w, h, r float64, fillC, line color.Color, width float64) {
	p.dc.DrawRoundedRectangle(x, y, w, h, r)
	p.dc.SetColor(fillC)
	p.dc.FillPreserve()
	p.stroke(line, width)
}

func (p *pen) disc(x, y, r float64, fillC, line color.Color, width float64) {
	p.dc.DrawCircle(x, y, r)
	p.dc.SetColor(fillC)
	p.dc.FillPreserve()
	p.stroke(line, width)
}

// head draws an arrowhead at (x, y) pointing along angle.
func (p *pen) head(x, y, angle, size float64, c color.Color) {
	p.dc.MoveTo(x, y)
	p.dc.LineTo(x-size*math.Cos(angle-0.45), y-size*math.Sin(angle-0.45))
	p.dc.LineTo(x-size*math.Cos(angle+0.45), y-size*math.Sin(angle+0.45))
	p.dc.ClosePath()
	p.fill(c)
}

func (p *pen) arrow(x1, y1, x2, y2 float64, c color.Color, width float64) {
	a := math.Atan2(y2-y1, x2-x1)
	p.dc.MoveTo(x1, y1)
	p.dc.LineTo(x2-width*1.5*math.Cos(a), y2-width*1.5*math.Sin(a))
	p.stroke(c, width)
	p.head(x2, y2, a, width*4+6, c)
}

// curve draws a horizontal S-curve from (x1, y1) to (x2, y2), optionally
// ending in an arrowhead.
func (p *pen) curve(x1, y1, x2, y2 float64, c color.Color, width float64, arrow bool, dash ...float64) {
	mx := (x1 + x2) / 2
	end := x2
	if arrow {
		end = x2 - width*2
	}
	p.dc.MoveTo(x1, y1)
	p.dc.CubicTo(mx, y1, mx, y2, end, y2)
	p.stroke(c, width, dash...)
	if arrow {
		p.head(x2, y2, 0, width*4+6, c)
	}
}

func (p *pen) check(x, y, s float64, c color.Color) {
	p.dc.MoveTo(x-s, y)
	p.dc.LineTo(x-s*0.3, y+s*0.7)
	p.dc.LineTo(x+s, y-s*0.8)
	p.dc.SetLineCap(gg.LineCapRound)
	p.stroke(c, s*0.35)
	p.dc.SetLineCap(gg.LineCapButt)
}

func (p *pen) shield(x, y, s float64, fillC, line color.Color) {
	p.dc.MoveTo(x, y-s)
	p.dc.LineTo(x+s*0.85, y-s*0.65)
	p.dc.LineTo(x+s*0.85, y)
	p.dc.CubicTo(x+s*0.85, y+s*0.6, x+s*0.4, y+s*0.95, x, y+s*1.15)
	p.dc.CubicTo(x-s*0.4, y+s*0.95, x-s*0.85, y+s*0.6, x-s*0.85, y)
	p.dc.LineTo(x-s*0.85, y-s*0.65)
	p.dc.ClosePath()
	p.dc.SetColor(fillC)
	p.dc.FillPreserve()
	p.stroke(line, 3)
}

// label returns labels[i], or def when the scene has fewer.
func label(labels []string, i int, def string) string {
	if i < len(labels) && strings.TrimSpace(labels[i]) != "" {
		return labels[i]
	}
	return def
}

// padLabels returns between min and max labels, filling from defs.
func padLabels(labels []string, minN, maxN int, defs ...string) []string {
	out := append([]string(nil), labels...)
	for i := 0; len(out) < minN && i < len(defs); i++ {
		out = append(out, defs[len(out)%len(defs)])
	}
	if len(out) > maxN {
		out = out[:maxN]
	}
	return out
}

// drawDiagramCover renders a scene as a designW × designH PNG in the
// house style: dark navy ground, one accent, a diagram of the article's idea
// and a small topic tag. The website prints the title under the card, so the
// title is not lettered on the image.
//
// seed (the slug) fixes the scatter in layouts that have any, so a redraw of
// the same article gives the same picture.
func drawDiagramCover(scene coverScene, accent, seed string) ([]byte, error) {
	if err := coverFonts(); err != nil {
		return nil, fmt.Errorf("cover fonts: %w", err)
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(seed))
	dc := gg.NewContext(int(designW), int(designH))
	p := &pen{dc: dc, accent: coverAccentColour(accent), faces: map[string]font.Face{}, rnd: rand.New(rand.NewSource(int64(h.Sum64())))}
	dc.SetLineJoin(gg.LineJoinRound)

	p.ground()
	switch scene.Layout {
	case layoutHub:
		p.hub(scene)
	case layoutLoop:
		p.loop(scene)
	case layoutBars:
		p.bars(scene)
	case layoutVersus:
		p.versus(scene)
	case layoutRipple:
		p.ripple(scene)
	case layoutTree:
		p.tree(scene)
	case layoutIceberg:
		p.iceberg(scene)
	case layoutHierarchy:
		p.hierarchy(scene)
	case layoutThreshold:
		p.threshold(scene)
	default:
		p.flow(scene)
	}
	p.tag(scene.Eyebrow)

	var buf bytes.Buffer
	if err := png.Encode(&buf, dc.Image()); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (p *pen) ground() {
	dc := p.dc
	g := gg.NewLinearGradient(0, 0, designW, designH)
	g.AddColorStop(0, cInk.a(1))
	g.AddColorStop(0.6, cNavy.a(1))
	g.AddColorStop(1, p.accent.mix(cInk, 0.84).a(1))
	dc.SetFillStyle(g)
	dc.DrawRectangle(0, 0, designW, designH)
	dc.Fill()
	for y := 2.0; y < designH; y += 32 {
		for x := 2.0; x < designW; x += 32 {
			dc.DrawCircle(x, y, 1.4)
		}
	}
	p.fill(color.NRGBA{255, 255, 255, 13})
	glow := gg.NewRadialGradient(800, 414, 0, 800, 414, 880)
	glow.AddColorStop(0, p.accent.a(0.2))
	glow.AddColorStop(1, p.accent.a(0))
	dc.SetFillStyle(glow)
	dc.DrawRectangle(0, 0, designW, designH)
	dc.Fill()
}

func (p *pen) tag(eyebrow string) {
	if eyebrow == "" {
		return
	}
	p.dc.DrawRoundedRectangle(72, 818, 10, 34, 3)
	p.fill(p.accent.a(1))
	p.spaced(eyebrow, 98, 836, 24, 2.5, p.accent.a(1))
}

// flow: steps left to right, the last one highlighted.
func (p *pen) flow(s coverScene) {
	steps := padLabels(s.Labels, 3, 5, "input", "process", "output")
	n := float64(len(steps))
	gap := 70.0
	w := (1320 - gap*(n-1)) / n
	for i, st := range steps {
		x := 140 + float64(i)*(w+gap)
		last := i == len(steps)-1
		fillC, line, lw := cPanel.a(0.92), cSlate2.a(1), 2.5
		if last {
			fillC, line, lw = p.accent.a(0.22), p.accent.a(1), 4
		}
		p.box(x, 330, w, 180, 22, fillC, line, lw)
		p.fitText(st, x+w/2, 420, w-36, 30, cText.a(1))
		numC := cSlate
		if last {
			numC = p.accent
		}
		p.disc(x+w/2, 290, 22, numC.a(1), cInk.a(1), 4)
		p.text(strconv.Itoa(i+1), x+w/2, 290, 22, cInk.a(1), true, 0.5)
		if !last {
			p.arrow(x+w+10, 420, x+w+gap-10, 420, p.accent.a(0.9), 4)
		}
	}
	// progress track
	p.dc.MoveTo(140, 640)
	p.dc.LineTo(1460, 640)
	p.dc.SetLineCap(gg.LineCapRound)
	p.stroke(cSlate.a(1), 8)
	p.dc.MoveTo(140, 640)
	p.dc.LineTo(1460-w/2, 640)
	p.stroke(p.accent.a(1), 8)
	p.dc.SetLineCap(gg.LineCapButt)
	p.disc(1460-w/2, 640, 18, p.accent.a(1), cInk.a(1), 5)
}

// hub: one core connected to the things around it.
func (p *pen) hub(s coverScene) {
	core := label(s.Labels, 0, "core")
	spokes := padLabels(s.Labels[min(1, len(s.Labels)):], 4, 6, "tools", "data", "teams", "systems")
	cx, cy := 800.0, 390.0
	type pt struct{ x, y float64 }
	pos := make([]pt, len(spokes))
	for i := range spokes {
		a := -math.Pi/2 + float64(i)*2*math.Pi/float64(len(spokes))
		pos[i] = pt{cx + 440*math.Cos(a), cy + 250*math.Sin(a)}
		p.dc.MoveTo(cx, cy)
		p.dc.LineTo(pos[i].x, pos[i].y)
		p.stroke(p.accent.a(0.6), 3.5)
		p.dc.DrawCircle((cx+pos[i].x)/2, (cy+pos[i].y)/2, 6)
		p.fill(p.accent.a(1))
	}
	for i, sp := range spokes {
		w := math.Min(320, math.Max(160, p.measure(sp, 24, true)+56))
		p.box(pos[i].x-w/2, pos[i].y-34, w, 68, 18, cPanel.a(1), cSky.a(0.9), 2.5)
		p.fitText(sp, pos[i].x, pos[i].y, w-28, 24, cText.a(1))
	}
	p.dc.DrawRegularPolygon(6, cx, cy, 112, 0)
	p.dc.SetColor(p.accent.mix(cInk, 0.8).a(1))
	p.dc.FillPreserve()
	p.stroke(p.accent.a(1), 5)
	p.fitText(core, cx, cy, 170, 28, cText.a(1))
}

// loop: stages that repeat, optionally inside a boundary.
func (p *pen) loop(s coverScene) {
	stages := padLabels(s.Labels, 3, 4, "plan", "act", "observe")
	if len(s.Labels) >= 5 {
		p.dc.DrawRoundedRectangle(230, 100, 1140, 640, 48)
		p.stroke(cAmber.a(1), 3.5, 18, 12)
		w := p.measure(s.Labels[4], 22, true) + 40
		p.dc.DrawRoundedRectangle(280, 82, w, 36, 18)
		p.fill(cInk.a(1))
		p.text(s.Labels[4], 300, 100, 22, cAmber.a(1), true, 0)
	}
	cx, cy, R := 800.0, 420.0, 230.0
	n := float64(len(stages))
	for i := range stages {
		a := -math.Pi/2 + float64(i)*2*math.Pi/n
		step := 2 * math.Pi / n
		a1, a2 := a+0.38, a+step-0.38
		p.dc.DrawArc(cx, cy, R, a1, a2)
		p.stroke(p.accent.a(0.85), 5)
		ex, ey := cx+R*math.Cos(a2), cy+R*math.Sin(a2)
		p.head(ex, ey, a2+math.Pi/2, 22, p.accent.a(0.85))
	}
	for i, st := range stages {
		a := -math.Pi/2 + float64(i)*2*math.Pi/n
		x, y := cx+R*math.Cos(a), cy+R*math.Sin(a)
		p.disc(x, y, 78, cPanel.a(1), p.accent.a(1), 4)
		p.fitText(st, x, y, 128, 26, cText.a(1))
	}
	// a repeat glyph at the centre
	p.disc(cx, cy, 70, p.accent.a(0.18), p.accent.a(0.8), 3)
	p.dc.DrawArc(cx, cy, 36, -math.Pi/2, math.Pi*1.15)
	p.stroke(cText.a(1), 6)
	p.head(cx+36*math.Cos(math.Pi*1.15), cy+36*math.Sin(math.Pi*1.15), math.Pi*1.15+math.Pi/2, 20, cText.a(1))
}

// labelValue splits "caching 76" into ("caching", 76, true).
var labelValue = regexp.MustCompile(`^(.*?)[\s:=]+(\d+(?:\.\d+)?)\s*%?$`)

// bars: a number stepping down (or up). Heights are only printed as numbers
// when the article supplied them; otherwise the bars show the direction and
// carry no figures, because a cover must not invent data.
func (p *pen) bars(s coverScene) {
	steps := padLabels(s.Labels, 3, 6, "before", "step one", "step two", "after")
	names := make([]string, len(steps))
	vals := make([]float64, len(steps))
	known := true
	for i, st := range steps {
		names[i] = st
		if m := labelValue.FindStringSubmatch(st); m != nil && strings.TrimSpace(m[1]) != "" {
			names[i] = strings.TrimSpace(m[1])
			vals[i], _ = strconv.ParseFloat(m[2], 64)
		} else {
			known = false
		}
	}
	rising := strings.HasPrefix(strings.TrimSpace(s.Stat), "+")
	if !known {
		end := 35.0
		if m := regexp.MustCompile(`\d+(?:\.\d+)?`).FindString(s.Stat); m != "" {
			pct, _ := strconv.ParseFloat(m, 64)
			if pct > 0 && pct < 95 {
				end = 100 - pct
			}
		}
		for i := range vals {
			t := float64(i) / float64(len(vals)-1)
			vals[i] = 100 + (end-100)*t
			if rising {
				vals[i] = end + (100-end)*t
			}
		}
	}
	maxV := 0.0
	for _, v := range vals {
		maxV = math.Max(maxV, v)
	}
	right := 1300.0
	if s.Stat == "" {
		right = 1460
	}
	n := float64(len(vals))
	gap := 50.0
	bw := (right - 160 - gap*(n-1)) / n
	base := 700.0
	var tops [][2]float64
	for i, v := range vals {
		x := 160 + float64(i)*(bw+gap)
		h := 470 * v / maxV
		t := float64(i) / math.Max(1, n-1)
		c := cRed.mix(cAmber, math.Min(1, t*2)).mix(cGreen, math.Max(0, t*2-1))
		if rising {
			c = cSlate2.mix(p.accent, t)
		}
		p.dc.DrawRoundedRectangle(x, base-h, bw, h, 14)
		p.fill(c.a(0.88))
		if known {
			p.text(strconv.FormatFloat(v, 'f', -1, 64), x+bw/2, base-h-24, 28, cText.a(1), true, 0.5)
		}
		p.fitText(names[i], x+bw/2, base+46, bw+gap-10, 22, cMuted.a(1))
		tops = append(tops, [2]float64{x + bw/2, base - h - 60})
	}
	p.dc.MoveTo(140, base)
	p.dc.LineTo(right+30, base)
	p.stroke(cSlate.a(1), 3)
	for i, tp := range tops {
		if i == 0 {
			p.dc.MoveTo(tp[0], tp[1])
		} else {
			p.dc.LineTo(tp[0], tp[1])
		}
	}
	p.stroke(cText.a(0.5), 3, 10, 9)
	last, prev := tops[len(tops)-1], tops[len(tops)-2]
	p.head(last[0], last[1], math.Atan2(last[1]-prev[1], last[0]-prev[0]), 18, cText.a(0.6))
	if s.Stat != "" {
		stat := strings.Replace(s.Stat, "-", "−", 1)
		size := 96.0
		for size > 40 && p.measure(stat, size, true) > 250 {
			size -= 6
		}
		p.text(stat, 1420, 330, size, p.accent.a(1), true, 0.5)
	}
}

// versus: one change with a good edge and a bad edge.
func (p *pen) versus(s coverScene) {
	good, bad := label(s.Labels, 0, "upside"), label(s.Labels, 1, "downside")
	bl := gg.NewLinearGradient(742, 0, 858, 0)
	bl.AddColorStop(0, cGreen.a(1))
	bl.AddColorStop(0.495, rgb{0xa7, 0xf3, 0xd0}.a(1))
	bl.AddColorStop(0.505, rgb{0xfe, 0xca, 0xca}.a(1))
	bl.AddColorStop(1, cRed.a(1))
	p.dc.MoveTo(800, 70)
	p.dc.LineTo(858, 170)
	p.dc.LineTo(858, 600)
	p.dc.LineTo(742, 600)
	p.dc.LineTo(742, 170)
	p.dc.ClosePath()
	p.dc.SetFillStyle(bl)
	p.dc.FillPreserve()
	p.stroke(rgb{0xf8, 0xfa, 0xfc}.a(1), 3)
	p.box(660, 600, 280, 34, 12, rgb{0xcb, 0xd5, 0xe1}.a(1), rgb{0xcb, 0xd5, 0xe1}.a(1), 1)
	p.box(772, 634, 56, 110, 10, cSlate2.a(1), cSlate2.a(1), 1)
	p.dc.DrawCircle(800, 762, 24)
	p.fill(rgb{0xcb, 0xd5, 0xe1}.a(1))

	p.box(150, 180, 460, 400, 22, cPanel.a(1), cGreen.a(0.6), 2)
	for i, h := range []float64{40, 60, 80, 110, 150, 200} {
		p.dc.DrawRoundedRectangle(200+float64(i)*70, 550-h, 40, h, 6)
		p.fill(cGreen.a(0.22))
	}
	pts := [][2]float64{{190, 520}, {260, 500}, {330, 470}, {400, 440}, {470, 380}, {540, 320}, {575, 262}}
	for i, q := range pts {
		if i == 0 {
			p.dc.MoveTo(q[0], q[1])
		} else {
			p.dc.LineTo(q[0], q[1])
		}
	}
	p.stroke(cGreen.a(1), 5)
	p.head(588, 240, math.Atan2(-58, 35), 24, cGreen.a(1))
	p.fitText(good, 380, 640, 460, 28, cGreen.a(1))

	p.box(990, 180, 460, 400, 22, cPanel.a(1), cRed.a(0.6), 2)
	spikes := [][2]float64{{1020, 500}, {1080, 495}, {1110, 430}, {1140, 500}, {1210, 490}, {1240, 380}, {1270, 505}, {1340, 480}, {1370, 300}, {1400, 500}, {1430, 470}}
	for i, q := range spikes {
		if i == 0 {
			p.dc.MoveTo(q[0], q[1])
		} else {
			p.dc.LineTo(q[0], q[1])
		}
	}
	p.stroke(cRed.a(1), 5)
	for _, q := range [][2]float64{{1110, 430}, {1240, 380}, {1370, 300}} {
		p.dc.DrawCircle(q[0], q[1], 9)
		p.fill(cRed.a(1))
	}
	p.fitText(bad, 1220, 640, 460, 28, cRed.a(1))
}

// ripple: a failure spreading out through the services that depend on it.
func (p *pen) ripple(s coverScene) {
	cx, cy := 760.0, 410.0
	for _, r := range []struct{ r, a float64 }{{340, 0.07}, {230, 0.12}, {130, 0.2}} {
		p.dc.DrawCircle(cx, cy, r.r)
		p.fill(cRed.a(r.a))
		p.dc.DrawCircle(cx, cy, r.r)
		if r.r == 130 {
			p.stroke(cRed.a(r.a*3), 2.5)
		} else {
			p.stroke(cRed.a(r.a*3), 2.5, 10, 10)
		}
	}
	type node struct {
		x, y float64
		ring int
	}
	var nodes []node
	for ring, spec := range [][2]float64{{5, 85}, {7, 185}, {9, 290}, {10, 400}} {
		off := p.rnd.Float64() * 2 * math.Pi
		for k := 0; k < int(spec[0]); k++ {
			a := off + float64(k)*2*math.Pi/spec[0]
			rx := spec[1]
			if ring == 3 {
				rx *= 1.25
			}
			nodes = append(nodes, node{cx + rx*math.Cos(a), cy + spec[1]*0.95*math.Sin(a), ring})
		}
	}
	ringC := []rgb{cRed, cRed, cAmber, cGreen}
	for _, n := range nodes {
		if n.ring == 0 {
			p.dc.MoveTo(cx, cy)
			p.dc.LineTo(n.x, n.y)
			p.stroke(cRed.a(0.7), 3)
			continue
		}
		var best node
		bd := math.MaxFloat64
		for _, m := range nodes {
			if m.ring == n.ring-1 {
				if d := math.Hypot(m.x-n.x, m.y-n.y); d < bd {
					bd, best = d, m
				}
			}
		}
		edge := ringC[n.ring]
		if n.ring == 3 {
			edge = cSlate2
		}
		p.dc.MoveTo(best.x, best.y)
		p.dc.LineTo(n.x, n.y)
		p.stroke(edge.a(0.55), 2.5)
	}
	for _, n := range nodes {
		if n.y < 40 || n.y > 780 {
			continue
		}
		r := 16.0
		if n.ring == 3 {
			r = 13
		}
		p.disc(n.x, n.y, r, cPanel.a(1), ringC[n.ring].a(1), 4)
	}
	p.dc.DrawCircle(cx, cy, 38)
	p.fill(cRed.a(1))
	p.text("!", cx, cy, 44, cInk.a(1), true, 0.5)
	legend := padLabels(s.Labels, 3, 3, "impacted", "at risk", "healthy")
	for i, l := range legend {
		y := 600 + float64(i)*46
		p.dc.DrawCircle(1300, y, 11)
		p.fill([]rgb{cRed, cAmber, cGreen}[i].a(1))
		size := 24.0
		for size > 14 && p.measure(l, size, true) > 240 {
			size -= 2
		}
		p.text(l, 1324, y, size, cText.a(1), true, 0)
	}
}

// tree: many candidate paths, the weak ones pruned, the survivors kept.
func (p *pen) tree(s coverScene) {
	result := label(s.Labels, 0, "best of k")
	caption := label(s.Labels, 1, "")
	root := [2]float64{230, 420}
	mids := [][2]float64{{560, 200}, {560, 420}, {560, 640}}
	leaves := [][2]float64{{900, 110}, {900, 250}, {900, 360}, {900, 480}, {900, 590}, {900, 720}}
	keep := map[int]rgb{0: cViolet, 2: cSky, 4: cTeal}
	for _, m := range mids {
		p.dc.MoveTo(root[0], root[1])
		p.dc.CubicTo(380, root[1], 400, m[1], m[0], m[1])
		p.stroke(cSlate2.a(1), 4)
	}
	for j, l := range leaves {
		m := mids[j/2]
		if c, ok := keep[j]; ok {
			p.curve(m[0], m[1], l[0], l[1], c.a(1), 5, false)
			ty := 360 + float64(j/2)*100
			p.curve(l[0], l[1], 1225, ty, c.a(1), 5, true)
			continue
		}
		p.curve(m[0], m[1], l[0], l[1], cSlate2.a(0.7), 3, false, 8, 9)
		sx, sy := (m[0]+l[0])/2+10, (m[1]+l[1])/2
		p.dc.DrawCircle(sx-9, sy-10, 7)
		p.dc.NewSubPath()
		p.dc.DrawCircle(sx-9, sy+10, 7)
		p.dc.MoveTo(sx-3, sy-6)
		p.dc.LineTo(sx+14, sy+8)
		p.dc.MoveTo(sx-3, sy+6)
		p.dc.LineTo(sx+14, sy-8)
		p.stroke(cRed.a(1), 3.5)
	}
	for j, l := range leaves {
		c := cSlate2
		if k, ok := keep[j]; ok {
			c = k
		}
		p.disc(l[0], l[1], 15, cPanel.a(1), c.a(1), 4)
	}
	for _, m := range mids {
		p.disc(m[0], m[1], 18, cPanel.a(1), cMuted.a(1), 4)
	}
	p.disc(root[0], root[1], 46, p.accent.a(0.25), p.accent.a(1), 4)
	p.text("?", root[0], root[1], 36, cText.a(1), true, 0.5)
	p.box(1235, 300, 245, 330, 22, cPanel.a(1), p.accent.a(1), 3)
	for k, c := range []rgb{cViolet, cSky, cTeal} {
		y := 360 + float64(k)*100
		p.box(1262, y-26, 190, 52, 12, c.a(0.18), c.a(1), 2.5)
		p.dc.DrawRoundedRectangle(1282, y-6, 110-float64(k)*18, 12, 6)
		p.fill(c.a(0.7))
	}
	p.check(1420, 460, 14, cGreen.a(1))
	p.fitText(result, 1357, 262, 300, 30, cText.a(1))
	if caption != "" {
		p.fitText(caption, 560, 790, 700, 24, cMuted.a(1))
	}
}

// iceberg: the visible effect above the waterline, the larger hidden ones below.
func (p *pen) iceberg(s coverScene) {
	tip := label(s.Labels, 0, "what you see")
	var hidden []string
	if len(s.Labels) > 1 {
		hidden = s.Labels[1:]
	}
	hidden = padLabels(hidden, 3, 5, "cost", "risk", "skills")
	p.dc.DrawRectangle(0, 330, designW, 470)
	p.fill(rgb{0x0e, 0x74, 0x90}.a(0.16))
	p.dc.MoveTo(0, 330)
	for x := 0.0; x < designW; x += 200 {
		p.dc.QuadraticTo(x+50, 318, x+100, 330)
		p.dc.QuadraticTo(x+150, 342, x+200, 330)
	}
	p.stroke(cSky.a(0.8), 3)
	ice := gg.NewLinearGradient(0, 330, 0, 770)
	ice.AddColorStop(0, cTeal.a(0.55))
	ice.AddColorStop(1, rgb{0x13, 0x4e, 0x4a}.a(0.35))
	body := [][2]float64{{700, 330}, {915, 330}, {1030, 400}, {1180, 470}, {1240, 580}, {1150, 690}, {960, 760}, {700, 770}, {500, 720}, {380, 610}, {420, 480}, {560, 400}}
	for i, q := range body {
		if i == 0 {
			p.dc.MoveTo(q[0], q[1])
		} else {
			p.dc.LineTo(q[0], q[1])
		}
	}
	p.dc.ClosePath()
	p.dc.SetFillStyle(ice)
	p.dc.FillPreserve()
	p.stroke(cTeal.a(0.6), 2)
	p.dc.MoveTo(700, 330)
	p.dc.LineTo(760, 205)
	p.dc.LineTo(800, 150)
	p.dc.LineTo(842, 214)
	p.dc.LineTo(872, 250)
	p.dc.LineTo(915, 330)
	p.dc.ClosePath()
	p.dc.SetColor(rgb{0xe6, 0xff, 0xfb}.a(1))
	p.dc.FillPreserve()
	p.stroke(color.White, 2)
	for i, y := range []float64{190, 225, 260} {
		p.dc.MoveTo(600-float64(i)*30, y)
		p.dc.LineTo(690-float64(i)*15, y)
		p.dc.SetLineCap(gg.LineCapRound)
		p.stroke(cTeal.a(0.9-float64(i)*0.25), 4)
	}
	p.dc.SetLineCap(gg.LineCapButt)
	p.text(tip, 990, 215, 38, cText.a(1), true, 0)
	spots := [][2]float64{{560, 470}, {880, 450}, {700, 570}, {1010, 590}, {790, 680}}
	for i, h := range hidden {
		x, y := spots[i][0], spots[i][1]
		w := math.Min(330, p.measure(h, 22, true)+44)
		p.box(x-w/2, y-24, w, 48, 24, cPanel.a(0.88), cTeal.a(1), 2)
		p.fitText(h, x, y, w-24, 22, rgb{0xcc, 0xfb, 0xf1}.a(1))
	}
}

// hierarchy: one policy or standard flowing down to the units it governs.
func (p *pen) hierarchy(s coverScene) {
	top := label(s.Labels, 0, "policy")
	var units []string
	if len(s.Labels) > 1 {
		units = s.Labels[1:]
	}
	units = padLabels(units, 3, 3, "team a", "team b", "team c")
	p.box(700, 70, 200, 160, 14, cPanel.a(1), p.accent.a(1), 3)
	for k := 0; k < 4; k++ {
		p.dc.DrawRoundedRectangle(725, 100+float64(k)*24, 120-float64(k%2)*40, 10, 5)
		p.fill(cSlate2.a(1))
	}
	p.shield(860, 168, 34, p.accent.a(1), cInk.a(1))
	p.check(860, 176, 12, cInk.a(1))
	p.fitText(top, 800, 268, 520, 26, p.accent.a(1))
	p.dc.MoveTo(800, 292)
	p.dc.LineTo(800, 330)
	p.dc.MoveTo(380, 330)
	p.dc.LineTo(1220, 330)
	for _, x := range []float64{380, 800, 1220} {
		p.dc.MoveTo(x, 330)
		p.dc.LineTo(x, 375)
	}
	p.stroke(p.accent.a(0.7), 3.5)
	for i, x := range []float64{380, 800, 1220} {
		p.box(x-175, 375, 350, 330, 22, cPanel.a(1), cSlate.a(1), 2.5)
		p.fitText(units[i], x, 420, 310, 26, cText.a(1))
		pts := [][2]float64{{x - 110, 500}, {x - 40, 570}, {x - 110, 640}, {x + 10, 480}}
		for _, e := range [][2]int{{0, 1}, {1, 2}, {1, 3}} {
			p.dc.MoveTo(pts[e[0]][0], pts[e[0]][1])
			p.dc.LineTo(pts[e[1]][0], pts[e[1]][1])
		}
		p.stroke(cSky.a(0.6), 2.5)
		for _, q := range pts {
			p.disc(q[0], q[1], 13, cPanel.a(1), cSky.a(1), 3)
		}
		p.shield(x+95, 560, 46, cGreen.a(0.18), cGreen.a(1))
		p.check(x+95, 572, 16, cGreen.a(1))
	}
}

// threshold: a monitored metric drifting from its baseline past its limit,
// raising an alert that is recorded.
func (p *pen) threshold(s coverScene) {
	dist := label(s.Labels, 0, "distribution")
	score := label(s.Labels, 1, "drift score")
	alert := label(s.Labels, 2, "alert")
	record := label(s.Labels, 3, "audit log")

	p.box(130, 130, 620, 520, 24, cPanel.a(1), cSlate.a(1), 2.5)
	p.text(dist, 170, 182, 24, cMuted.a(1), true, 0)
	bell := func(mu, sig, amp float64) [][2]float64 {
		var pts [][2]float64
		for i := 0; i <= 90; i++ {
			x := 170 + 540*float64(i)/90
			u := (x - mu) / sig
			pts = append(pts, [2]float64{x, 580 - amp*math.Exp(-u*u/2)})
		}
		return pts
	}
	for _, c := range []struct {
		pts  [][2]float64
		col  rgb
		dash bool
	}{{bell(370, 62, 300), cGreen, false}, {bell(520, 80, 230), cRed, true}} {
		p.dc.MoveTo(170, 580)
		for _, q := range c.pts {
			p.dc.LineTo(q[0], q[1])
		}
		p.dc.LineTo(710, 580)
		p.dc.ClosePath()
		p.fill(c.col.a(0.16))
		for i, q := range c.pts {
			if i == 0 {
				p.dc.MoveTo(q[0], q[1])
			} else {
				p.dc.LineTo(q[0], q[1])
			}
		}
		if c.dash {
			p.stroke(c.col.a(1), 5, 14, 8)
		} else {
			p.stroke(c.col.a(1), 5)
		}
	}
	p.dc.MoveTo(170, 580)
	p.dc.LineTo(710, 580)
	p.stroke(cSlate2.a(1), 3)
	p.arrow(385, 250, 505, 250, cAmber.a(1), 5)
	p.dc.DrawCircle(190, 620, 9)
	p.fill(cGreen.a(1))
	p.text("baseline", 208, 620, 20, cText.a(1), true, 0)
	p.dc.DrawCircle(480, 620, 9)
	p.fill(cRed.a(1))
	p.text("now", 498, 620, 20, cText.a(1), true, 0)

	p.box(830, 130, 640, 320, 24, cPanel.a(1), cSlate.a(1), 2.5)
	p.text(score, 870, 182, 24, cMuted.a(1), true, 0)
	p.dc.MoveTo(870, 270)
	p.dc.LineTo(1430, 270)
	p.stroke(cRed.a(1), 3, 10, 8)
	p.text("limit", 1060, 254, 20, cRed.a(1), true, 0)
	line := [][2]float64{{870, 400}, {940, 392}, {1010, 398}, {1080, 380}, {1150, 372}, {1220, 340}, {1290, 300}, {1360, 250}, {1420, 222}}
	for i, q := range line {
		if i == 0 {
			p.dc.MoveTo(q[0], q[1])
		} else {
			p.dc.LineTo(q[0], q[1])
		}
	}
	p.stroke(cAmber.a(1), 5)
	p.dc.DrawCircle(1360, 250, 12)
	p.fill(cRed.a(1))

	p.box(830, 490, 300, 160, 24, cRed.a(0.12), cRed.a(1), 3)
	p.dc.MoveTo(890, 610)
	p.dc.LineTo(930, 540)
	p.dc.LineTo(970, 610)
	p.dc.ClosePath()
	p.stroke(cRed.a(1), 5)
	p.text("!", 930, 588, 30, cRed.a(1), true, 0.5)
	p.fitText(alert, 1055, 570, 130, 28, cText.a(1))
	p.box(1170, 490, 300, 160, 24, cPanel.a(1), cSky.a(1), 3)
	p.shield(1240, 560, 34, cSky.a(0.2), cSky.a(1))
	p.fitText(record, 1370, 570, 170, 26, cText.a(1))
}
