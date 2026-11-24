package main

import (
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"nook/core/checkpoint"
	"nook/core/config"
)

// settingsSections are the settings' sections, as the left of the page
// lists them.
func settingsSections() []string {
	return []string{L("通用", "General"), L("联网与 AI", "Network and AI"), L("外观", "Appearance"), L("快捷键", "Shortcuts"), L("关于", "About")}
}

// settingsDialog shows the settings in a dialog over the window, which
// stays in sight: the sections on the left, the one chosen on the right;
// or the drawing board, the window behind showing each change. A click
// outside, Escape or × closes it.
func (a *app) settingsDialog(c *ui.Context) {
	if a.settings == "" {
		return
	}
	t := c.Theme()
	open := true
	board := a.settings == "designer"
	w, h := float32(1000), float32(720)
	if board {
		w, h = 1120, 760
	}
	if a.winW > 0 {
		w, h = min(w, a.winW-48), min(h, a.winH-48)
	}
	ui.DialogBase(c, &open, func(back, panel ui.Element) {
		// The board dims the window little: it is what the colors are for.
		dim := float32(0.32)
		if board {
			dim = 0.1
		}
		back.Background(ui.RGBA(0, 0, 0, dim))
		panel.Width(w).Height(h).Radius(12).Clip().Background(a.dialogColor(t)).Border(1, t.Border).
			Shadow(0, 16, 44, 0, ui.RGBA(0, 0, 0, 0.35))
		if board {
			a.designerBody(c)
			return
		}
		ui.Column(c).Fill().Children(func() {
			ui.Row(c).FillWidth().Height(48).PaddingStart(20).PaddingEnd(10).AlignItems(ui.Center).Children(func() {
				ui.Text(c, L("设置", "Settings")).FontSize(15).Bold().Grow(1)
				if iconButton(c, iconX, L("关闭（Esc）", "Close (Esc)")).Clicked() {
					open = false
				}
			})
			ui.Box(c).FillWidth().Height(1).Background(t.Border)
			ui.Row(c).FillWidth().Grow(1).Children(func() {
				ui.Column(c).Width(190).FillHeight().Padding(10).Gap(2).Shrink(0).Children(func() {
					for i, name := range settingsSections() {
						c.Key(i)
						b := ui.ButtonBase(c).Label(name).FillWidth().Height(34).PaddingX(12).Radius(7).Justify(ui.Start)
						if i == a.settingsTab {
							b.Background(t.Selection)
						} else if b.Hovered() {
							b.Background(t.SurfaceHover)
						}
						b.Children(func() {
							txt := ui.Text(c, name).FontSize(13)
							if i == a.settingsTab {
								txt.Bold()
							} else {
								txt.TextColor(t.TextMuted)
							}
						})
						if b.Clicked() {
							a.settingsTab = i
						}
					}
				})
				ui.Box(c).Width(1).FillHeight().Background(t.Border)
				ui.Scroll(c).Grow(1).FillHeight().Children(func() {
					ui.Column(c).FillWidth().MaxWidth(760).PaddingX(28).PaddingY(18).Children(func() {
						settingRows = 0
						ui.Text(c, settingsSections()[a.settingsTab]).FontSize(18).Bold().Padding(0, 0, 8, 0)
						switch a.settingsTab {
						case 0:
							a.generalSettings(c)
						case 1:
							a.networkSettings(c)
						case 2:
							a.appearanceSettings(c)
						case 3:
							a.shortcutSettings(c)
						default:
							a.aboutSettings(c)
						}
						ui.Box(c).Height(20)
					})
				})
			})
		})
	})
	if !open {
		a.leavePages()
	}
}

// dialogColor is the dialogs' own color: solid, whatever lets the window
// show elsewhere.
func (a *app) dialogColor(t *ui.Theme) ui.Color {
	if d := a.currentDesign(); d != nil {
		return ui.Hex(d.Surface).Alpha(1)
	}
	return t.Background
}

func (a *app) generalSettings(c *ui.Context) {
	a.languageChoice(c)
	a.shellChoice(c)
	setting(c, L("关窗口时", "Closing the window"), "", func() {
		if choice(c, &a.st.CloseAction, [2]string{closeAsk, L("每次都问", "Ask")}, [2]string{closeTray, L("收进托盘", "Tray")}, [2]string{closeQuit, L("退出", "Quit")}) {
			a.save()
			a.syncTray()
		}
	})
	a.backupSetting(c)
}

// backupSetting chooses where the copies for undoing go: by default on
// each project's own drive, that a large project fills no other.
func (a *app) backupSetting(c *ui.Context) {
	mode := "drive"
	if a.st.BackupDir != "" {
		mode = "dir"
	}
	where := a.st.BackupDir
	if where == "" {
		where = L("项目所在的盘", "The project's drive")
		if proj := a.currentProject(); proj != "" {
			where = checkpoint.Place(proj)
		}
	}
	setting(c, L("撤销用的备份", "Copies for undoing"), where, func() {
		if choice(c, &mode, [2]string{"drive", L("跟项目同盘", "Project's drive")}, [2]string{"dir", L("自选文件夹…", "A folder…")}) {
			if mode == "drive" {
				a.setBackupDir("")
			} else {
				a.pickBackupDir()
			}
		}
	})
}

// pickBackupDir asks for the folder of the copies.
func (a *app) pickBackupDir() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: L("选择放备份的文件夹", "Choose a folder for the copies"), Directory: true})
		if err != nil || len(paths) == 0 {
			return
		}
		a.update(func() { a.setBackupDir(paths[0]) })
	}()
}

// setBackupDir sets the folder of new copies; those made stay where they
// are until they go.
func (a *app) setBackupDir(dir string) {
	a.st.BackupDir = dir
	checkpoint.Dir = dir
	a.save()
}

func (a *app) networkSettings(c *ui.Context) {
	t := c.Theme()
	a.requireChoice(c)
	setting(c, L("联网锁", "Network lock"), L("锁上后 AI 不能启动", "Locked, no AI can start"), func() {
		locked := a.st.Locked
		if ui.Switch(c, &locked).Label(L("联网锁", "Network lock")).Changed() {
			a.toggleLock()
			a.syncTray()
		}
	})
	setting(c, L("代理地址", "Proxy address"), L("留空就自动找", "Empty: found automatically"), func() {
		ui.Row(c).Gap(6).Children(func() {
			in := ui.TextInput(c, &a.proxyInput).Placeholder(L("自动", "Automatic")).Width(170)
			if ui.Button(c, L("应用", "Apply")).Clicked() || in.Submitted() {
				a.setProxy(a.proxyInput)
			}
		})
	})
	setting(c, L("AI", "AI tools"), L("启动前先查网络。绿点：已安装。", "Checked before they start. Green: installed."), func() {})
	ui.Row(c).FillWidth().Wrap().Gap(6).Children(func() {
		installed := a.installedTools()
		for _, tool := range a.st.AITools {
			c.Key("tool:" + tool)
			ui.Row(c).Gap(5).Padding(4, 10).Radius(12).Border(1, t.Border).AlignItems(ui.Center).Children(func() {
				if slices.Contains(installed, tool) {
					ui.Box(c).Width(6).Height(6).Radius(3).Background(t.Success)
					ui.Text(c, tool).FontSize(12)
				} else {
					ui.Text(c, tool).FontSize(12).TextColor(t.TextMuted)
				}
				if slices.Contains(a.st.MoreAITools, tool) && iconButton(c, iconX, L("去掉", "Remove")).Clicked() {
					a.st.MoreAITools = slices.DeleteFunc(a.st.MoreAITools, func(x string) bool { return x == tool })
					a.st.AITools = slices.DeleteFunc(a.st.AITools, func(x string) bool { return x == tool })
					a.save()
				}
			})
		}
	})
	ui.Row(c).Gap(6).PaddingY(10).Children(func() {
		in := ui.TextInput(c, &a.toolInput).Placeholder(L("加一个，比如 mycli", "Add one, as mycli")).Width(220)
		if ui.Button(c, L("添加", "Add")).Clicked() || in.Submitted() {
			a.addTool(a.toolInput)
			a.toolInput = ""
		}
	})
}

// lookFace is the small window a look shows as: its window color (or
// background), a card in its terminals' color, a few lines of its text.
type lookFace struct {
	paint      func(e ui.Element) ui.Element
	card, line ui.Color
	noCard     bool
}

// lookCard is a look to choose, as its small window and its name; a click
// reports.
func lookCard(c *ui.Context, key, name string, chosen bool, f lookFace, tip string) bool {
	t := c.Theme()
	c.Key("look:" + key)
	b := ui.ButtonBase(c).Label(name).Width(88).Radius(10).Padding(2)
	if tip != "" {
		b.Tooltip(tip)
	}
	clicked := b.Clicked()
	b.Children(func() {
		ui.Column(c).FillWidth().Gap(5).AlignItems(ui.Center).Children(func() {
			face := ui.Box(c).Width(84).Height(54).Radius(9).Clip()
			switch {
			case chosen:
				face.Border(2, t.Accent)
			case b.Hovered():
				face.Border(1, t.TextMuted)
			default:
				face.Border(1, t.Border)
			}
			f.paint(face).Children(func() {
				for i := range 3 {
					ui.Box(c).Absolute().Left(7).Top(float32(12 + i*7)).Width(11).Height(3).Radius(2).Background(f.line)
				}
				if !f.noCard {
					ui.Box(c).Absolute().Left(25).Top(11).Right(7).Bottom(7).Radius(5).Background(f.card).
						Shadow(0, 2, 6, 0, ui.RGBA(0, 0, 0, 0.25))
				}
			})
			txt := ui.Text(c, name).FontSize(12).SingleLine()
			if chosen {
				txt.Bold()
			} else {
				txt.TextColor(t.TextMuted)
			}
		})
	})
	return clicked
}

// paletteFace is the small window of a palette of Nook's.
func paletteFace(p palette) lookFace {
	return lookFace{
		paint: func(e ui.Element) ui.Element { return e.Background(ui.Hex(p.surface)) },
		card:  ui.Hex(p.bg), line: ui.Hex(p.muted),
	}
}

// designFace is the small window of a look of the user's.
func designFace(d *config.Design) lookFace {
	return lookFace{
		paint: func(e ui.Element) ui.Element { return paintBackground(e, d) },
		card:  colorOr(d.TermBg, d.Background), line: colorOr(d.Muted, d.Text),
	}
}

// choosePalette takes palette id: Nook's, a Ghostty theme, or "my:" a
// look of the user's, which has its own accent.
func (a *app) choosePalette(id string) {
	// The terminals' flavor goes with the look: kept for the one left,
	// found again for the one chosen.
	if a.st.TermFlavors == nil {
		a.st.TermFlavors = map[string]string{}
	}
	if a.st.TermTheme != "" {
		a.st.TermFlavors[a.st.Palette] = a.st.TermTheme
	} else {
		delete(a.st.TermFlavors, a.st.Palette)
	}
	a.st.Palette, a.st.TermTheme = id, a.st.TermFlavors[id]
	if strings.HasPrefix(id, "my:") || strings.HasPrefix(id, "look:") {
		a.st.Accent = ""
	}
	a.applyColors()
}

// moreLink is a quiet link to more choices: words and ›.
func moreLink(c *ui.Context, label string) bool {
	t := c.Theme()
	b := ui.ButtonBase(c).Label(label).Padding(2, 0)
	b.Children(func() {
		ui.Row(c).Gap(2).AlignItems(ui.Center).Children(func() {
			fg := t.TextMuted
			if b.Hovered() {
				fg = t.Text
			}
			ui.Text(c, label).FontSize(12).TextColor(fg)
			ui.Icon(c, iconChevronRight).FontSize(12).TextColor(fg)
		})
	})
	return b.Clicked()
}

// lookCards are the looks to choose: the system's and the ready-made
// ones, a palette or a Ghostty theme chosen before; then the user's, and
// one more to make.
func (a *app) lookCards(c *ui.Context) {
	t := c.Theme()
	small := func(s string) { ui.Text(c, s).FontSize(12).TextColor(t.TextMuted) }
	small(L("现成的", "Ready-made"))
	ui.Row(c).FillWidth().Wrap().Gap(10).Children(func() {
		light, dark := ui.LightTheme(), ui.DarkTheme()
		sys := lookFace{
			paint: func(e ui.Element) ui.Element {
				return e.LinearGradient(ui.LinearGradient{Angle: 135, Stops: []ui.GradientStop{
					{Color: light.Surface, At: 0}, {Color: light.Surface, At: 0.5}, {Color: dark.Surface, At: 0.5}, {Color: dark.Surface, At: 1}}})
			},
			line: t.TextMuted, noCard: true,
		}
		if lookCard(c, "", L("跟随系统", "System"), a.st.Palette == "", sys, L("跟着 Windows 的浅色或深色", "Light or dark, as Windows is")) {
			a.choosePalette("")
		}
		for i, p := range quickPresets {
			id := "look:" + p.id
			if lookCard(c, id, L(p.zh, p.en), a.st.Palette == id, designFace(presetLook(i)), L(p.descZh, p.descEn)) {
				a.choosePalette(id)
			}
		}
		// A palette of Nook 1's, or a Ghostty theme, while chosen.
		if p, ok := paletteByID(a.st.Palette); ok {
			lookCard(c, p.id, L(p.zh, p.en), true, paletteFace(p), "")
		}
		if name, ok := strings.CutPrefix(a.st.Palette, "ghostty:"); ok {
			if p, _, ok := ghosttyPalette(name); ok {
				lookCard(c, a.st.Palette, name, true, paletteFace(p), "")
			}
		}
	})
	small(L("我的", "Mine"))
	ui.Row(c).FillWidth().Wrap().Gap(10).Children(func() {
		for i := range a.st.Designs {
			d := &a.st.Designs[i]
			id := "my:" + d.ID
			chosen := a.st.Palette == id
			tip := ""
			if chosen {
				tip = L("再点一下，在画板里改", "Click again to change it on the drawing board")
			}
			if lookCard(c, id, d.Name, chosen, designFace(d), tip) {
				if chosen {
					a.openDesigner(c)
				} else {
					a.choosePalette(id)
				}
			}
		}
		// One more: a new look, made on the drawing board.
		c.Key("look:+")
		add := ui.ButtonBase(c).Label(L("新建一套", "New look")).Width(88).Radius(10).Padding(2).Tooltip(L("在画板里配一套自己的", "Make one of your own on the drawing board"))
		add.Children(func() {
			ui.Column(c).FillWidth().Gap(5).AlignItems(ui.Center).Children(func() {
				face := ui.Box(c).Width(84).Height(54).Radius(9).Center().Background(t.Text.Alpha(0.05)).Border(1, t.Border)
				if add.Hovered() {
					face.Background(t.Text.Alpha(0.09))
				}
				face.Children(func() { ui.Icon(c, iconPlus).FontSize(20).TextColor(t.TextMuted) })
				ui.Text(c, L("新建", "New")).FontSize(12).TextColor(t.TextMuted)
			})
		})
		if add.Clicked() {
			a.newLook(c)
		}
	})
	if moreLink(c, L("更多配色（Ghostty 主题）", "More colors (Ghostty themes)")) {
		a.themeFor, a.themeQuery = "palette", ""
	}
}

// newLook makes a look, from the first of the quick presets, and opens
// the drawing board on it.
func (a *app) newLook(c *ui.Context) {
	d := a.newDesign(c.Theme())
	quickPresets[0].apply(d)
	a.applyColors()
	a.openDesigner(c)
}

// themesRead keeps Ghostty's themes the settings show, read once.
var themesRead = map[string]*terminal.Theme{}

func ghosttyTheme(name string) *terminal.Theme {
	if th, ok := themesRead[name]; ok {
		return th
	}
	th, err := terminal.GhosttyTheme(name)
	if err != nil {
		th = nil
	}
	themesRead[name] = th
	return th
}

// termSuggestions are the terminal themes the settings show as samples,
// besides the palette's own.
var termSuggestions = []string{"Nord", "Catppuccin Mocha", "TokyoNight", "Catppuccin Latte"}

// paletteTerm is the terminals' colors when they follow the palette.
func (a *app) paletteTerm(t *ui.Theme) *terminal.Theme {
	saved := a.st.TermTheme
	a.st.TermTheme = ""
	light, dark := a.termThemes()
	a.st.TermTheme = saved
	switch {
	case t.Dark && dark != nil:
		return dark
	case !t.Dark && light != nil:
		return light
	case t.Dark:
		return terminal.DarkTheme()
	}
	return terminal.LightTheme()
}

// termSample is a few lines of a terminal in theme th, to choose it by,
// and its name; a click reports.
func termSample(c *ui.Context, key, name string, th *terminal.Theme, chosen bool) bool {
	t := c.Theme()
	c.Key("term:" + key)
	b := ui.ButtonBase(c).Label(name).Width(206).Radius(10).Padding(2)
	clicked := b.Clicked()
	b.Children(func() {
		ui.Column(c).FillWidth().Gap(5).Children(func() {
			face := ui.Column(c).FillWidth().Padding(9, 11).Gap(2).Radius(9).Background(th.Background)
			switch {
			case chosen:
				face.Border(2, t.Accent)
			case b.Hovered():
				face.Border(1, t.TextMuted)
			default:
				face.Border(1, t.Border)
			}
			const mono = "Cascadia Mono, Consolas, monospace"
			type part struct {
				s   string
				col ui.Color
			}
			line := func(parts ...part) {
				ui.Row(c).Children(func() {
					for _, p := range parts {
						ui.Text(c, p.s).Font(mono).FontSize(11).TextColor(p.col).SingleLine()
					}
				})
			}
			p := th.Palette
			face.Children(func() {
				// Spaces lead a part: a text's last spaces do not show.
				line(part{"~/my-app>", p[4]}, part{" npm run dev", th.Foreground})
				line(part{"  ready", p[2]}, part{" localhost:5173", p[6]})
				line(part{"✓ 3", p[2]}, part{"  ⚠ 1", p[3]}, part{"  ✗ 0", p[1]})
			})
			txt := ui.Text(c, name).FontSize(12).SingleLine().Padding(0, 0, 0, 2)
			if chosen {
				txt.Bold()
			} else {
				txt.TextColor(t.TextMuted)
			}
		})
	})
	return clicked
}

// termSamples are the terminals' colors to choose: the palette's, a few
// themes of Ghostty's, the one chosen, and the way to the others.
func (a *app) termSamples(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).FillWidth().Wrap().Gap(10).Children(func() {
		if termSample(c, "", L("这套自带的", "The look's own"), a.paletteTerm(t), a.st.TermTheme == "") && a.st.TermTheme != "" {
			a.st.TermTheme = ""
			a.applyColors()
		}
		names := a.termFlavors()
		if a.st.TermTheme != "" && !slices.Contains(names, a.st.TermTheme) {
			names = append([]string{a.st.TermTheme}, names...)
		}
		for _, name := range names {
			th := ghosttyTheme(name)
			if th == nil {
				continue
			}
			if termSample(c, name, name, th, a.st.TermTheme == name) && a.st.TermTheme != name {
				a.st.TermTheme = name
				a.applyColors()
			}
		}
	})
	ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
		if smallButton(c, L("换个口味", "Another flavor"), false).Tooltip(L("换成这套配色搭的下一种", "The next one that goes with the look")).Clicked() {
			a.nextTermFlavor()
		}
		if moreLink(c, L("更多终端主题", "More terminal themes")) {
			a.themeFor, a.themeQuery = "term", ""
		}
	})
}

func (a *app) appearanceSettings(c *ui.Context) {
	t := c.Theme()
	settingStack(c, L("配色", "Colors"), "", func() { a.lookCards(c) })
	if a.st.Palette == "" {
		setting(c, L("浅色还是深色", "Light or dark"), L("跟着 Windows 走，或固定一种", "Follows Windows, or keeps one"), func() {
			if choice(c, &a.st.Theme, [2]string{"", L("跟随系统", "System")}, [2]string{"light", L("浅色", "Light")}, [2]string{"dark", L("深色", "Dark")}) {
				a.save()
				a.applyTheme()
			}
		})
	}
	if settingLink(c, iconPalette, L("画板", "Drawing board"), L("拖色点，配一套自己的颜色", "Drag the colors: a look of your own")) {
		a.openDesigner(c)
	}
	setting(c, L("强调色", "Accent"), L("按钮、选中、光标", "Buttons, selections, the cursor"), func() {
		ui.Row(c).Wrap().Gap(7).AlignItems(ui.Center).Children(func() {
			auto := ui.ButtonBase(c).Label(L("跟随配色", "The palette's")).Height(26).PaddingX(9).Radius(7).Border(1, t.Border)
			if a.st.Accent == "" {
				auto.Border(2, t.Accent)
			}
			auto.Children(func() { ui.Text(c, L("跟随配色", "The palette's")).FontSize(12) })
			if auto.Clicked() && a.st.Accent != "" {
				a.st.Accent = ""
				a.applyColors()
			}
			for _, col := range accents {
				c.Key("acc:" + col)
				dot := ui.ButtonBase(c).Label(col).Width(22).Height(22).Radius(11).Background(ui.Hex(col))
				if strings.EqualFold(a.st.Accent, col) {
					dot.Border(2, t.Text)
				}
				if dot.Clicked() {
					a.st.Accent = col
					a.applyColors()
				}
			}
			in := ui.TextInput(c, &a.accentInput).Placeholder("#rrggbb").Width(86)
			if in.Submitted() {
				if v := strings.TrimSpace(a.accentInput); len(v) == 7 && v[0] == '#' {
					a.st.Accent = strings.ToLower(v)
					a.applyColors()
				}
			}
		})
	})
	settingStack(c, L("终端配色", "Terminal colors"), L("每套配色都搭好了几种口味，玩腻了就换一个", "Each look has a few that go with it: change when one gets old"), func() { a.termSamples(c) })
	setting(c, L("终端字体", "Terminal font"), L("留空用系统等宽字体", "Empty: the system's monospace"), func() {
		ui.Row(c).Gap(6).Children(func() {
			in := ui.TextInput(c, &a.fontInput).Placeholder(L("系统等宽字体", "System monospace")).Width(170)
			if ui.Button(c, L("应用", "Apply")).Clicked() || in.Submitted() {
				a.st.TermFont = strings.TrimSpace(a.fontInput)
				a.save()
				a.applyFont() // the size as it is, the default kept
			}
		})
	})
	setting(c, L("终端字号", "Terminal font size"), L("默认 ", "Default ")+itoa(defaultFontSize), func() {
		size := a.fontSize()
		if n := stepper(c, size, L("终端字号", "Terminal font size")); n != size {
			a.setFontSize(n)
		}
	})
	setting(c, L("布局", "Layout"), L("拖边调宽，双击还原", "Drag edges to resize; double-click to reset"), func() {
		if ui.Button(c, L("全部还原", "Reset all")).Clicked() {
			a.st.SidebarWidth, a.st.SideWidth, a.st.ViewerSplit = sidebarDefault, sideDefault, nil
			a.save()
		}
	})
}

// themePicker lists Ghostty's themes, found by typing, while one is being
// chosen for the window or the terminals.
func (a *app) themePicker(c *ui.Context) {
	if a.themeFor == "" {
		return
	}
	t := c.Theme()
	open := true
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(480).Gap(8).Children(func() {
			title := L("给整个窗口挑一套配色", "Colors for the whole window")
			switch a.themeFor {
			case "term", "designTerm":
				title = L("给终端挑一套配色", "Colors for the terminals")
			case "design":
				title = L("从一套 Ghostty 主题开始", "Start from a Ghostty theme")
			}
			ui.Text(c, title).FontSize(15).Bold()
			ui.TextInput(c, &a.themeQuery).Placeholder(L("搜索，比如 Catppuccin、Nord、Dracula", "Search, as Catppuccin, Nord, Dracula")).AutoFocus().FillWidth()
			ui.Scroll(c).FillWidth().Height(380).Children(func() {
				ui.Column(c).FillWidth().Gap(1).Children(func() {
					q := strings.ToLower(strings.TrimSpace(a.themeQuery))
					shown := 0
					for _, name := range terminal.GhosttyThemes() {
						if q != "" && !strings.Contains(strings.ToLower(name), q) {
							continue
						}
						if shown++; shown > 200 {
							ui.Text(c, L("还有更多，打字搜索…", "More: type to search…")).FontSize(12).TextColor(t.TextMuted).Padding(6)
							break
						}
						c.Key(name)
						row := ui.Row(c).FillWidth().Height(30).PaddingX(8).Gap(8).AlignItems(ui.Center).Radius(6).Cursor(ui.CursorPointer)
						if row.Hovered() {
							row.Background(t.SurfaceHover)
						}
						row.Children(func() {
							if p, _, ok := ghosttyPalette(name); ok {
								swatch(c, p.bg, 14)
								swatch(c, p.accent, 14)
								swatch(c, p.text, 14)
							}
							ui.Text(c, name).FontSize(13).SingleLine()
						})
						if row.Clicked() {
							d := a.currentDesign()
							switch {
							case a.themeFor == "design" && d != nil:
								// The look starts over from the theme, terminals and all.
								if p, _, ok := ghosttyPalette(name); ok {
									fromPalette(d, p)
									termTheme(d, name)
									a.pickPart(d, a.designPart)
								}
							case a.themeFor == "designTerm" && d != nil:
								termTheme(d, name)
								a.pickPart(d, a.designPart)
							case a.themeFor == "term":
								a.st.TermTheme = name
							default:
								a.choosePalette("ghostty:" + name)
							}
							a.themeFor = ""
							a.applyColors()
						}
					}
				})
			})
		})
	})
	if !open {
		a.themeFor = ""
	}
}

func (a *app) shortcutSettings(c *ui.Context) {
	t := c.Theme()
	for _, k := range shortcutList() {
		c.Key(k[0])
		ui.Row(c).FillWidth().PaddingY(7).AlignItems(ui.Center).Children(func() {
			ui.Text(c, k[1]).FontSize(13).Grow(1)
			ui.Text(c, k[0]).FontSize(12).TextColor(t.TextMuted).Padding(3, 8).Radius(5).Background(t.Surface)
		})
	}
}

func (a *app) aboutSettings(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Gap(12).AlignItems(ui.Center).PaddingY(8).Children(func() {
		ui.Image(c, appBitmap()).Size(44, 44)
		ui.Column(c).Gap(2).Children(func() {
			ui.Text(c, "Nook "+version).FontSize(15).Bold()
			ui.Text(c, L("替你管住联网的 AI 终端", "The AI terminal that keeps its networking in your hands")).FontSize(12).TextColor(t.TextMuted)
		})
	})
	ui.Text(c, L("Nook 自己不联网，也不改你的 AI 设置。", "Nook itself never goes online, and never changes your AI tools' settings.")).FontSize(13).PaddingY(8)
	ui.Row(c).Gap(8).PaddingY(4).Children(func() {
		if ui.Button(c, L("打开设置文件", "Open the settings file")).Tooltip(config.Path()).Clicked() {
			mygo.Shell.ShowItemInFolder(config.Path())
		}
		dir := a.st.BackupDir
		if dir == "" {
			if proj := a.currentProject(); proj != "" {
				dir = checkpoint.Place(proj)
			}
		}
		if dir != "" && ui.Button(c, L("打开备份文件夹", "Open the copies' folder")).Tooltip(dir).Clicked() {
			mygo.Shell.OpenPath(dir)
		}
	})
}
