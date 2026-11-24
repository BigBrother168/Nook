package main

import (
	"image"
	"image/color"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"nook/core/config"
)

// A click on a part of the board's small window chooses that part, and
// opens the fine way; a panel colored on its own colors the window's
// panel, the others letting the window show.
func TestDesignerParts(t *testing.T) {
	a := testApp()
	a.st.Palette = "warm"
	d := a.newDesign(ui.DarkTheme())
	a.openDesigner(nil)
	tt := ui.NewTester(a.view, 1300, 900)
	tt.Frame()
	for _, c := range []struct{ click, part string }{{"website", "text"}, {"— my-app", "muted"}, {"● 好的，我先看看 index.html", "termFg"}, {"标签栏", "tabbar"}, {"侧栏", "sidebar"}} {
		if err := tt.Click(c.click); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if a.designPart != c.part {
			t.Errorf("clicking %q chose %q, not %q", c.click, a.designPart, c.part)
		}
	}
	// The sidebar takes a color of its own; the other panels keep theirs.
	a.setPart(d, ui.Hex("#336699"))
	th := a.windowTheme(nil)
	if got := a.panelColor(th, "sidebar"); hex(got) != "#336699" {
		t.Fatalf("sidebar %s", hex(got))
	}
	if got := a.panelColor(th, "panel"); got.A != 0 {
		t.Fatalf("right panel %s, not clear", hex(got))
	}
	if u := usedColors(d); !slices.Contains(u, "#336699") {
		t.Fatalf("used colors %v", u)
	}
	// A Ghostty theme on the terminals.
	termTheme(d, "Dracula")
	if d.TermTheme != "Dracula" || !d.TermOwn || d.TermBg == d.Background {
		t.Fatalf("terminal theme: %+v", d)
	}
	if light, _ := a.termThemes(); light == nil || hex(light.Background) != d.TermBg {
		t.Fatal("the terminals do not take it")
	}
	// Starting over from a palette drops the panels' own colors.
	fromPalette(d, palettes[3])
	if d.Sidebar != "" || d.Background != palettes[3].bg || d.Dark {
		t.Fatalf("from paper: %+v", d)
	}
}

// A dark picture with a red patch gives a dark look with a red accent.
func TestPictureColors(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			c := color.RGBA{20, 24, 40, 255}
			if x < 30 && y < 30 {
				c = color.RGBA{220, 40, 40, 255}
			}
			img.Set(x, y, c)
		}
	}
	avg, vivid, ok := pictureColors(img)
	if !ok || vivid.R < 150 || vivid.G > 100 {
		t.Fatalf("avg %v vivid %v", avg, vivid)
	}
	var d config.Design
	setColors(&d, avg, vivid, ok)
	if !d.Dark || luminance(ui.Hex(d.Text)) < 0.8 || luminance(ui.Hex(d.Background)) > 0.2 {
		t.Fatalf("not a readable dark look: %+v", d)
	}
	randomColors(&d)
	if d.Background == "" || d.Accent == "" || !d.Dark {
		t.Fatalf("random: %+v", d)
	}
}

// The quick way: dragging a color point of the field colors the
// background, and every other color follows; the dial turns the
// gradient; the bar sets how light the terminals are.
func TestQuickColors(t *testing.T) {
	a := testApp()
	a.st.Palette = "warm"
	d := a.newDesign(ui.DarkTheme())
	quickPresets[0].apply(d)
	a.openDesigner(nil)
	tt := ui.NewTester(a.view, 1300, 900)
	tt.Frame()
	field, ok := tt.Find("背景颜色点阵")
	if !ok {
		t.Fatal("no field")
	}
	// Point 1 sits low (dark); dragged to the top, it turns light.
	x, y := fieldPlace(ui.Hex(d.Background))
	px, py := field.X+float32(x*fieldW), field.Y+float32(y*fieldH)
	tt.Press(px, py)
	tt.Frame()
	tt.Move(px+40, field.Y+20)
	tt.Frame()
	tt.Release(px+40, field.Y+20)
	tt.Frame()
	if l := luminance(ui.Hex(d.Background)); l < 0.6 {
		t.Fatalf("background %s after the drag, not light", d.Background)
	}
	if a.quickStop != 0 {
		t.Fatalf("point %d moved, not 1", a.quickStop+1)
	}
	// The text follows: it reads on the background.
	if d.Middle == "" || d.Gradient == "" || d.TitleBar != "" {
		t.Fatalf("not three colors and clear panels: %+v", d)
	}
	// The dial: pressed at its right, the gradient goes right (90°).
	dial, ok := tt.Find("背景的方向")
	if !ok {
		t.Fatal("no dial")
	}
	tt.Press(dial.X+dial.W-2, dial.Y+dial.H/2)
	tt.Frame()
	tt.Release(dial.X+dial.W-2, dial.Y+dial.H/2)
	tt.Frame()
	if d.GradientAngle < 80 || d.GradientAngle > 100 {
		t.Fatalf("angle %v, not about 90", d.GradientAngle)
	}
	// The bar: pressed at its right end, the terminals turn light.
	bar, ok := tt.Find("终端从深到浅")
	if !ok {
		t.Fatal("no bar")
	}
	tt.Press(bar.X+bar.W-3, bar.Y+bar.H/2)
	tt.Frame()
	tt.Release(bar.X+bar.W-3, bar.Y+bar.H/2)
	tt.Frame()
	if luminance(ui.Hex(d.TermBg)) < 0.8 || luminance(ui.Hex(d.TermFg)) > 0.4 {
		t.Fatalf("terminals %s on %s after the bar", d.TermFg, d.TermBg)
	}
}

// Every ready-made look's terminal theme, and each flavor of it, is one
// of Ghostty's; Aurora's terminals are the colors its users liked.
// Changing flavor goes round them, and each look keeps its own.
func TestTermFlavors(t *testing.T) {
	for i, p := range quickPresets {
		for _, name := range append([]string{p.term}, p.flavors...) {
			if ghosttyTheme(name) == nil {
				t.Errorf("%s: no theme %q", p.id, name)
			}
		}
		d := presetLook(i)
		if d.TermBg == "" || d.TermFg == "" || d.TermTheme != p.term {
			t.Errorf("%s: terminals %+v", p.id, d)
		}
		// Text reads on its terminals.
		if diff := luminance(ui.Hex(d.TermBg)) - luminance(ui.Hex(d.TermFg)); diff > -0.35 && diff < 0.35 {
			t.Errorf("%s: %s on %s", p.id, d.TermFg, d.TermBg)
		}
	}
	if d := presetLook(0); d.TermBg != "#182431" || d.TermFg != "#b9cad2" || d.TermTheme != "Nord" {
		t.Fatalf("aurora's terminals %s on %s, %s", d.TermFg, d.TermBg, d.TermTheme)
	}
	a := testApp()
	a.choosePalette("look:aurora")
	a.nextTermFlavor()
	if a.st.TermTheme != quickPresets[0].flavors[0] {
		t.Fatalf("first flavor %q", a.st.TermTheme)
	}
	for range quickPresets[0].flavors {
		a.nextTermFlavor()
	}
	if a.st.TermTheme != "" {
		t.Fatalf("round again: %q", a.st.TermTheme)
	}
	a.nextTermFlavor() // aurora with its first flavor
	a.choosePalette("look:paper")
	if a.st.TermTheme != "" {
		t.Fatalf("paper took aurora's flavor %q", a.st.TermTheme)
	}
	a.choosePalette("look:aurora")
	if a.st.TermTheme != quickPresets[0].flavors[0] {
		t.Fatalf("aurora's flavor not kept: %q", a.st.TermTheme)
	}
	// The board shows a look's own terminals, to change them: a copy of
	// aurora starts without the flavor, and aurora keeps it.
	a.openDesigner(nil)
	if a.st.TermTheme != "" || !strings.HasPrefix(a.st.Palette, "my:") {
		t.Fatalf("on the board: palette %q, flavor %q", a.st.Palette, a.st.TermTheme)
	}
	if a.st.TermFlavors["look:aurora"] != quickPresets[0].flavors[0] {
		t.Fatalf("aurora's flavor lost: %v", a.st.TermFlavors)
	}
}
