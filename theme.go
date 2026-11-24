package main

import (
	"strings"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

// Colors: a palette of Nook's (or one made from a terminal theme of
// Ghostty's) colors the window and the terminals alike; an accent of the
// user's goes over it; the terminals can take a theme of their own.

// palette is a set of the window's colors.
type palette struct {
	id, zh, en                                      string
	dark                                            bool
	bg, surface, hover, border, text, muted, accent string
}

var palettes = []palette{
	{"warm", "暖灰", "Warm gray", true, "#1c1b19", "#242320", "#2f2d29", "#3a3733", "#ece8e1", "#a39e96", "#e0a458"},
	{"ink", "墨绿", "Ink green", true, "#141a17", "#1b2320", "#25302b", "#2f3a34", "#e3ece6", "#93a39a", "#5fbf8f"},
	{"ocean", "深海", "Deep sea", true, "#10151f", "#17202e", "#202b3d", "#2a354b", "#e2e8f2", "#8e9ab0", "#5aa9e6"},
	{"paper", "纸白", "Paper", false, "#faf8f4", "#f0ede6", "#e6e2d9", "#d8d2c7", "#2b2824", "#7a746b", "#b5651d"},
}

// accents are the accent colors offered, besides the palette's own.
var accents = []string{"#e0a458", "#e5484d", "#f76b15", "#30a46c", "#12a594", "#0090ff", "#6e56cf", "#d6409f"}

func paletteByID(id string) (palette, bool) {
	for _, p := range palettes {
		if p.id == id {
			return p, true
		}
	}
	return palette{}, false
}

// ghosttyPalette makes a palette of a terminal theme of Ghostty's: the
// window takes the terminal's background and text.
func ghosttyPalette(name string) (palette, *terminal.Theme, bool) {
	th, err := terminal.GhosttyTheme(name)
	if err != nil {
		return palette{}, nil, false
	}
	bg, fg := th.Background, th.Foreground
	dark := luminance(bg) < 0.5
	accent := th.Palette[12] // bright blue
	if !dark {
		accent = th.Palette[4]
	}
	return palette{
		id: "ghostty:" + name, zh: name, en: name, dark: dark,
		bg: hex(bg), surface: hex(bg.Mix(fg, 0.05)), hover: hex(bg.Mix(fg, 0.10)), border: hex(bg.Mix(fg, 0.16)),
		text: hex(fg), muted: hex(fg.Mix(bg, 0.42)), accent: hex(accent),
	}, th, true
}

func luminance(c ui.Color) float32 {
	return (0.2126*float32(c.R) + 0.7152*float32(c.G) + 0.0722*float32(c.B)) / 255
}

func hex(c ui.Color) string {
	const digits = "0123456789abcdef"
	b := []byte{'#', 0, 0, 0, 0, 0, 0}
	for i, v := range []uint8{c.R, c.G, c.B} {
		b[1+2*i], b[2+2*i] = digits[v>>4], digits[v&15]
	}
	return string(b)
}

// currentPalette is the palette chosen, ok false for the system's look.
func (a *app) currentPalette() (palette, bool) {
	if d := a.currentDesign(); d != nil {
		return designPalette(d), true
	}
	if name, ok := strings.CutPrefix(a.st.Palette, "ghostty:"); ok {
		p, _, ok := ghosttyPalette(name)
		return p, ok
	}
	return paletteByID(a.st.Palette)
}

// windowTheme is the window's theme as chosen: nil for MyGo's own,
// following the system, when nothing is chosen.
func (a *app) windowTheme(c *ui.Context) *ui.Theme {
	p, ok := a.currentPalette()
	key := a.st.Palette + "|" + a.st.Accent
	if !ok {
		if a.st.Accent == "" {
			return nil
		}
		key += "|" + map[bool]string{true: "dark"}[c.Theme().Dark]
	}
	if a.themeKey == key && a.themeCache != nil {
		return a.themeCache
	}
	var t *ui.Theme
	if ok {
		base := ui.LightTheme()
		if p.dark {
			base = ui.DarkTheme()
		}
		th := *base
		th.Background, th.Surface, th.SurfaceHover = ui.Hex(p.bg), ui.Hex(p.surface), ui.Hex(p.hover)
		th.SurfacePressed = ui.Hex(p.hover).Mix(ui.Hex(p.text), 0.06)
		th.Border, th.Text, th.TextMuted = ui.Hex(p.border), ui.Hex(p.text), ui.Hex(p.muted)
		th.Scrollbar = ui.Hex(p.muted).Alpha(0.35)
		accent := p.accent
		if a.st.Accent != "" {
			accent = a.st.Accent
		}
		t = th.WithAccent(ui.Hex(accent))
		// Rows chosen: a little of the accent in the background, soft
		// enough for any text, as in the user's looks.
		t.Selection = ui.Hex(p.bg).Mix(ui.Hex(accent), 0.2)
		if d := a.currentDesign(); d != nil {
			// The user's look: rows chosen as it says, or softly, so that
			// any text reads on them.
			t.Selection = ui.Hex(selectionOf(d))
			if d.Selection != "" {
				t.Selection = ui.Hex(d.Selection)
			}
			// The panels let its background show through.
			if seeThrough(d) {
				t.Surface = t.Surface.Alpha(d.PanelOpacity)
			}
		}
	} else {
		t = c.Theme().WithAccent(ui.Hex(a.st.Accent))
	}
	a.themeKey, a.themeCache = key, t
	return t
}

// termThemes are the terminals' colors as chosen: nil, nil for the
// window's light or dark ones.
func (a *app) termThemes() (*terminal.Theme, *terminal.Theme) {
	if a.st.TermTheme != "" {
		if th, err := terminal.GhosttyTheme(a.st.TermTheme); err == nil {
			return th, th
		}
	}
	if name, ok := strings.CutPrefix(a.st.Palette, "ghostty:"); ok {
		if _, th, ok := ghosttyPalette(name); ok {
			return th, th
		}
	}
	if d := a.currentDesign(); d != nil {
		th := terminal.LightTheme()
		if d.Dark {
			th = terminal.DarkTheme()
		}
		if d.TermTheme != "" {
			if g, err := terminal.GhosttyTheme(d.TermTheme); err == nil {
				copy := *g
				th = &copy
			}
		}
		th.Background, th.Foreground = colorOr(d.TermBg, d.Background), colorOr(d.TermFg, d.Text)
		th.Cursor, th.Selection = colorOr(d.Accent, d.Text), colorOr(d.Accent, d.Text).Alpha(0.35)
		return th, th
	}
	p, ok := paletteByID(a.st.Palette)
	if !ok {
		return nil, nil
	}
	th := terminal.LightTheme()
	if p.dark {
		th = terminal.DarkTheme()
	}
	th.Background, th.Foreground = ui.Hex(p.bg), ui.Hex(p.text)
	th.Cursor = ui.Hex(p.accent)
	if a.st.Accent != "" {
		th.Cursor = ui.Hex(a.st.Accent)
	}
	th.Selection = ui.Hex(p.accent).Alpha(0.35)
	return th, th
}

// applyColors gives the terminals their colors, after a change.
func (a *app) applyColors() {
	a.themeCache, a.termBgSet = nil, false
	light, dark := a.termThemes()
	for _, s := range a.sessions() {
		if s.term != nil {
			s.term.SetTheme(light, dark)
		}
	}
	a.save()
}

// swatch is a small square of a color.
func swatch(c *ui.Context, color string, size float32) ui.Element {
	return ui.Box(c).Width(size).Height(size).Radius(3).Background(ui.Hex(color)).Border(1, c.Theme().Border)
}
