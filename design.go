package main

import (
	"crypto/sha1"
	hexenc "encoding/hex"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	_ "golang.org/x/image/webp"

	"nook/core/config"
)

// The drawing board: the user's own looks. A small Nook shows the look
// being made; a click on a part of it chooses the part to color. Behind
// everything, a gradient or a picture of the user's, the panels and the
// terminals letting it show through as much as they choose.

// designPart is a part of the look the board colors.
type designPart struct {
	id, zh, en string
	field      func(d *config.Design) *string
}

// currentDesign is the user's look chosen, if one is.
func (a *app) currentDesign() *config.Design {
	id, ok := strings.CutPrefix(a.st.Palette, "my:")
	if !ok {
		return nil
	}
	for i := range a.st.Designs {
		if a.st.Designs[i].ID == id {
			return &a.st.Designs[i]
		}
	}
	return nil
}

// designPalette is a look's colors as a palette of Nook's.
func designPalette(d *config.Design) palette {
	return palette{
		id: "my:" + d.ID, zh: d.Name, en: d.Name, dark: d.Dark,
		bg: d.Background, surface: d.Surface, hover: hex(ui.Hex(d.Surface).Mix(ui.Hex(d.Text), 0.07)), border: d.Border,
		text: d.Text, muted: d.Muted, accent: d.Accent,
	}
}

// seeThrough reports whether the look lets something show behind the
// panels and the terminals.
func seeThrough(d *config.Design) bool {
	return d != nil && (d.Image != "" || d.Gradient != "") && d.PanelOpacity > 0 && d.PanelOpacity < 1
}

// panelAlpha is how opaque the panels are in the look.
func panelAlpha(d *config.Design) float32 {
	if !seeThrough(d) {
		return 1
	}
	return d.PanelOpacity
}

// newDesign makes a look of the colors showing now, and chooses it.
func (a *app) newDesign(t *ui.Theme) *config.Design {
	n := len(a.st.Designs) + 1
	d := config.Design{
		ID:         strconv.FormatInt(time.Now().UnixNano(), 36),
		Name:       fmt.Sprintf(L("我的配色 %d", "My look %d"), n),
		Dark:       t.Dark,
		Background: hex(t.Background), Surface: hex(t.Surface), Text: hex(t.Text), Muted: hex(t.TextMuted),
		Border: hex(t.Border), Accent: hex(t.Accent), TermBg: hex(t.Background), TermFg: hex(t.Text),
		GradientAngle: 135, ImageOpacity: 0.45, PanelOpacity: 0.82,
	}
	if light, dark := a.termThemes(); light != nil {
		th := light
		if t.Dark && dark != nil {
			th = dark
		}
		d.TermBg, d.TermFg = hex(th.Background), hex(th.Foreground)
	}
	a.st.Designs = append(a.st.Designs, d)
	a.st.Palette, a.st.Accent = "my:"+d.ID, ""
	a.applyColors()
	return &a.st.Designs[len(a.st.Designs)-1]
}

// openDesigner shows the page making a look, for the look chosen, making one
// when none is.
func (a *app) openDesigner(c *ui.Context) {
	if a.currentDesign() == nil {
		a.newDesign(c.Theme())
	}
	a.designPart = "" // the board starts at the background
	a.viewing = "designer"
}

// designChanged is after an edit of the look: the window and the
// terminals take it, and it is saved at the next tick.
func (a *app) designChanged() {
	a.themeCache = nil
	light, dark := a.termThemes()
	for _, s := range a.sessions() {
		if s.term != nil {
			s.term.SetTheme(light, dark)
		}
	}
	a.layoutChanged = true
}

// backdrop is a look's picture, decoded once.
func (a *app) backdrop(d *config.Design) *ui.Bitmap {
	if d == nil || d.Image == "" {
		return nil
	}
	if a.bgPath != d.Image {
		a.bgPath, a.bgBmp = d.Image, nil
		path := d.Image
		go func() {
			data, err := os.ReadFile(path)
			if err != nil {
				return
			}
			bmp, err := ui.DecodeBitmap(data)
			if err != nil {
				return
			}
			a.update(func() {
				if a.bgPath == path {
					a.bgBmp = bmp
				}
			})
		}()
	}
	return a.bgBmp
}

// windowBackdrop fills the window's root with the look's background:
// its color or gradient, and its picture.
func (a *app) windowBackdrop(c *ui.Context, root ui.Element) {
	d := a.currentDesign()
	if d == nil {
		return
	}
	if d.Gradient != "" {
		root.Gradient(ui.Hex(d.Background), ui.Hex(d.Gradient), d.GradientAngle)
	}
	if bmp := a.backdrop(d); bmp != nil {
		img := ui.Image(c, bmp).Absolute().Top(0).Left(0).Right(0).Bottom(0).Fit(ui.Cover).Opacity(d.ImageOpacity).ZIndex(-1)
		if d.ImageBlur > 0 {
			img.Blur(d.ImageBlur).Cache()
		}
	}
}

// chooseBackground asks for a picture and puts a copy of it behind the
// look; with colors, the look takes its colors too.
func (a *app) chooseBackground(colors bool) {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: L("选一张背景图", "Choose a background picture"),
			Filters: []mygo.FileFilter{{Name: L("图片", "Pictures"), Extensions: []string{"png", "jpg", "jpeg", "webp", "gif", "bmp"}}}})
		if err != nil || len(paths) == 0 {
			return
		}
		kept, err := keepPicture(paths[0])
		a.update(func() {
			d := a.currentDesign()
			if d == nil {
				return
			}
			if err != nil {
				a.notice = L("这张图用不了：", "This picture cannot be used: ") + err.Error()
				return
			}
			d.Image = kept
			if d.ImageOpacity == 0 {
				d.ImageOpacity = 0.45
			}
			if d.PanelOpacity == 0 || d.PanelOpacity == 1 {
				d.PanelOpacity = 0.82
			}
			if colors {
				a.colorsOfPicture(d)
			}
			a.designChanged()
		})
	}()
}

// keepPicture copies a picture into Nook's folder, so that it stays
// though the original moves.
func keepPicture(path string) (string, error) {
	src, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer src.Close()
	if _, _, err := image.DecodeConfig(src); err != nil {
		return "", err
	}
	src.Seek(0, io.SeekStart)
	h := sha1.New()
	io.Copy(h, src)
	dir := filepath.Join(config.Dir(), "backgrounds")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, hexenc.EncodeToString(h.Sum(nil))[:12]+strings.ToLower(filepath.Ext(path)))
	if _, err := os.Stat(dst); err == nil {
		return dst, nil
	}
	src.Seek(0, io.SeekStart)
	out, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(out, src); err != nil {
		out.Close()
		return "", err
	}
	return dst, out.Close()
}

// colorsOfPicture gives the look colors that go with its picture.
func (a *app) colorsOfPicture(d *config.Design) {
	f, err := os.Open(d.Image)
	if err != nil {
		return
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		a.notice = L("没能从这张图取色：", "No colors from this picture: ") + err.Error()
		return
	}
	avg, vivid, ok := pictureColors(img)
	setColors(d, avg, vivid, ok)
}

// pictureColors finds a picture's average color and its most vivid hue.
func pictureColors(img image.Image) (avg, vivid ui.Color, found bool) {
	b := img.Bounds()
	step := max(1, max(b.Dx(), b.Dy())/96)
	var sum [3]float64
	n := 0.0
	type bucket struct{ w, r, g, b float64 }
	var hues [12]bucket
	for y := b.Min.Y; y < b.Max.Y; y += step {
		for x := b.Min.X; x < b.Max.X; x += step {
			r16, g16, b16, a16 := img.At(x, y).RGBA()
			if a16 < 0x8000 {
				continue
			}
			r, g, bl := float64(r16>>8), float64(g16>>8), float64(b16>>8)
			sum[0], sum[1], sum[2] = sum[0]+r, sum[1]+g, sum[2]+bl
			n++
			h, s, v := hsv(r/255, g/255, bl/255)
			if s < 0.25 || v < 0.25 {
				continue
			}
			w := s * v
			k := int(h/30) % 12
			hues[k].w += w
			hues[k].r, hues[k].g, hues[k].b = hues[k].r+r*w, hues[k].g+g*w, hues[k].b+bl*w
		}
	}
	if n == 0 {
		return ui.Color{}, ui.Color{}, false
	}
	avg = ui.RGB(uint8(sum[0]/n), uint8(sum[1]/n), uint8(sum[2]/n))
	best := -1
	for i, h := range hues {
		if h.w > 0 && (best < 0 || h.w > hues[best].w) {
			best = i
		}
	}
	if best < 0 {
		return avg, ui.Color{}, true
	}
	h := hues[best]
	return avg, ui.RGB(uint8(h.r/h.w), uint8(h.g/h.w), uint8(h.b/h.w)), true
}

// setColors colors a look from a picture's average color and vivid one.
func setColors(d *config.Design, avg, vivid ui.Color, found bool) {
	if !found {
		return
	}
	black, white := ui.RGB(10, 10, 12), ui.RGB(250, 250, 248)
	d.Dark = luminance(avg) < 0.55
	var bg, text ui.Color
	if d.Dark {
		bg, text = avg.Mix(black, 0.78), white.Mix(avg, 0.08)
	} else {
		bg, text = avg.Mix(white, 0.82), black.Mix(avg, 0.12)
	}
	accent := vivid
	if accent.A == 0 {
		accent = ui.Hex("#5aa9e6")
	}
	if d.Dark && luminance(accent) < 0.45 {
		accent = accent.Mix(white, 0.35)
	} else if !d.Dark && luminance(accent) > 0.6 {
		accent = accent.Mix(black, 0.35)
	}
	d.Background, d.Surface = hex(bg), hex(bg.Mix(text, 0.05))
	d.Text, d.Muted, d.Border = hex(text), hex(text.Mix(bg, 0.42)), hex(bg.Mix(text, 0.16))
	d.Accent = hex(accent)
	if !d.TermOwn {
		d.TermBg, d.TermFg = hex(bg), hex(text)
	}
	if d.Gradient != "" {
		d.Gradient = hex(bg.Mix(accent, 0.25))
	}
}

// randomColors gives the look colors of a random hue, dark or light as
// it is.
func randomColors(d *config.Design) {
	h := rand.Float64() * 360
	acc := math.Mod(h+[]float64{0, 30, 150, 180, 210}[rand.IntN(5)], 360)
	var bg, surface, text ui.Color
	if d.Dark {
		bg, surface, text = hsl(h, 0.2, 0.1), hsl(h, 0.2, 0.135), hsl(h, 0.12, 0.92)
	} else {
		bg, surface, text = hsl(h, 0.25, 0.97), hsl(h, 0.22, 0.93), hsl(h, 0.2, 0.16)
	}
	accent := hsl(acc, 0.65, map[bool]float64{true: 0.64, false: 0.42}[d.Dark])
	d.Background, d.Surface, d.Text = hex(bg), hex(surface), hex(text)
	d.Muted, d.Border, d.Accent = hex(text.Mix(bg, 0.42)), hex(bg.Mix(text, 0.16)), hex(accent)
	d.TermBg, d.TermFg = hex(bg), hex(text)
	if d.Gradient != "" {
		d.Gradient = hex(bg.Mix(accent, 0.3))
	}
}

func hsv(r, g, b float64) (h, s, v float64) {
	mx, mn := max(r, g, b), min(r, g, b)
	v = mx
	if mx == 0 {
		return 0, 0, 0
	}
	s = (mx - mn) / mx
	d := mx - mn
	switch {
	case d == 0:
		h = 0
	case mx == r:
		h = math.Mod((g-b)/d, 6) * 60
	case mx == g:
		h = ((b-r)/d + 2) * 60
	default:
		h = ((r-g)/d + 4) * 60
	}
	if h < 0 {
		h += 360
	}
	return
}

func hsl(h, s, l float64) ui.Color {
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - c/2
	var r, g, b float64
	switch {
	case h < 60:
		r, g = c, x
	case h < 120:
		r, g = x, c
	case h < 180:
		g, b = c, x
	case h < 240:
		g, b = x, c
	case h < 300:
		r, b = x, c
	default:
		r, b = c, x
	}
	return ui.RGB(uint8((r+m)*255), uint8((g+m)*255), uint8((b+m)*255))
}
