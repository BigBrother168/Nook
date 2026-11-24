package main

import (
	"math"
	"slices"
	"strings"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"nook/core/config"
)

// The drawing board: a small Nook in the look being made; a click on any
// part of it chooses the part, which the picker on the right colors. Each
// panel can take a color of its own, the terminals a theme of Ghostty's;
// a look can start from any palette or theme, and the colors the look
// uses already are a click away for another part.

// boardParts are the parts the board colors, as the chips under the
// small window list them; fallback is the color a part shows while it
// has none of its own.
type boardPart struct {
	designPart
	fallback func(d *config.Design) string
}

// bgOf is what a panel shows while it has no color of its own: the
// window's background, through it.
func bgOf(d *config.Design) string { return d.Background }

// middleOf is the gradient's middle while it has none of its own:
// halfway between its ends.
func middleOf(d *config.Design) string {
	if d.Gradient == "" {
		return d.Background
	}
	return hex(ui.Hex(d.Background).Mix(ui.Hex(d.Gradient), 0.5))
}

// selectionOf is the rows chosen when the look gives them no color: a
// little of the accent in the background, which any text reads on.
func selectionOf(d *config.Design) string {
	return hex(ui.Hex(d.Background).Mix(ui.Hex(d.Accent), 0.22))
}

var boardParts = []boardPart{
	{designPart{"bg", "背景颜色 1", "Background color 1", func(d *config.Design) *string { return &d.Background }}, nil},
	{designPart{"middle", "背景颜色 2", "Background color 2", func(d *config.Design) *string { return &d.Middle }}, middleOf},
	{designPart{"gradient", "背景颜色 3", "Background color 3", func(d *config.Design) *string { return &d.Gradient }}, nil},
	{designPart{"titlebar", "标题栏", "Title bar", func(d *config.Design) *string { return &d.TitleBar }}, bgOf},
	{designPart{"sidebar", "侧栏", "Sidebar", func(d *config.Design) *string { return &d.Sidebar }}, bgOf},
	{designPart{"tabbar", "标签栏", "Tab bar", func(d *config.Design) *string { return &d.TabBar }}, bgOf},
	{designPart{"panel", "右侧面板", "Right panel", func(d *config.Design) *string { return &d.Panel }}, bgOf},
	{designPart{"surface", "卡片和菜单", "Cards and menus", func(d *config.Design) *string { return &d.Surface }}, nil},
	{designPart{"text", "文字", "Text", func(d *config.Design) *string { return &d.Text }}, nil},
	{designPart{"muted", "次要文字", "Secondary text", func(d *config.Design) *string { return &d.Muted }}, nil},
	{designPart{"border", "边框和分隔线", "Borders", func(d *config.Design) *string { return &d.Border }}, nil},
	{designPart{"accent", "强调色", "Accent", func(d *config.Design) *string { return &d.Accent }}, nil},
	{designPart{"selection", "选中的行", "Rows chosen", func(d *config.Design) *string { return &d.Selection }}, selectionOf},
	{designPart{"termBg", "终端背景", "Terminal background", func(d *config.Design) *string { return &d.TermBg }}, nil},
	{designPart{"termFg", "终端文字", "Terminal text", func(d *config.Design) *string { return &d.TermFg }}, nil},
}

func boardPartByID(id string) boardPart {
	for _, p := range boardParts {
		if p.id == id {
			return p
		}
	}
	return boardParts[0]
}

// value is the color a part shows: its own, else its fallback's, else
// the background's.
func (p boardPart) value(d *config.Design) string {
	if v := *p.field(d); v != "" {
		return v
	}
	if p.fallback != nil {
		return p.fallback(d)
	}
	return d.Background
}

// panelColor is the background of panel id (titlebar, sidebar, tabbar,
// panel) in the look chosen: its own color, else none, the window's
// showing.
func (a *app) panelColor(t *ui.Theme, id string) ui.Color {
	if d := a.currentDesign(); d != nil {
		if v := *boardPartByID(id).field(d); v != "" {
			return ui.Hex(v).Alpha(panelAlpha(d))
		}
	}
	return ui.Transparent
}

// pickPart chooses the part of the look to color.
func (a *app) pickPart(d *config.Design, id string) {
	a.designPart = id
	a.designColor = ui.Hex(boardPartByID(id).value(d))
}

// setPart colors the part chosen.
func (a *app) setPart(d *config.Design, col ui.Color) {
	p := boardPartByID(a.designPart)
	*p.field(d) = hexA(col)
	if p.id == "termBg" || p.id == "termFg" {
		d.TermOwn = true
	}
	a.designColor = col
	a.designChanged()
}

// fromPalette starts the look over from palette p: every color of it,
// the panels' own going.
func fromPalette(d *config.Design, p palette) {
	d.Dark = p.dark
	d.Background, d.Surface, d.Text, d.Muted, d.Border, d.Accent = p.bg, p.surface, p.text, p.muted, p.border, p.accent
	d.TitleBar, d.Sidebar, d.TabBar, d.Panel = "", "", "", ""
	d.TermBg, d.TermFg, d.TermOwn = p.bg, p.text, false
	if d.Gradient != "" {
		d.Gradient = hex(ui.Hex(p.bg).Mix(ui.Hex(p.accent), 0.3))
	}
}

// termTheme puts Ghostty's theme name on the look's terminals.
func termTheme(d *config.Design, name string) {
	th, err := terminal.GhosttyTheme(name)
	if err != nil {
		return
	}
	d.TermTheme, d.TermOwn = name, true
	d.TermBg, d.TermFg = hex(th.Background), hex(th.Foreground)
}

// usedColors are the colors the look uses, each once.
func usedColors(d *config.Design) []string {
	var out []string
	for _, p := range boardParts {
		v := strings.ToLower(*p.field(d))
		if v != "" && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

// designerBody is the drawing board, in the settings' dialog: the look
// on a small window, the looks to start from under it, and beside, the
// quick way to color it, then the fine one, part by part. The window
// behind the dialog takes every change as it is made.
func (a *app) designerBody(c *ui.Context) {
	t := c.Theme()
	d := a.currentDesign()
	if d == nil {
		a.settings = "settings"
		return
	}
	if a.designPart == "" {
		a.pickPart(d, "bg")
	}
	part := boardPartByID(a.designPart)
	// Back goes to the settings' appearance, where the board opens from.
	back := func() {
		a.save()
		if a.designFrom {
			a.settings, a.designFrom = "settings", false
			return
		}
		a.leavePages()
	}
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).FillWidth().Height(48).PaddingStart(8).PaddingEnd(10).Gap(8).AlignItems(ui.Center).Children(func() {
			label := L("返回", "Back")
			if a.designFrom {
				label = L("外观", "Appearance")
			}
			if backButton(c, label) {
				back()
			}
			ui.Text(c, L("画板", "Drawing board")).FontSize(15).Bold().Padding(0, 0, 0, 4)
			if ui.TextInput(c, &d.Name).Width(160).Label(L("这套配色的名字", "The look's name")).Changed() {
				a.layoutChanged = true
			}
			ui.Spacer(c)
			if smallButton(c, L("随机一套", "Surprise me"), false).Tooltip(L("随便来一套搭配好的颜色", "A random set of colors that go together")).Clicked() {
				a.designShade = randomQuick(d)
				a.pickPart(d, a.designPart)
				a.designChanged()
			}
			if smallButton(c, L("复制一套", "Duplicate"), false).Tooltip(L("复制成新的一套接着改，原来那套留着", "A copy to go on with; this one stays")).Clicked() {
				a.copyDesign(d, "")
				return // the looks moved: the next frame finds the copy
			}
			if iconButton(c, iconTrash, L("删除这套", "Delete this look")).Clicked() {
				id, name := d.ID, d.Name
				a.confirm = &confirmAsk{title: L("删除这套配色？", "Delete this look?"), text: name, yes: L("删除", "Delete"), danger: true, do: func() {
					a.st.Designs = slices.DeleteFunc(a.st.Designs, func(x config.Design) bool { return x.ID == id })
					a.choosePalette("")
					delete(a.st.TermFlavors, "my:"+id)
					a.settings, a.designFrom = "settings", false
				}}
			}
			if iconButton(c, iconX, L("关闭（Esc）", "Close (Esc)")).Clicked() {
				a.leavePages()
			}
		})
		ui.Box(c).FillWidth().Height(1).Background(t.Border)
		ui.Row(c).FillWidth().Grow(1).Children(func() {
			ui.Scroll(c).Grow(1).Shrink(1).FillHeight().Children(func() {
				ui.Column(c).FillWidth().Padding(18).Gap(12).Children(func() {
					a.designPreview(c, d)
					ui.Text(c, L("小窗口上的 1、2、3 是背景的三个颜色所在的地方；点小窗口别的地方，可以在「细调」里单独给它上色。", "1, 2, 3 on the small window are where the background's three colors are; click elsewhere on it to color that part alone, under “Fine”.")).FontSize(12).TextColor(t.TextMuted).Wrap()
					ui.Text(c, L("现成的配色，点一下套用", "Ready-made: click one to use it")).FontSize(12).Bold().TextColor(t.TextMuted).Padding(6, 0, 0, 0)
					ui.Row(c).FillWidth().Wrap().Gap(10).Children(func() {
						for i, p := range quickPresets {
							pd := presetLook(i)
							if lookCard(c, "preset:"+p.id, L(p.zh, p.en), false, designFace(pd), L(p.descZh, p.descEn)) {
								keep := *d
								p.apply(d)
								d.ID, d.Name, d.Image, d.ImageOpacity, d.ImageBlur = keep.ID, keep.Name, keep.Image, keep.ImageOpacity, keep.ImageBlur
								a.designShade = shadeOf(d)
								a.pickPart(d, a.designPart)
								a.designChanged()
							}
						}
					})
				})
			})
			ui.Box(c).Width(1).FillHeight().Background(t.Border)
			ui.Scroll(c).Width(340).FillHeight().Shrink(0).Children(func() {
				ui.Column(c).FillWidth().Padding(16).Gap(12).Children(func() {
					a.quickControls(c, d)
					ui.Box(c).FillWidth().Height(1).Background(t.Border.Alpha(0.6))
					fine := ui.Row(c).FillWidth().Gap(6).PaddingY(2).AlignItems(ui.Center).Cursor(ui.CursorPointer)
					fine.Children(func() {
						ui.Text(c, L("细调", "Fine")).FontSize(13).Bold()
						ui.Text(c, L("标题栏、侧栏、文字……一块块单独上色", "Title bar, sidebar, text…: each part a color of its own")).FontSize(12).TextColor(t.TextMuted).Grow(1).Shrink(1)
						icon := iconChevronRight
						if a.designFine {
							icon = iconChevron
						}
						ui.Icon(c, icon).FontSize(14).TextColor(t.TextMuted)
					})
					if fine.Clicked() {
						a.designFine = !a.designFine
					}
					if a.designFine {
						a.designControls(c, d, part)
					}
				})
			})
		})
	})
}

// quickSwatches are colors a click gives the background color chosen.
var quickSwatches = []string{"#f4f1ec", "#0b0f14", "#1b2a4a", "#132e37", "#2a1d3d", "#4a8fe0", "#3fa9a3", "#5fbf73", "#e8c75a", "#e0605a", "#e58fb0"}

// stopNames are where the background's three colors are, as the
// direction places them: the first opposite where the gradient goes, the
// last where it goes; from a point, its middle, a ring, the edges.
func stopNames(d *config.Design) [3]string {
	if d.Radial {
		return [3]string{L("中心", "Center"), L("中圈", "Ring"), L("外圈", "Edges")}
	}
	dirs := []string{L("上", "Top"), L("右上", "Top right"), L("右", "Right"), L("右下", "Bottom right"), L("下", "Bottom"), L("左下", "Bottom left"), L("左", "Left"), L("左上", "Top left")}
	at := func(angle float64) string {
		return dirs[int(math.Round(math.Mod(angle+360, 360)/45))%8]
	}
	angle := float64(d.GradientAngle)
	return [3]string{at(angle + 180), L("中间", "Middle"), at(angle)}
}

// stopSpots are where the background's three colors show on a box, in
// fractions of its width and height: along the gradient's line, or from
// its point outward.
func stopSpots(d *config.Design) [3]ui.Point {
	r := float64(d.GradientAngle) * math.Pi / 180
	dx, dy := math.Sin(r), -math.Cos(r)
	if !d.Radial {
		at := func(k float64) ui.Point { return ui.Point{X: float32(0.5 + k*dx), Y: float32(0.5 + k*dy)} }
		return [3]ui.Point{at(-0.36), at(0), at(0.36)}
	}
	ctr := radialCenter(d.GradientAngle)
	// The far side, where the colors end.
	fx, fy := float32(0.08), float32(0.12)
	if ctr.X < 0.5 {
		fx = 0.92
	}
	if ctr.Y < 0.5 {
		fy = 0.88
	}
	mid := ui.Point{X: ctr.X + (fx-ctr.X)*0.45, Y: ctr.Y + (fy-ctr.Y)*0.45}
	return [3]ui.Point{ctr, mid, {X: fx, Y: fy}}
}

// stopDot is the round mark of background color i, numbered.
func stopDot(c *ui.Context, col ui.Color, i int, size float32) ui.Element {
	ring := float32(2)
	if size >= 28 {
		ring = 3
	}
	return ui.Box(c).Width(size).Height(size).Radius(size/2).Background(col).Border(ring, ui.RGBA(255, 255, 255, 0.92)).
		Shadow(0, 2, 6, 0, ui.RGBA(0, 0, 0, 0.45)).Center().
		Children(func() { ui.Text(c, itoa(i+1)).FontSize(min(11, size/2.4)).Bold().TextColor(contrastOn(col)) })
}

// quickControls color the look the quick way. The background is three
// colors, one at each of three places of the window: choose a place, then
// its color, on the field of dots or among a few; then how the colors
// blend and which way; and how light the terminals are. Every other color
// follows.
func (a *app) quickControls(c *ui.Context, d *config.Design) {
	t := c.Theme()
	stops := quickStops(d)
	names := stopNames(d)
	a.quickStop = min(max(a.quickStop, 0), 2)
	recolor := false
	title := func(s, help string) {
		ui.Column(c).Gap(2).Children(func() {
			ui.Text(c, s).FontSize(13).Bold()
			if help != "" {
				ui.Text(c, help).FontSize(11).TextColor(t.TextMuted).Wrap()
			}
		})
	}

	title(L("背景颜色", "Background colors"), L("背景从 1 过渡到 2 再到 3。先点一个，再在下面选它的颜色。", "The background goes from 1 to 2 to 3. Choose one, then its color below."))
	ui.Row(c).FillWidth().Gap(6).Children(func() {
		for i := range 3 {
			c.Key("stop:" + itoa(i))
			label := itoa(i+1) + " " + names[i]
			b := ui.ButtonBase(c).Label(label).Grow(1).Shrink(1).Basis(0).Height(34).Radius(8).Border(1, t.Border)
			switch {
			case i == a.quickStop:
				b.Border(2, t.Accent).Background(t.Text.Alpha(0.06))
			case b.Hovered():
				b.Background(t.Text.Alpha(0.04))
			}
			b.Children(func() {
				ui.Row(c).FillWidth().Justify(ui.Center).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Width(14).Height(14).Radius(7).Background(stops[i]).Border(1, t.Text.Alpha(0.25))
					txt := ui.Text(c, label).FontSize(12).SingleLine()
					if i != a.quickStop {
						txt.TextColor(t.TextMuted)
					}
				})
			})
			if b.Clicked() {
				a.quickStop = i
			}
		}
	})

	// The field: left to right the hues, top to bottom light to dark. A
	// press, or a drag, gives the one chosen the color under the pointer.
	f := ui.Box(c).Width(fieldW).Height(fieldH).Radius(10).Clip().Background(ui.Hex("#0d1419")).Cursor(ui.CursorCrosshair).
		Label(L("背景颜色点阵", "The field of colors"))
	if f.Pressed() {
		x, y, _ := f.PointerPosition()
		stops[a.quickStop] = fieldColor(float64(x)/fieldW, float64(y)/fieldH)
		recolor = true
	}
	f.Children(func() {
		ui.Image(c, dotField()).Size(fieldW, fieldH)
		// The others small, the one chosen over them.
		for _, i := range []int{(a.quickStop + 1) % 3, (a.quickStop + 2) % 3, a.quickStop} {
			c.Key(i)
			sx, sy := fieldPlace(stops[i])
			size := float32(16)
			if i == a.quickStop {
				size = 30
			}
			e := stopDot(c, stops[i], i, size).Absolute().Left(float32(sx*fieldW) - size/2).Top(float32(sy*fieldH) - size/2)
			if i != a.quickStop {
				e.Opacity(0.8)
			}
		}
	})
	ui.Row(c).FillWidth().Children(func() {
		ui.Text(c, L("← 左右换颜色 →　↑ 上浅下深 ↓", "← hue →　↑ light, dark ↓")).FontSize(11).TextColor(t.TextMuted)
	})

	// A few colors, a click away.
	ui.Row(c).FillWidth().Wrap().Gap(6).Children(func() {
		for _, col := range quickSwatches {
			c.Key("sw:" + col)
			b := ui.ButtonBase(c).Label(col).Width(20).Height(20).Radius(10).Background(ui.Hex(col)).Border(1, t.Text.Alpha(0.18)).Tooltip(col)
			if b.Clicked() {
				stops[a.quickStop] = ui.Hex(col)
				recolor = true
			}
		}
	})

	// How the colors blend, and which way.
	title(L("过渡", "Blend"), L("直线：从一边到另一边；圆形：从一个点散开。转动圆盘换方向。", "Linear: side to side; radial: out from a point. Turn the dial for the direction."))
	ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
		sel := 0
		if d.Radial {
			sel = 1
		}
		if i, ok := segmented(c, sel, L("直线", "Linear"), L("圆形", "Radial")); ok {
			d.Radial = i == 1
			a.designChanged()
		}
		ui.Spacer(c)
		a.dial(c, d, stops[0])
	})

	// How light the terminals are.
	title(L("终端底色", "Terminal background"), "")
	ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, L("深", "Dark")).FontSize(11).TextColor(t.TextMuted)
		bar := ui.Box(c).Grow(1).Height(22).Radius(7).Cursor(ui.CursorPointer).Label(L("终端从深到浅", "The terminals, dark to light")).
			LinearGradient(ui.LinearGradient{From: ui.RGB(8, 11, 15), To: ui.RGB(246, 247, 248), Angle: 90})
		bw := bar.Bounds().W
		if bw <= 22 {
			bw = fieldW - 50
		}
		if bar.Pressed() {
			bx, _, _ := bar.PointerPosition()
			a.designShade = min(max(float64(bx-11)/float64(bw-22), 0), 1)
			recolor = true
		}
		bar.Children(func() {
			ui.Box(c).Absolute().Left(float32(a.designShade)*(bw-22)).Top(0).Width(22).Height(22).Radius(7).
				Background(ui.Hex(d.TermBg)).Border(2, ui.RGB(255, 255, 255)).Shadow(0, 1, 4, 0, ui.RGBA(0, 0, 0, 0.5)).Center().
				Children(func() { ui.Text(c, "A").FontSize(11).Bold().TextColor(ui.Hex(d.TermFg)) })
		})
		ui.Text(c, L("浅", "Light")).FontSize(11).TextColor(t.TextMuted)
	})

	if recolor {
		setQuickStops(d, stops)
		quickLook(d, a.designShade)
		a.pickPart(d, a.designPart)
		a.designChanged()
	}
}

// dial turns the background's direction: the dot points where it goes.
func (a *app) dial(c *ui.Context, d *config.Design, fill ui.Color) {
	t := c.Theme()
	const size, inner = 48, 30
	k := ui.Box(c).Width(size).Height(size).Radius(size/2).Border(1, t.Border).Cursor(ui.CursorPointer).Label(L("背景的方向", "The background's direction")).
		Tooltip(L("拖动转方向", "Drag to turn"))
	if k.Pressed() {
		x, y, _ := k.PointerPosition()
		if math.Hypot(float64(x-size/2), float64(y-size/2)) > 3 {
			ang := math.Atan2(float64(x-size/2), -float64(y-size/2)) * 180 / math.Pi
			d.GradientAngle = float32(math.Mod(ang+360, 360))
			a.designChanged()
		}
	}
	r := float64(d.GradientAngle) * math.Pi / 180
	k.Children(func() {
		ui.Box(c).Absolute().Left((size-inner)/2).Top((size-inner)/2).Width(inner).Height(inner).Radius(inner/2).
			Background(fill).Border(2, t.Text.Alpha(0.7))
		ui.Box(c).Absolute().Left(float32(size/2+18*math.Sin(r)) - 3.5).Top(float32(size/2-18*math.Cos(r)) - 3.5).
			Width(7).Height(7).Radius(3.5).Background(t.Accent)
	})
}

// designControls are the fine way: a part of the look chosen (on the
// small window, or here) and its color; the colors used already; the
// controls' look; the terminals' theme; the picture behind.
func (a *app) designControls(c *ui.Context, d *config.Design, part boardPart) {
	t := c.Theme()
	heading := func(s string) { ui.Text(c, s).FontSize(12).Bold().TextColor(t.TextMuted).Margin(6, 0, 0, 0) }
	ui.Row(c).FillWidth().Wrap().Gap(6).Children(func() {
		for _, p := range boardParts {
			if (p.id == "gradient" || p.id == "middle") && d.Gradient == "" {
				continue
			}
			c.Key("part:" + p.id)
			b := ui.ButtonBase(c).Label(L(p.zh, p.en)).Padding(3, 9).Radius(12).Border(1, t.Border)
			if p.id == part.id {
				b.Border(2, t.Accent)
			} else if b.Hovered() {
				b.Background(t.SurfaceHover)
			}
			b.Children(func() {
				ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
					swatch(c, p.value(d), 11)
					ui.Text(c, L(p.zh, p.en)).FontSize(11)
				})
			})
			if b.Clicked() {
				a.pickPart(d, p.id)
			}
		}
	})
	heading(L("正在改：", "Coloring: ") + L(part.zh, part.en))
	if ui.ColorPicker(c, &a.designColor).Changed() {
		a.setPart(d, a.designColor)
	}
	if part.fallback != nil && *part.field(d) != "" {
		label := L("透明，露出背景", "Clear: the background shows")
		switch part.id {
		case "selection":
			label = L("自动（跟着强调色）", "Automatic (from the accent)")
		case "middle":
			label = L("自动（两头的中间）", "Automatic (halfway)")
		}
		if smallButton(c, label, false).Clicked() {
			*part.field(d) = ""
			a.pickPart(d, part.id)
			a.designChanged()
		}
	}

	heading(L("这套里用过的颜色", "Colors in this look"))
	ui.Row(c).FillWidth().Wrap().Gap(6).Children(func() {
		for _, col := range usedColors(d) {
			c.Key("used:" + col)
			dot := ui.ButtonBase(c).Label(col).Width(22).Height(22).Radius(6).Background(ui.Hex(col)).Border(1, t.Border).Tooltip(L("用到正在改的部分：", "Use for the part chosen: ") + col)
			if dot.Clicked() {
				a.setPart(d, ui.Hex(col))
			}
		}
	})

	heading(L("按钮和输入框", "Buttons and fields"))
	sel := 1
	if d.Dark {
		sel = 0
	}
	if i, ok := segmented(c, sel, L("深色", "Dark"), L("浅色", "Light")); ok {
		d.Dark = i == 0
		a.designChanged()
	}

	heading(L("终端里程序输出的颜色", "Colors of the terminals' output"))
	ui.Row(c).Gap(6).Wrap().AlignItems(ui.Center).Children(func() {
		label := L("没有套用", "None")
		if d.TermTheme != "" {
			label = d.TermTheme
		}
		ui.Text(c, label).FontSize(12)
		if smallButton(c, L("挑一套…", "Pick…"), false).Tooltip(L("连程序输出的颜色一起换", "Program output's colors too")).Clicked() {
			a.themeFor, a.themeQuery = "designTerm", ""
		}
		if d.TermTheme != "" && smallButton(c, L("不用了", "None"), false).Clicked() {
			d.TermTheme = ""
			a.designChanged()
		}
	})

	heading(L("背景", "Background"))
	grad := d.Gradient != ""
	var gradChanged bool
	ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
		ui.Text(c, L("渐变", "Gradient")).FontSize(12).Grow(1)
		gradChanged = ui.Switch(c, &grad).Label(L("渐变", "Gradient")).Changed()
	})
	if gradChanged {
		if grad {
			d.Gradient = hex(ui.Hex(d.Background).Mix(ui.Hex(d.Accent), 0.3))
			d.Middle = ""
		} else {
			d.Gradient, d.Middle = "", ""
			if a.designPart == "gradient" || a.designPart == "middle" {
				a.pickPart(d, "bg")
			}
		}
		a.designChanged()
	}
	slider := func(label string, v *float32, lo, hi float64) {
		ui.Text(c, label).FontSize(12)
		f := float64(*v)
		if ui.Slider(c, &f, lo, hi).FillWidth().Changed() {
			*v = float32(f)
			a.designChanged()
		}
	}
	ui.Row(c).Gap(6).Wrap().Children(func() {
		if smallButton(c, L("选背景图…", "Picture…"), false).Clicked() {
			a.chooseBackground(false)
		}
		if smallButton(c, L("选图并按它配色…", "Picture and its colors…"), false).Tooltip(L("选一张图当背景，整套颜色跟着这张图走", "A background picture, and colors that go with it")).Clicked() {
			a.chooseBackground(true)
		}
		if d.Image != "" && smallButton(c, L("去掉图片", "No picture"), false).Clicked() {
			d.Image = ""
			a.designChanged()
		}
	})
	if d.Image != "" {
		slider(L("图片清楚程度", "Picture strength"), &d.ImageOpacity, 0.05, 1)
		slider(L("图片模糊", "Picture blur"), &d.ImageBlur, 0, 30)
	}
	if d.Image != "" || d.Gradient != "" {
		if d.PanelOpacity == 0 {
			d.PanelOpacity = 1
		}
		slider(L("终端和卡片的不透明程度（往左越透）", "Terminals' and cards' opacity (left: see-through)"), &d.PanelOpacity, 0.3, 1)
		ui.Text(c, L("终端要新开一个才会透明。", "Terminals turn see-through once opened again.")).FontSize(11).TextColor(t.TextMuted).Wrap()
	}
	if smallButton(c, L("从一套 Ghostty 主题开始…", "Start from a Ghostty theme…"), false).Clicked() {
		a.themeFor, a.themeQuery = "design", ""
	}
}

// designPreview is a small Nook in the look's colors, as the window
// floats: its background around, the terminals on a card. A click on a
// part chooses it, for the fine way to color it.
func (a *app) designPreview(c *ui.Context, d *config.Design) {
	col := func(id string) ui.Color { return ui.Hex(boardPartByID(id).value(d)) }
	own := func(id string) ui.Color { // a panel's own color, else none
		if v := *boardPartByID(id).field(d); v != "" {
			return ui.Hex(v).Alpha(panelAlpha(d))
		}
		return ui.Transparent
	}
	text, muted, border, accent := col("text"), col("muted"), col("border"), col("accent")
	mark := ui.RGBA(255, 255, 255, 0.9)
	if !d.Dark {
		mark = ui.RGBA(0, 0, 0, 0.75)
	}
	// region is a part of the small window that chooses part id.
	region := func(e ui.Element, id string) ui.Element {
		p := boardPartByID(id)
		e.Cursor(ui.CursorPointer).Tooltip(L(p.zh, p.en))
		if a.designPart == id && a.designFine {
			e.Border(2, mark)
		}
		if e.Clicked() {
			a.pickPart(d, id)
			a.designFine = true
		}
		return e
	}
	word := func(s string, color ui.Color, size float32, id string) {
		region(ui.Text(c, s).FontSize(size).TextColor(color).Padding(1, 3).Radius(3).SingleLine(), id)
	}
	box := region(ui.Column(c).FillWidth().MaxWidth(860).AspectRatio(1.7).Radius(12).Clip().Border(1, border), "bg")
	paintBackground(box, d)
	bw, bh := box.Bounds().W, box.Bounds().H
	box.Children(func() {
		// Where the background's three colors are: a click chooses one.
		defer func() {
			if a.designFine || bw == 0 {
				return
			}
			stops := quickStops(d)
			for i, p := range stopSpots(d) {
				c.Key("spot:" + itoa(i))
				size := float32(22)
				if i == a.quickStop {
					size = 28
				}
				dot := stopDot(c, stops[i], i, size).Absolute().Left(p.X*bw - size/2).Top(p.Y*bh - size/2).ZIndex(5).Cursor(ui.CursorPointer).
					Tooltip(L("背景颜色 ", "Background color ") + itoa(i+1))
				if dot.Clicked() {
					a.quickStop = i
				}
			}
		}()
		if bmp := a.backdrop(d); bmp != nil {
			img := ui.Image(c, bmp).Absolute().Top(0).Left(0).Right(0).Bottom(0).Fit(ui.Cover).Opacity(d.ImageOpacity).ZIndex(-1)
			if d.ImageBlur > 0 {
				img.Blur(d.ImageBlur / 2).Cache()
			}
		}
		// The title bar.
		region(ui.Row(c).FillWidth().Height(28).PaddingX(10).Gap(6).AlignItems(ui.Center).Background(own("titlebar")), "titlebar").Children(func() {
			word("Nook", text, 11, "text")
			word("— my-app", muted, 10, "muted")
			ui.Spacer(c)
			region(ui.Row(c).Height(16).PaddingX(7).Gap(4).Radius(8).Border(1, border).AlignItems(ui.Center), "border").Children(func() {
				ui.Box(c).Width(5).Height(5).Radius(3).Background(ui.Hex("#3ccf7a"))
				ui.Text(c, L("真实 IP 没露", "Real IP hidden")).FontSize(8).TextColor(text)
			})
		})
		ui.Row(c).FillWidth().Grow(1).Children(func() {
			// The sidebar.
			region(ui.Column(c).Width(150).FillHeight().Padding(6, 8).Gap(4).Background(own("sidebar")), "sidebar").Children(func() {
				word(L("项目", "Projects"), muted, 9, "muted")
				for i, name := range []string{"my-app", "website"} {
					c.Key(i)
					ui.Row(c).Gap(6).AlignItems(ui.Center).Padding(2).Children(func() {
						region(ui.Box(c).Width(16).Height(16).Radius(4).Background(accent.Alpha(0.9-float32(i)*0.3)), "accent")
						word(name, text, 10, "text")
					})
					rows := []string{"Codex · 你好", "Claude Code"}
					if i == 1 {
						rows = []string{"PowerShell 7"}
					}
					for j, r := range rows {
						c.Key(r)
						row := ui.Row(c).FillWidth().PaddingStart(20).Padding(2).Radius(4)
						if i == 0 && j == 1 {
							region(row.Background(col("selection")), "selection")
						}
						row.Children(func() { word(r, text, 9, "text") })
					}
				}
			})
			// The middle: the tab bar, and the card.
			ui.Column(c).Grow(1).FillHeight().Padding(0, 6, 8, 3).Children(func() {
				region(ui.Row(c).FillWidth().Height(26).Gap(6).AlignItems(ui.Center).Background(own("tabbar")), "tabbar").Children(func() {
					region(ui.Row(c).Padding(3, 8).Radius(5).Gap(4).AlignItems(ui.Center).Background(col("selection")), "selection").Children(func() {
						word("Claude Code", text, 9, "text")
					})
					word("Codex · 你好", muted, 9, "muted")
				})
				region(ui.Column(c).Fill().Padding(10).Gap(3).Radius(8).Background(col("termBg").Alpha(panelAlpha(d))).
					Border(1, border.Alpha(0.7)).Shadow(0, 3, 10, 0, ui.RGBA(0, 0, 0, 0.25)), "termBg").Children(func() {
					for _, l := range []string{"PS D:\\code\\my-app> claude", "> 给首页加个小标题", "● 好的，我先看看 index.html", "  ✓ 改了 index.html +4 −1"} {
						c.Key(l)
						word(l, col("termFg"), 10, "termFg")
					}
				})
			})
			// The right panel.
			region(ui.Column(c).Width(160).FillHeight().Padding(6, 8).Gap(6).Background(own("panel")), "panel").Children(func() {
				ui.Row(c).Gap(6).Children(func() {
					word(L("改动", "Changes"), text, 10, "text")
					word(L("文件", "Files"), muted, 10, "muted")
					word(L("联网", "Network"), muted, 10, "muted")
				})
				region(ui.Column(c).FillWidth().Padding(6).Gap(4).Radius(6).Background(col("surface").Alpha(panelAlpha(d))).Border(1, border), "surface").Children(func() {
					word(L("AI 改了 2 个文件", "2 files changed"), text, 9, "text")
					word("index.html · style.css", muted, 8, "muted")
					region(ui.Row(c).Padding(3, 8).Radius(5).Background(accent), "accent").Children(func() {
						ui.Text(c, L("全部保留", "Keep all")).FontSize(9).TextColor(contrastOn(accent))
					})
				})
			})
		})
	})
}

// contrastOn is black or white, whichever reads on color.
func contrastOn(color ui.Color) ui.Color {
	if luminance(color) > 0.6 {
		return ui.RGB(20, 20, 20)
	}
	return ui.RGB(255, 255, 255)
}
