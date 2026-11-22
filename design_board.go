package main

import (
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

func surfaceOf(d *config.Design) string { return d.Surface }

// selectionOf is the rows chosen when the look gives them no color: a
// little of the accent in the background, which any text reads on.
func selectionOf(d *config.Design) string {
	return hex(ui.Hex(d.Background).Mix(ui.Hex(d.Accent), 0.22))
}

var boardParts = []boardPart{
	{designPart{"bg", "窗口背景", "Background", func(d *config.Design) *string { return &d.Background }}, nil},
	{designPart{"titlebar", "标题栏", "Title bar", func(d *config.Design) *string { return &d.TitleBar }}, surfaceOf},
	{designPart{"sidebar", "侧栏", "Sidebar", func(d *config.Design) *string { return &d.Sidebar }}, surfaceOf},
	{designPart{"tabbar", "标签栏", "Tab bar", func(d *config.Design) *string { return &d.TabBar }}, surfaceOf},
	{designPart{"panel", "右侧面板", "Right panel", func(d *config.Design) *string { return &d.Panel }}, surfaceOf},
	{designPart{"surface", "其他面板", "Other panels", func(d *config.Design) *string { return &d.Surface }}, nil},
	{designPart{"text", "文字", "Text", func(d *config.Design) *string { return &d.Text }}, nil},
	{designPart{"muted", "次要文字", "Secondary text", func(d *config.Design) *string { return &d.Muted }}, nil},
	{designPart{"border", "边框和分隔线", "Borders", func(d *config.Design) *string { return &d.Border }}, nil},
	{designPart{"accent", "强调色", "Accent", func(d *config.Design) *string { return &d.Accent }}, nil},
	{designPart{"selection", "选中的行", "Rows chosen", func(d *config.Design) *string { return &d.Selection }}, selectionOf},
	{designPart{"termBg", "终端背景", "Terminal background", func(d *config.Design) *string { return &d.TermBg }}, nil},
	{designPart{"termFg", "终端文字", "Terminal text", func(d *config.Design) *string { return &d.TermFg }}, nil},
	{designPart{"gradient", "渐变的另一头", "Gradient's other end", func(d *config.Design) *string { return &d.Gradient }}, nil},
}

func boardPartByID(id string) boardPart {
	for _, p := range boardParts {
		if p.id == id {
			return p
		}
	}
	return boardParts[0]
}

// value is the color a part shows: its own, else its fallback's.
func (p boardPart) value(d *config.Design) string {
	if v := *p.field(d); v != "" || p.fallback == nil {
		return v
	}
	return p.fallback(d)
}

// panelColor is the background of panel id (titlebar, sidebar, tabbar,
// panel) in the look chosen: its own color, else the theme's surface.
func (a *app) panelColor(t *ui.Theme, id string) ui.Color {
	if d := a.currentDesign(); d != nil {
		if v := *boardPartByID(id).field(d); v != "" {
			return ui.Hex(v).Alpha(panelAlpha(d))
		}
	}
	return t.Surface
}

// pickPart chooses the part of the look to color.
func (a *app) pickPart(d *config.Design, id string) {
	a.designPart = id
	a.designColor = ui.Hex(boardPartByID(id).value(d))
}

// setPart colors the part chosen.
func (a *app) setPart(d *config.Design, col ui.Color) {
	p := boardPartByID(a.designPart)
	*p.field(d) = hex(col)
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

// designerPage is the middle while the drawing board shows.
func (a *app) designerPage(c *ui.Context) {
	t := c.Theme()
	d := a.currentDesign()
	if d == nil {
		a.viewing = "settings"
		return
	}
	if a.designPart == "" {
		a.pickPart(d, "bg")
	}
	part := boardPartByID(a.designPart)
	done := func() {
		a.viewing = "settings"
		a.save()
	}
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		ui.Row(c).FillWidth().PaddingStart(20).PaddingEnd(10).Height(50).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, L("画板", "Drawing board")).FontSize(15).Bold()
			if ui.TextInput(c, &d.Name).Width(170).Changed() {
				a.layoutChanged = true
			}
			ui.Spacer(c)
			ui.MenuButton(c, L("从现成配色开始", "Start from…"), func(m *ui.Menu) {
				for _, p := range palettes {
					if m.Item(L(p.zh, p.en)).Chosen() {
						fromPalette(d, p)
						a.pickPart(d, a.designPart)
						a.designChanged()
					}
				}
				m.Separator()
				if m.Item(L("Ghostty 主题…（几百套）", "A Ghostty theme… (hundreds)")).Chosen() {
					a.themeFor, a.themeQuery = "design", ""
				}
			})
			if smallButton(c, L("随机一套", "Surprise me"), false).Tooltip(L("随便来一套搭配好的颜色", "A random set of colors that go together")).Clicked() {
				randomColors(d)
				d.TitleBar, d.Sidebar, d.TabBar, d.Panel, d.TermOwn = "", "", "", "", false
				a.pickPart(d, a.designPart)
				a.designChanged()
			}
			if smallButton(c, L("新建一套", "New look"), false).Clicked() {
				a.pickPart(a.newDesign(t), a.designPart)
			}
			if smallButton(c, L("删除这套", "Delete"), false).Clicked() {
				id, name := d.ID, d.Name
				a.confirm = &confirmAsk{title: L("删除这套配色？", "Delete this look?"), text: name, yes: L("删除", "Delete"), danger: true, do: func() {
					a.st.Designs = slices.DeleteFunc(a.st.Designs, func(x config.Design) bool { return x.ID == id })
					a.st.Palette = ""
					a.viewing = "settings"
					a.applyColors()
				}}
			}
			if smallButton(c, L("完成", "Done"), true).Clicked() {
				done()
			}
		})
		ui.Divider(c)
		ui.Row(c).Fill().Children(func() {
			ui.Scroll(c).Grow(1).FillHeight().Children(func() {
				ui.Column(c).FillWidth().Padding(20).Gap(10).Children(func() {
					ui.Text(c, L("这里改的是整个 Nook：窗口、各个面板，还有终端。点小窗口里的任意部分，在右边给它上色，马上就生效。", "This colors the whole of Nook: the window, each panel, and the terminals. Click any part of the small window, color it on the right; it takes effect at once.")).FontSize(12).TextColor(t.TextMuted).Wrap()
					a.designPreview(c, d)
					ui.Row(c).FillWidth().Wrap().Gap(6).PaddingY(4).Children(func() {
						for _, p := range boardParts {
							if p.id == "gradient" && d.Gradient == "" {
								continue
							}
							c.Key("part:" + p.id)
							b := ui.ButtonBase(c).Label(L(p.zh, p.en)).Padding(4, 10).Radius(14).Border(1, t.Border)
							if p.id == part.id {
								b.Border(2, t.Accent)
							} else if b.Hovered() {
								b.Background(t.SurfaceHover)
							}
							b.Children(func() {
								ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
									swatch(c, p.value(d), 12)
									ui.Text(c, L(p.zh, p.en)).FontSize(12)
								})
							})
							if b.Clicked() {
								a.pickPart(d, p.id)
							}
						}
					})
				})
			})
			ui.Box(c).Width(1).FillHeight().Background(t.Border)
			ui.Scroll(c).Width(320).FillHeight().Shrink(0).Children(func() {
				ui.Column(c).FillWidth().Padding(16).Gap(10).Children(func() {
					a.designControls(c, d, part)
				})
			})
		})
	})
	c.OnWindowShortcut(0, ui.KeyEscape, done)
}

// designControls are the board's right side: the color of the part
// chosen, the colors used already, then the terminals and the background.
func (a *app) designControls(c *ui.Context, d *config.Design, part boardPart) {
	t := c.Theme()
	heading := func(s string) { ui.Text(c, s).FontSize(12).Bold().TextColor(t.TextMuted).Margin(8, 0, 0, 0) }
	heading(L("正在改：", "Coloring: ") + L(part.zh, part.en))
	if ui.ColorPicker(c, &a.designColor).Changed() {
		a.setPart(d, a.designColor)
	}
	if part.fallback != nil && *part.field(d) != "" {
		label := L("跟「其他面板」一样", "As the other panels")
		if part.id == "selection" {
			label = L("自动（跟着强调色）", "Automatic (from the accent)")
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
			dot := ui.ButtonBase(c).Label(col).Width(24).Height(24).Radius(6).Background(ui.Hex(col)).Border(1, t.Border).Tooltip(L("用到正在改的部分：", "Use for the part chosen: ") + col)
			if dot.Clicked() {
				a.setPart(d, ui.Hex(col))
			}
		}
	})

	heading(L("控件风格", "Controls"))
	mode := 1
	if d.Dark {
		mode = 0
	}
	if ui.Segmented(c, &mode, L("深色", "Dark"), L("浅色", "Light")).Changed() {
		d.Dark = mode == 0
		a.designChanged()
	}

	heading(L("终端主题", "Terminal theme"))
	ui.Text(c, L("套一套 Ghostty 主题，连程序输出的彩色字一起换；套完还能接着改终端背景和文字。", "A Ghostty theme, the colors programs print included; then the background and text can still change.")).FontSize(11).TextColor(t.TextMuted).Wrap()
	ui.Row(c).Gap(6).Wrap().AlignItems(ui.Center).Children(func() {
		label := L("没有套用", "None")
		if d.TermTheme != "" {
			label = d.TermTheme
		}
		ui.Text(c, label).FontSize(12)
		if smallButton(c, L("挑一套…", "Pick…"), false).Clicked() {
			a.themeFor, a.themeQuery = "designTerm", ""
		}
		if d.TermTheme != "" && smallButton(c, L("不用了", "None"), false).Clicked() {
			d.TermTheme, d.TermOwn = "", false
			d.TermBg, d.TermFg = d.Background, d.Text
			a.designChanged()
		}
	})

	heading(L("背景", "Background"))
	grad := d.Gradient != ""
	var gradChanged bool
	ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
		ui.Text(c, L("渐变背景", "Gradient")).FontSize(12).Grow(1)
		gradChanged = ui.Switch(c, &grad).Label(L("渐变背景", "Gradient")).Changed()
	})
	if gradChanged {
		if grad {
			d.Gradient = hex(ui.Hex(d.Background).Mix(ui.Hex(d.Accent), 0.3))
			if d.PanelOpacity == 0 || d.PanelOpacity == 1 {
				d.PanelOpacity = 0.82
			}
			a.pickPart(d, "gradient")
		} else {
			d.Gradient = ""
			if a.designPart == "gradient" {
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
	if d.Gradient != "" {
		slider(L("渐变方向", "Direction"), &d.GradientAngle, 0, 360)
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
		slider(L("面板不透明程度（往左越透）", "Panels' opacity (left: see-through)"), &d.PanelOpacity, 0.3, 1)
		ui.Text(c, L("终端要新开一个才会透明。", "Terminals turn see-through once opened again.")).FontSize(11).TextColor(t.TextMuted).Wrap()
	}
}

// designPreview is a small Nook in the look's colors, each part of
// which chooses the part to color.
func (a *app) designPreview(c *ui.Context, d *config.Design) {
	col := func(id string) ui.Color { return ui.Hex(boardPartByID(id).value(d)) }
	bg, text, muted, border, accent := col("bg"), col("text"), col("muted"), col("border"), col("accent")
	alpha := panelAlpha(d)
	mark := ui.RGBA(255, 255, 255, 0.9)
	if !d.Dark {
		mark = ui.RGBA(0, 0, 0, 0.75)
	}
	// region is a part of the small window that chooses part id.
	region := func(e ui.Element, id string) ui.Element {
		p := boardPartByID(id)
		e.Cursor(ui.CursorPointer).Tooltip(L(p.zh, p.en))
		if a.designPart == id {
			e.Border(2, mark)
		}
		if e.Clicked() {
			a.pickPart(d, id)
		}
		return e
	}
	word := func(s string, color ui.Color, size float32, id string) {
		region(ui.Text(c, s).FontSize(size).TextColor(color).Padding(1, 3).Radius(3), id)
	}
	box := region(ui.Column(c).FillWidth().MaxWidth(760).Height(380).Radius(10).Clip().Background(bg).Border(1, border), "bg")
	if d.Gradient != "" {
		box.Gradient(bg, ui.Hex(d.Gradient), d.GradientAngle)
	}
	box.Children(func() {
		if bmp := a.backdrop(d); bmp != nil {
			img := ui.Image(c, bmp).Absolute().Top(0).Left(0).Right(0).Bottom(0).Fit(ui.Cover).Opacity(d.ImageOpacity).ZIndex(-1)
			if d.ImageBlur > 0 {
				img.Blur(d.ImageBlur / 2).Cache()
			}
		}
		// The title bar.
		region(ui.Row(c).FillWidth().Height(28).PaddingX(10).Gap(6).AlignItems(ui.Center).Background(col("titlebar").Alpha(alpha)), "titlebar").Children(func() {
			word("Nook", text, 11, "text")
			ui.Spacer(c)
			region(ui.Box(c).Width(70).Height(14).Radius(7).Border(1, border), "border")
			region(ui.Box(c).Width(56).Height(14).Radius(7).Border(1, border), "border")
		})
		ui.Row(c).FillWidth().Grow(1).Children(func() {
			// The sidebar.
			region(ui.Column(c).Width(150).FillHeight().Padding(8).Gap(6).Background(col("sidebar").Alpha(alpha)), "sidebar").Children(func() {
				word(L("项目", "Projects"), muted, 9, "muted")
				for i, name := range []string{"Demo", "Website", "Notes"} {
					c.Key(i)
					r := ui.Row(c).Gap(6).AlignItems(ui.Center).Padding(2).Radius(4)
					if i == 0 {
						region(r.Background(col("selection")), "selection")
					}
					r.Children(func() {
						region(ui.Box(c).Width(16).Height(16).Radius(4).Background(accent.Alpha(0.85-float32(i)*0.2)), "accent")
						ui.Column(c).Children(func() {
							word(name, text, 10, "text")
							word(`E:\`+name, muted, 8, "muted")
						})
					})
				}
			})
			region(ui.Box(c).Width(1).FillHeight().Background(border), "border")
			// The terminals.
			ui.Column(c).Grow(1).FillHeight().Children(func() {
				region(ui.Row(c).FillWidth().Height(26).PaddingX(8).Gap(6).AlignItems(ui.Center).Background(col("tabbar").Alpha(alpha)), "tabbar").Children(func() {
					region(ui.Row(c).Padding(3, 8).Radius(5).Gap(4).AlignItems(ui.Center).Background(bg.Alpha(alpha)), "bg").Children(func() {
						region(ui.Box(c).Width(6).Height(6).Radius(3).Background(accent), "accent")
						word("claude", text, 9, "text")
					})
					word("PowerShell", muted, 9, "muted")
				})
				region(ui.Column(c).Fill().Padding(10).Gap(3).Background(col("termBg").Alpha(alpha)), "termBg").Children(func() {
					for _, l := range []string{"PS E:\\Demo> claude", "✻ 正在修改首页…", "  ⎿ index.html  +12 −3", "> "} {
						c.Key(l)
						word(l, col("termFg"), 10, "termFg")
					}
				})
			})
			region(ui.Box(c).Width(1).FillHeight().Background(border), "border")
			// The right panel.
			region(ui.Column(c).Width(170).FillHeight().Padding(8).Gap(6).Background(col("panel").Alpha(alpha)), "panel").Children(func() {
				ui.Row(c).Gap(6).Children(func() {
					word(L("改动", "Changes"), text, 10, "text")
					word(L("文件", "Files"), muted, 10, "muted")
				})
				for i, f := range []string{"index.html", "app.js"} {
					c.Key(i)
					word(f, text, 9, "text")
				}
				region(ui.Row(c).Padding(4, 10).Radius(5).Background(accent), "accent").Children(func() {
					ui.Text(c, L("全部保留", "Keep all")).FontSize(9).TextColor(contrastOn(accent))
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
