package main

import (
	"image"
	"image/color"
	"math"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"nook/core/config"
)

// Looks float: the window's own color (a look's background, a gradient,
// a picture) shows all around; the terminals, and what opens beside them,
// sit on it as one card. The quick way to make a look: three colors of
// the background and how light the terminals are; every other color
// follows from them.

// chromeColor is the window's own color, around the card: the look's
// background, else the palette's panels'.
func (a *app) chromeColor(t *ui.Theme) ui.Color {
	if d := a.currentDesign(); d != nil {
		return ui.Hex(d.Background)
	}
	return t.Surface
}

// cardColor is the card's color where no terminal covers it: what opens
// beside the terminals, the empty project.
func (a *app) cardColor(t *ui.Theme) ui.Color {
	if d := a.currentDesign(); d != nil {
		return ui.Hex(d.Surface).Alpha(panelAlpha(d))
	}
	return t.Background
}

// termBg is the terminals' background, as their theme says; kept until
// the colors change (applyColors, designChanged).
func (a *app) termBg(t *ui.Theme) ui.Color {
	if a.termBgSet && a.termBgDark == t.Dark {
		return a.termBgVal
	}
	light, dark := a.termThemes()
	var bg ui.Color
	switch {
	case t.Dark && dark != nil:
		bg = dark.Background
	case !t.Dark && light != nil:
		bg = light.Background
	case t.Dark:
		bg = terminal.DarkTheme().Background
	default:
		bg = terminal.LightTheme().Background
	}
	a.termBgVal, a.termBgDark, a.termBgSet = bg, t.Dark, true
	return bg
}

// card makes e the floating card: rounded, a hairline around it, a soft
// shadow below.
func (a *app) card(c *ui.Context, e ui.Element) ui.Element {
	t := c.Theme()
	shadow := ui.RGBA(0, 0, 0, 0.10)
	if t.Dark {
		shadow = ui.RGBA(0, 0, 0, 0.30)
	}
	bg := a.cardColor(t)
	if seeThrough(a.currentDesign()) {
		bg = ui.Transparent // the terminals let the picture show themselves
	}
	return e.Radius(10).Clip().Background(bg).Border(1, t.Border.Alpha(0.7)).Shadow(0, 4, 16, 0, shadow)
}

// gradientStops are a look's background colors, in order: one, or the
// gradient's two or three.
func gradientStops(d *config.Design) []ui.Color {
	out := []ui.Color{ui.Hex(d.Background)}
	if d.Gradient == "" {
		return out
	}
	if d.Middle != "" {
		out = append(out, ui.Hex(d.Middle))
	}
	return append(out, ui.Hex(d.Gradient))
}

// paintBackground fills e with the look's background: its color, or its
// gradient, along a line or from a point.
func paintBackground(e ui.Element, d *config.Design) ui.Element {
	cols := gradientStops(d)
	if len(cols) == 1 {
		return e.Background(cols[0])
	}
	stops := make([]ui.GradientStop, len(cols))
	for i, col := range cols {
		stops[i] = ui.GradientStop{Color: col, At: float32(i) / float32(len(cols)-1)}
	}
	if d.Radial {
		return e.RadialGradient(ui.RadialGradient{Stops: stops, Center: radialCenter(d.GradientAngle), Oklab: true})
	}
	return e.LinearGradient(ui.LinearGradient{Stops: stops, Angle: d.GradientAngle, Oklab: true})
}

// radialCenter is where a radial background spreads from: off the middle
// toward angle, in degrees clockwise from up.
func radialCenter(angle float32) ui.Point {
	r := float64(angle) * math.Pi / 180
	return ui.Point{X: float32(0.5 + 0.38*math.Sin(r)), Y: float32(0.5 - 0.38*math.Cos(r))}
}

// quickStops are the three colors the quick way colors a look by: its
// background's, a single color three times.
func quickStops(d *config.Design) [3]ui.Color {
	bg := ui.Hex(d.Background)
	end, mid := bg, bg
	if d.Gradient != "" {
		end = ui.Hex(d.Gradient)
		mid = bg.Mix(end, 0.5)
	}
	if d.Middle != "" {
		mid = ui.Hex(d.Middle)
	}
	return [3]ui.Color{bg, mid, end}
}

// setQuickStops gives the look's background the three colors.
func setQuickStops(d *config.Design, s [3]ui.Color) {
	d.Background, d.Middle, d.Gradient = hex(s[0]), hex(s[1]), hex(s[2])
}

// shadeOf is how light the look's terminals are, from 0 (black) to 1
// (white), as quickLook makes them.
func shadeOf(d *config.Design) float64 {
	if d == nil || d.TermBg == "" {
		return 0.06
	}
	return min(max((float64(luminance(ui.Hex(d.TermBg)))-0.035)/0.93, 0), 1)
}

// colorOr is the color s names, else fallback's: a look missing one.
func colorOr(s, fallback string) ui.Color {
	if s == "" {
		s = fallback
	}
	if s == "" {
		return ui.Transparent
	}
	return ui.Hex(s)
}

// quickLook colors look d from its background and shade, how light its
// terminals are: text that reads on the background, an accent of its
// most vivid color, rows chosen and panels that let it show.
func quickLook(d *config.Design, shade float64) {
	cols := gradientStops(d)
	var r, g, b, lum float64
	for _, col := range cols {
		r, g, b = r+float64(col.R), g+float64(col.G), b+float64(col.B)
		lum += float64(luminance(col))
	}
	n := float64(len(cols))
	base := ui.RGB(uint8(r/n), uint8(g/n), uint8(b/n))
	white, black := ui.RGB(250, 251, 252), ui.RGB(14, 17, 20)
	d.Dark = lum/n < 0.5
	if d.Dark {
		d.Text, d.Surface, d.Border, d.Selection = hex(white.Mix(base, 0.08)), hex(base.Mix(white, 0.07)), hex(base.Mix(white, 0.16)), "#ffffff1c"
	} else {
		d.Text, d.Surface, d.Border, d.Selection = hex(black.Mix(base, 0.12)), hex(base.Mix(white, 0.6)), hex(base.Mix(black, 0.13)), "#00000012"
	}
	d.Muted = hex(ui.Hex(d.Text).Mix(base, 0.42))
	// The accent: the most vivid of the colors, light enough to read.
	vivid, best := cols[0], -1.0
	for _, col := range cols {
		_, s, l := hslOf(col)
		if v := s * (1 - math.Abs(2*l-1)); v > best {
			vivid, best = col, v
		}
	}
	h, s, _ := hslOf(vivid)
	s = min(max(s+0.15, 0.4), 0.7)
	if d.Dark {
		d.Accent = hex(hsl(h, s, 0.68))
	} else {
		d.Accent = hex(hsl(h, s, 0.4))
	}
	// The panels show the background; colors given them go.
	d.TitleBar, d.Sidebar, d.TabBar, d.Panel = "", "", "", ""
	term := ui.RGB(8, 11, 15).Mix(ui.RGB(246, 247, 248), float32(shade)).Mix(cols[0], 0.12)
	d.TermBg, d.TermOwn = hex(term), true
	if luminance(term) < 0.5 {
		d.TermFg = hex(ui.RGB(226, 232, 236).Mix(term, 0.12))
	} else {
		d.TermFg = hex(ui.RGB(28, 33, 38).Mix(term, 0.1))
	}
	if d.Image == "" {
		d.PanelOpacity = 1 // the card stays solid; see-through is the picture's
	}
}

// quickPreset is a look ready-made: three colors of the background, how
// they blend and which way, an accent of its own, and its terminals: a
// theme of Ghostty's for the colors programs print, on its own background
// unless the look gives them one, and a few other flavors that go with
// the background, to change to when this one gets old. Each look is for
// a mood, not a kind of person: cool colors for long focused work, warm
// ones for a looser one, light ones for the day.
type quickPreset struct {
	id, zh, en     string
	descZh, descEn string
	stops          [3]string
	radial         bool
	angle          float32
	accent         string
	term           string   // the terminals' theme
	termBg, termFg string   // over the theme's own, when set
	flavors        []string // other terminal themes that go with it
}

var quickPresets = []quickPreset{
	{id: "aurora", zh: "极光", en: "Aurora", descZh: "深夜的蓝绿渐到紫，安静专注", descEn: "Night blue-green into violet: calm, focused",
		stops: [3]string{"#0a1424", "#0f2f38", "#2b1a4c"}, angle: 135, accent: "#7cc7c4",
		term: "Nord", termBg: "#182431", termFg: "#b9cad2", flavors: []string{"TokyoNight Storm", "Catppuccin Frappe", "Rose Pine Moon"}},
	{id: "tokyo", zh: "东京夜", en: "Tokyo night", descZh: "深蓝底加一点霓虹，城市的夜", descEn: "Deep blue with a neon touch: the city at night",
		stops: [3]string{"#0d0f1e", "#16193a", "#2a1640"}, radial: true, angle: 45, accent: "#7aa2f7",
		term: "TokyoNight", flavors: []string{"TokyoNight Moon", "Catppuccin Mocha", "Nord"}},
	{id: "rose", zh: "玫瑰", en: "Rosé", descZh: "低对比的暮色粉紫，最护眼", descEn: "Muted dusk rose, low contrast: easiest on the eyes",
		stops: [3]string{"#191724", "#26233a", "#3b2437"}, angle: 160, accent: "#ebbcba",
		term: "Rose Pine", flavors: []string{"Rose Pine Moon", "Catppuccin Macchiato", "Catppuccin Mocha"}},
	{id: "forest", zh: "森林", en: "Forest", descZh: "柔和的墨绿，放松不刺眼", descEn: "Soft green: relaxed, never glaring",
		stops: [3]string{"#1b2124", "#1f2d27", "#2b3a2c"}, angle: 160, accent: "#a7c080",
		term: "Everforest Dark Hard", flavors: []string{"Everforest Dark Med", "Gruvbox Material Dark", "Kanagawa Wave"}},
	{id: "campfire", zh: "篝火", en: "Campfire", descZh: "暖棕带一点橙光，复古温暖", descEn: "Warm brown, an orange glow: retro, cozy",
		stops: [3]string{"#1a1715", "#272019", "#3d2416"}, radial: true, angle: 200, accent: "#e8a04c",
		term: "Gruvbox Dark", flavors: []string{"Gruvbox Dark Hard", "Kanagawa Dragon", "Melange Dark"}},
	{id: "midnight", zh: "午夜", en: "Midnight", descZh: "接近纯黑，极简不分心", descEn: "Nearly black: minimal, nothing to distract",
		stops: [3]string{"#0a0b0d", "#0d1014", "#12161d"}, angle: 180, accent: "#8ab4f8",
		term: "GitHub Dark Default", flavors: []string{"TokyoNight Night", "Ayu", "Catppuccin Mocha"}},
	{id: "mist", zh: "晨雾", en: "Mist", descZh: "浅灰蓝，白天清爽", descEn: "Light blue-gray: fresh, for the day",
		stops: [3]string{"#e3eaf0", "#edf1f4", "#d9e3ec"}, angle: 150, accent: "#3b78c4",
		term: "Catppuccin Latte", flavors: []string{"TokyoNight Day", "GitHub Light Default", "Nord Light"}},
	{id: "paper", zh: "纸白", en: "Paper", descZh: "米白纸张，长时间看字不累", descEn: "Cream paper: long reading, no strain",
		stops: [3]string{"#f4efe6", "#efe8db", "#e6dccb"}, angle: 160, accent: "#b0622a",
		term: "Rose Pine Dawn", flavors: []string{"Gruvbox Light", "Everforest Light Med", "Melange Light"}},
}

// apply colors d as the preset, its terminals too.
func (p quickPreset) apply(d *config.Design) {
	setQuickStops(d, [3]ui.Color{ui.Hex(p.stops[0]), ui.Hex(p.stops[1]), ui.Hex(p.stops[2])})
	d.Radial, d.GradientAngle = p.radial, p.angle
	quickLook(d, 0.05)
	if p.accent != "" {
		d.Accent = p.accent
	}
	d.TermTheme, d.TermOwn = p.term, true
	if th := ghosttyTheme(p.term); th != nil {
		d.TermBg, d.TermFg = hex(th.Background), hex(th.Foreground)
	}
	if p.termBg != "" {
		d.TermBg, d.TermFg = p.termBg, p.termFg
	}
	// What opens beside the terminals shares their card: its color is
	// theirs, a shade toward the text.
	d.Surface = hex(ui.Hex(d.TermBg).Mix(ui.Hex(d.Text), 0.04))
}

// presetOf is the ready-made look palette id chooses, nil for another.
func presetOf(id string) *quickPreset {
	if name, ok := strings.CutPrefix(id, "look:"); ok {
		if i := presetIndex(name); i >= 0 {
			return &quickPresets[i]
		}
	}
	return nil
}

// termFlavors are the terminal themes to change to under the look
// chosen, besides its own: those that go with a ready-made look, else a
// few liked by many.
func (a *app) termFlavors() []string {
	if p := presetOf(a.st.Palette); p != nil {
		return p.flavors
	}
	return termSuggestions
}

// nextTermFlavor changes the terminals' colors to the next flavor of the
// look's, after its own, round again.
func (a *app) nextTermFlavor() {
	all := append([]string{""}, a.termFlavors()...)
	i := slices.Index(all, a.st.TermTheme)
	a.st.TermTheme = all[(i+1)%len(all)]
	a.applyColors()
	name := a.st.TermTheme
	if name == "" {
		name = L("这套配色自带的", "the look's own")
	}
	a.toast(L("终端换成了：", "Terminals now: ")+name, nil)
}

var presetLooks []config.Design

// presetLook is quick preset i as a look, made once: what the palette
// "look:" and its id chooses, and the card that shows it.
func presetLook(i int) *config.Design {
	if presetLooks == nil {
		presetLooks = make([]config.Design, len(quickPresets))
		for j, p := range quickPresets {
			d := &presetLooks[j]
			d.ID, d.Name = p.id, L(p.zh, p.en)
			p.apply(d)
		}
	}
	return &presetLooks[i]
}

// presetIndex is the index of the preset id names, -1 for none.
func presetIndex(id string) int {
	for i, p := range quickPresets {
		if p.id == id {
			return i
		}
	}
	return -1
}

// randomQuick colors d at random: three neighboring hues, dark mostly.
func randomQuick(d *config.Design) float64 {
	h := rand.Float64() * 360
	dark := rand.Float64() < 0.75
	var s [3]ui.Color
	for i := range s {
		l := 0.1 + rand.Float64()*0.14
		if !dark {
			l = 0.82 + rand.Float64()*0.1
		}
		s[i] = hsl(math.Mod(h+float64(i)*(20+rand.Float64()*40), 360), 0.35+rand.Float64()*0.35, l)
	}
	setQuickStops(d, s)
	d.Radial, d.GradientAngle = rand.Float64() < 0.3, float32(rand.Float64()*360)
	shade := rand.Float64() * 0.12
	if !dark {
		shade = 0.9 + rand.Float64()*0.1
	}
	quickLook(d, shade)
	return shade
}

// copyDesign makes a copy of look d, named name (else the user's next),
// and chooses it.
func (a *app) copyDesign(d *config.Design, name string) *config.Design {
	cp := *d
	cp.ID = strconv.FormatInt(time.Now().UnixNano(), 36)
	cp.Name = name
	if name == "" {
		cp.Name = L("我的配色 ", "My look ") + itoa(len(a.st.Designs)+1)
	}
	a.st.Designs = append(a.st.Designs, cp)
	a.choosePalette("my:" + cp.ID)
	return &a.st.Designs[len(a.st.Designs)-1]
}

// The quick colors' field: hue across, light at the top to dark at the
// bottom, in dots.
const (
	fieldW, fieldH       = 288, 208
	fieldLTop, fieldLBot = 0.94, 0.06
	fieldSat             = 0.62
)

// fieldColor is the color at (x, y) of the field, each from 0 to 1.
func fieldColor(x, y float64) ui.Color {
	x, y = min(max(x, 0), 0.999), min(max(y, 0), 1)
	return hsl(x*360, fieldSat, fieldLTop-y*(fieldLTop-fieldLBot))
}

// fieldPlace is where color col sits on the field, each from 0 to 1.
func fieldPlace(col ui.Color) (x, y float64) {
	h, _, l := hslOf(col)
	return h / 360, min(max((fieldLTop-l)/(fieldLTop-fieldLBot), 0), 1)
}

var fieldBitmap *ui.Bitmap

// dotField is the field's picture, drawn once, at twice its size for
// sharp dots.
func dotField() *ui.Bitmap {
	if fieldBitmap != nil {
		return fieldBitmap
	}
	const scale, gap, radius = 2, 11.0, 2.1
	img := image.NewRGBA(image.Rect(0, 0, fieldW*scale, fieldH*scale))
	for cy := gap / 2; cy < fieldH; cy += gap {
		for cx := gap / 2; cx < fieldW; cx += gap {
			col := fieldColor(cx/fieldW, cy/fieldH)
			px, py, pr := cx*scale, cy*scale, radius*scale
			for y := int(py - pr - 1); y <= int(py+pr+1); y++ {
				for x := int(px - pr - 1); x <= int(px+pr+1); x++ {
					// How much of the pixel the dot covers, for smooth edges.
					cover := min(max(pr-math.Hypot(float64(x)+0.5-px, float64(y)+0.5-py)+0.5, 0), 1)
					if cover > 0 {
						a := uint8(cover * 255)
						img.SetRGBA(x, y, color.RGBA{uint8(float64(col.R) * cover), uint8(float64(col.G) * cover), uint8(float64(col.B) * cover), a})
					}
				}
			}
		}
	}
	fieldBitmap = ui.NewBitmap(img)
	return fieldBitmap
}

// hexA is c as #rrggbb, or #rrggbbaa when it lets what is behind show.
func hexA(c ui.Color) string {
	if c.A == 255 {
		return hex(c)
	}
	const digits = "0123456789abcdef"
	return hex(c) + string([]byte{digits[c.A>>4], digits[c.A&15]})
}

// hslOf is color c's hue (degrees), saturation and lightness (0 to 1).
func hslOf(c ui.Color) (h, s, l float64) {
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	mx, mn := max(r, g, b), min(r, g, b)
	l = (mx + mn) / 2
	if mx == mn {
		return 0, 0, l
	}
	d := mx - mn
	if l > 0.5 {
		s = d / (2 - mx - mn)
	} else {
		s = d / (mx + mn)
	}
	switch mx {
	case r:
		h = math.Mod((g-b)/d+6, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	return h * 60, s, l
}
