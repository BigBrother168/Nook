package main

import (
	"image"
	"image/color"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"nook/core/config"
)

// A click on a part of the board's small window chooses that part; a
// panel colored on its own colors the window's panel.
func TestDesignerParts(t *testing.T) {
	a := testApp()
	a.st.Palette = "warm"
	d := a.newDesign(ui.DarkTheme())
	a.openDesigner(nil)
	tt := ui.NewTester(a.view, 1300, 900)
	tt.Frame()
	for _, c := range []struct{ click, part string }{{"Notes", "text"}, {`E:\Notes`, "muted"}, {"✻ 正在修改首页…", "termFg"}, {"标签栏", "tabbar"}, {"侧栏", "sidebar"}} {
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
	if got := a.panelColor(th, "panel"); hex(got) != hex(th.Surface) {
		t.Fatalf("right panel %s, not the surface", hex(got))
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
