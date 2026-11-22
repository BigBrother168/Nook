package main

import (
	"slices"
	"strconv"
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

// settingsPage is the middle when the settings show: their sections on the
// left, the one chosen on the right.
func (a *app) settingsPage(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).FillWidth().PaddingStart(20).PaddingEnd(10).Height(46).AlignItems(ui.Center).Children(func() {
			ui.Text(c, L("设置", "Settings")).FontSize(15).Bold().Grow(1)
			if iconButton(c, iconX, L("关闭（Esc）", "Close (Esc)")).Clicked() {
				a.closeMainView()
			}
		})
		ui.Divider(c)
		ui.Row(c).Fill().Children(func() {
			ui.Column(c).Width(180).FillHeight().Padding(10).Gap(2).Shrink(0).Children(func() {
				for i, name := range settingsSections() {
					c.Key(i)
					b := ui.ButtonBase(c).Label(name).FillWidth().Height(34).PaddingX(12).Radius(7).Justify(ui.Start)
					if i == a.settingsTab {
						b.Background(t.SurfaceHover)
					} else if b.Hovered() {
						b.Background(t.Surface)
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
				ui.Column(c).FillWidth().MaxWidth(760).PaddingX(32).PaddingY(20).Children(func() {
					ui.Text(c, settingsSections()[a.settingsTab]).FontSize(20).Bold().Padding(0, 0, 8, 0)
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
					ui.Box(c).Height(24)
				})
			})
		})
	})
	c.OnWindowShortcut(0, ui.KeyEscape, a.closeMainView)
}

func (a *app) generalSettings(c *ui.Context) {
	a.languageChoice(c)
	a.shellChoice(c)
	setting(c, L("点窗口的 × 时", "The window's ×"), L("收进托盘时终端和 AI 继续运行；退出就全部结束。", "In the tray, terminals and AI tools go on; quitting ends them all."), func() {
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
	t := c.Theme()
	mode := "drive"
	if a.st.BackupDir != "" {
		mode = "dir"
	}
	setting(c, L("撤销用的备份放在哪", "Where copies for undoing go"), L("AI 改文件前，Nook 记下文件原来的样子，你点保留或撤销后就删掉。项目可能很大，所以默认放在项目所在的那个盘，不占 C 盘。", "Before the AI changes files, Nook keeps them as they were, until you keep or undo the changes. A project may be large, so by default the copy goes on the project's own drive, not on C:."), func() {
		ui.Row(c).Gap(6).Children(func() {
			if choice(c, &mode, [2]string{"drive", L("跟项目同一个盘", "The project's drive")}, [2]string{"dir", L("自选文件夹…", "A folder…")}) {
				if mode == "drive" {
					a.setBackupDir("")
				} else {
					a.pickBackupDir()
				}
			}
		})
	})
	where := a.st.BackupDir
	if where == "" {
		where = L(`例：项目在 E 盘，就放在 E:\.nook\backups（隐藏文件夹）；项目在 C 盘才放 C 盘。`, `As: a project on E: has its copy in E:\.nook\backups (hidden); only one on C: has it on C:.`)
	}
	if proj := a.currentProject(); proj != "" {
		where += L("\n当前项目：", "\nThis project: ") + checkpoint.Place(proj)
	}
	ui.Text(c, where).FontSize(11).TextColor(t.TextMuted).Wrap().Margin(0, 0, 6, 0)
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
	setting(c, L("联网锁", "Network lock"), L("锁上后，AI 一律不能启动；预览也不打开外部网页。", "Locked, no AI tool can start, and previews open no page of the internet."), func() {
		locked := a.st.Locked
		if ui.Switch(c, &locked).Label(L("联网锁", "Network lock")).Changed() {
			a.toggleLock()
			a.syncTray()
		}
	})
	setting(c, L("代理地址", "Proxy address"), L("留空就自动找：Windows 系统代理、环境变量、常用端口。写了就只用它，比如 127.0.0.1:7890。", "Empty, Nook finds it: Windows' system proxy, environment variables, common ports. Given, only that one, as 127.0.0.1:7890."), func() {
		ui.Row(c).Gap(6).Children(func() {
			in := ui.TextInput(c, &a.proxyInput).Placeholder(L("自动", "Automatic")).Width(170)
			if ui.Button(c, L("应用", "Apply")).Clicked() || in.Submitted() {
				a.setProxy(a.proxyInput)
			}
		})
	})
	setting(c, L("AI 命令行", "AI command-line tools"), L("这些命令在 Nook 的终端里启动前，门卫先检查网络。绿点是这台电脑装了的。新加的名字重开 Nook 后生效。", "Before these commands start in Nook's terminals, the guard checks the network. A green dot: installed here. Names added take effect when Nook opens again."), func() {})
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
		in := ui.TextInput(c, &a.toolInput).Placeholder(L("加一个命令名，比如 mycli", "Add a command, as mycli")).Width(220)
		if ui.Button(c, L("添加", "Add")).Clicked() || in.Submitted() {
			a.addTool(a.toolInput)
			a.toolInput = ""
		}
	})
}

// paletteCard is a palette to choose: three of its colors and its name.
func (a *app) paletteCard(c *ui.Context, id, name string, colors []string) {
	t := c.Theme()
	c.Key("pal:" + id)
	chosen := a.st.Palette == id
	b := ui.ButtonBase(c).Label(name).Padding(8, 12).Radius(9).Border(1, t.Border)
	if chosen {
		b.Border(2, t.Accent)
	} else if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	b.Children(func() {
		ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
			for _, col := range colors {
				swatch(c, col, 16)
			}
			txt := ui.Text(c, name).FontSize(13).Padding(0, 0, 0, 6)
			if chosen {
				txt.Bold()
			}
		})
	})
	if b.Clicked() && !chosen {
		a.st.Palette = id
		if strings.HasPrefix(id, "my:") {
			a.st.Accent = "" // a look of the user's has its own
		}
		a.applyColors()
	}
}

func (a *app) appearanceSettings(c *ui.Context) {
	t := c.Theme()
	// The drawing board, first: the user's own looks.
	ui.Row(c).FillWidth().Margin(0, 0, 10, 0).Padding(14, 16).Gap(14).Radius(12).AlignItems(ui.Center).Border(1, t.Border).
		Gradient(t.Accent.Alpha(0.16), t.Surface, 120).Children(func() {
		ui.Icon(c, iconPalette).FontSize(26).TextColor(t.Accent)
		ui.Column(c).Grow(1).Shrink(1).Gap(3).Children(func() {
			ui.Text(c, L("画板：自己设计外观", "Drawing board: make your own look")).FontSize(14).Bold()
			ui.Text(c, L("点哪里改哪里：窗口、标题栏、侧栏、标签栏、面板、文字、终端，每一块都能配自己的颜色；还能套 Ghostty 主题、放背景图、做渐变、从图里取色。", "Click a part to color it: the window, the title bar, the sidebar, the tab bar, the panels, the text, the terminals; or a Ghostty theme, a picture, a gradient, the colors of a picture.")).FontSize(12).TextColor(t.TextMuted).Wrap()
		})
		label := L("打开画板", "Open")
		if a.currentDesign() == nil {
			label = L("新建一套", "New look")
		}
		if ui.PrimaryButton(c, label).Clicked() {
			a.openDesigner(c)
		}
	})
	if len(a.st.Designs) > 0 {
		setting(c, L("我的配色", "My looks"), L("在画板里做的。点一下换上，再点上面的「打开画板」接着改。", "Made on the drawing board. Click one to use it, then “Open” above to edit it."), func() {})
		ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {
			for _, d := range a.st.Designs {
				a.paletteCard(c, "my:"+d.ID, d.Name, []string{d.Background, d.Accent, d.Text})
			}
		})
	}
	setting(c, L("配色", "Colors"), L("窗口和终端一起换。也可以从 Ghostty 自带的几百套终端主题里挑一套，整个窗口跟着它的颜色走。", "The window and the terminals change together. Or pick one of the hundreds of terminal themes Ghostty ships: the whole window takes its colors."), func() {})
	ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {
		a.paletteCard(c, "", L("跟随系统", "System"), []string{hex(t.Background), hex(t.Accent), hex(t.Text)})
		for _, p := range palettes {
			a.paletteCard(c, p.id, L(p.zh, p.en), []string{p.bg, p.accent, p.text})
		}
		if name, ok := strings.CutPrefix(a.st.Palette, "ghostty:"); ok {
			if p, _, ok := ghosttyPalette(name); ok {
				a.paletteCard(c, a.st.Palette, name, []string{p.bg, p.accent, p.text})
			}
		}
		more := ui.Button(c, L("Ghostty 主题…", "Ghostty themes…"))
		if more.Clicked() {
			a.themeFor, a.themeQuery = "palette", ""
		}
	})
	if a.st.Palette == "" {
		setting(c, L("浅色还是深色", "Light or dark"), L("“跟随系统”配色下，跟着 Windows 的深色和浅色模式走，也可以固定一种。", "With the system's colors, follows Windows' dark or light mode, or keeps one."), func() {
			if choice(c, &a.st.Theme, [2]string{"", L("跟随系统", "System")}, [2]string{"light", L("浅色", "Light")}, [2]string{"dark", L("深色", "Dark")}) {
				a.save()
				a.applyTheme()
			}
		})
	}
	setting(c, L("强调色", "Accent"), L("按钮、选中、光标的颜色。", "Of buttons, selections and the cursor."), func() {})
	ui.Row(c).FillWidth().Wrap().Gap(8).AlignItems(ui.Center).Children(func() {
		auto := ui.ButtonBase(c).Label(L("跟随配色", "The palette's")).Padding(5, 10).Radius(7).Border(1, t.Border)
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
			dot := ui.ButtonBase(c).Label(col).Width(26).Height(26).Radius(13).Background(ui.Hex(col))
			if strings.EqualFold(a.st.Accent, col) {
				dot.Border(3, t.Text)
			}
			if dot.Clicked() {
				a.st.Accent = col
				a.applyColors()
			}
		}
		in := ui.TextInput(c, &a.accentInput).Placeholder("#rrggbb").Width(96)
		if in.Submitted() {
			if v := strings.TrimSpace(a.accentInput); len(v) == 7 && v[0] == '#' {
				a.st.Accent = strings.ToLower(v)
				a.applyColors()
			}
		}
	})
	setting(c, L("终端配色", "Terminal colors"), L("默认跟着上面的配色走，也可以单独给终端挑一套 Ghostty 主题。", "The palette's by default, or a Ghostty theme for the terminals alone."), func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			label := L("跟随配色", "The palette's")
			if a.st.TermTheme != "" {
				label = a.st.TermTheme
			}
			ui.Text(c, label).FontSize(12).TextColor(t.TextMuted)
			if ui.Button(c, L("挑一套…", "Pick…")).Clicked() {
				a.themeFor, a.themeQuery = "term", ""
			}
			if a.st.TermTheme != "" && ui.Button(c, L("跟随配色", "The palette's")).Clicked() {
				a.st.TermTheme = ""
				a.applyColors()
			}
		})
	})
	setting(c, L("终端字体", "Terminal font"), L("留空用系统的等宽字体。写字体名，比如 Cascadia Code。", "Empty, the system's monospaced font. Or a font's name, as Cascadia Code."), func() {
		ui.Row(c).Gap(6).Children(func() {
			in := ui.TextInput(c, &a.fontInput).Placeholder(L("系统等宽字体", "System monospace")).Width(170)
			if ui.Button(c, L("应用", "Apply")).Clicked() || in.Submitted() {
				a.st.TermFont = strings.TrimSpace(a.fontInput)
				a.save()
				a.applyFont() // the size as it is, the default kept
			}
		})
	})
	setting(c, L("终端字号", "Terminal font size"), L("Ctrl+= 和 Ctrl+- 只改当前终端，这里改全部。", "Ctrl+= and Ctrl+- change one terminal; this, all of them."), func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			size := a.fontSize()
			if ui.Button(c, "−").Clicked() {
				a.setFontSize(size - 1)
			}
			label := strconv.Itoa(size)
			if a.st.FontSize == 0 {
				label += L("（默认）", " (default)")
			}
			ui.Text(c, label).FontSize(13).MinWidth(28)
			if ui.Button(c, "+").Clicked() {
				a.setFontSize(size + 1)
			}
		})
	})
	setting(c, L("布局", "Layout"), L("左栏、右侧面板、联网面板、终端和旁边查看页之间的边都能拖动，双击还原。", "Drag the edges between the sidebar, the middle, the right panel, the network panel and a viewer beside the terminals; double-click one to reset it."), func() {
		if ui.Button(c, L("全部还原", "Reset all")).Clicked() {
			a.st.SidebarWidth, a.st.SideWidth, a.st.NetWidth, a.st.ViewerSplit = sidebarDefault, sideDefault, netDefault, nil
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
								a.st.Palette = "ghostty:" + name
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
			ui.Text(c, L("替你管住联网的 AI 命令行终端", "The AI terminal that keeps its networking in your hands")).FontSize(12).TextColor(t.TextMuted)
		})
	})
	ui.Text(c, L("Nook 自己不联网、不统计；只在你点了带小地球的按钮时，才会有网页或 AI 联网。不改 ~/.claude、~/.codex，也不碰项目自己的 .git。", "Nook itself never goes online and collects nothing; pages and AI tools go online only after a button with a globe. It never changes ~/.claude or ~/.codex, nor a project's .git.")).FontSize(13).PaddingY(8)
	setting(c, L("设置文件", "Settings"), config.Path(), func() {})
	setting(c, L("撤销用的备份", "Copies for undoing"), L(`默认放在每个项目所在的盘（X:\.nook\backups），保留或撤销后自动删掉。可以在「通用」里改。`, `On each project's own drive by default (X:\.nook\backups), gone once you keep or undo the changes. Change it in General.`), func() {})
}
