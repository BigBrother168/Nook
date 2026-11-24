package main

import (
	"net"
	"os/exec"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"nook/core/lang"
	"nook/core/netguard"
)

// The settings page, and the first run's welcome, which asks the same
// few things.

// openSettings shows the settings, in a dialog over the window.
func (a *app) openSettings() {
	a.proxyInput, a.accentInput, a.fontInput = a.st.Proxy, a.st.Accent, a.st.TermFont
	a.settings = "settings"
}

// onPage reports whether the settings' dialog shows, the drawing board's
// too.
func (a *app) onPage() bool { return a.settings != "" }

// leavePages closes the settings' dialog, whatever it shows; the window
// was there behind it all along.
func (a *app) leavePages() {
	if a.settings == "" {
		return
	}
	a.settings, a.designFrom = "", false
	a.save()
	if s := a.focused(); s != nil {
		a.pendingFocus = s
	}
}

// backButton is the “Back” at the top left of a dialog's page.
func backButton(c *ui.Context, label string) bool {
	t := c.Theme()
	b := ui.ButtonBase(c).Label(label).Padding(5, 10, 5, 6).Radius(7)
	if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	b.Children(func() {
		ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconBack).FontSize(15)
			ui.Text(c, label).FontSize(13)
		})
	})
	return b.Clicked()
}

// applyLanguage sets the language of Nook's texts and of MyGo's own.
func (a *app) applyLanguage() {
	lang.Set(a.st.Language, lang.System())
	if lang.English {
		mygo.App.SetLocale("en")
	} else {
		mygo.App.SetLocale("zh-Hans")
	}
	a.syncTray()
}

// applyTheme sets light, dark, or the system's.
func (a *app) applyTheme() {
	switch a.st.Theme {
	case "light":
		mygo.Theme.SetSource(mygo.ThemeLight)
	case "dark":
		mygo.Theme.SetSource(mygo.ThemeDark)
	default:
		mygo.Theme.SetSource(mygo.ThemeSystem)
	}
}

// termFont is the terminals' font: the system's monospaced font, at the
// size chosen.
func (a *app) termFont() terminal.Font {
	return terminal.Font{Family: a.st.TermFont, Size: float32(a.st.FontSize)}
}

// defaultFontSize is the terminals' font size until one is chosen, the
// terminal's own.
const defaultFontSize = 13

// fontSize is the terminals' font size.
func (a *app) fontSize() int {
	if a.st.FontSize == 0 {
		return defaultFontSize
	}
	return a.st.FontSize
}

// setFontSize sets the terminals' font size, the open ones too.
func (a *app) setFontSize(size int) {
	a.st.FontSize = min(max(size, 9), 28)
	a.save()
	a.applyFont()
}

// applyFont gives the open terminals the font of the settings.
func (a *app) applyFont() {
	for _, s := range a.sessions() {
		if s.term != nil {
			s.term.SetFont(a.termFont())
		}
	}
}

// installedTools returns the AI tools of the list this computer has.
func (a *app) installedTools() []string {
	var out []string
	for _, t := range a.st.AITools {
		if _, err := exec.LookPath(t); err == nil {
			out = append(out, t)
		}
	}
	return out
}

// choice is one of a few options, in one strip.
func choice(c *ui.Context, value *string, options ...[2]string) bool {
	sel, labels := -1, make([]string, len(options))
	for i, o := range options {
		labels[i] = o[1]
		if *value == o[0] {
			sel = i
		}
	}
	i, changed := segmented(c, sel, labels...)
	if changed {
		*value = options[i][0]
	}
	return changed
}

// segmented is a few options in one strip, the one chosen lifted out of
// it: the index chosen, and whether a click changed it.
func segmented(c *ui.Context, sel int, labels ...string) (int, bool) {
	t := c.Theme()
	out := sel
	ui.Row(c).Padding(2).Gap(2).Radius(8).Background(t.Text.Alpha(0.06)).Children(func() {
		for i, l := range labels {
			c.Key(i)
			b := ui.ButtonBase(c).Label(l).Height(26).PaddingX(12).Radius(6)
			switch {
			case i == sel && t.Dark:
				b.Background(t.Text.Alpha(0.14))
			case i == sel:
				b.Background(ui.RGB(255, 255, 255)).Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.12))
			case b.Hovered():
				b.Background(t.Text.Alpha(0.05))
			}
			b.Children(func() {
				txt := ui.Text(c, l).FontSize(12).SingleLine()
				if i != sel {
					txt.TextColor(t.TextMuted)
				}
			})
			if b.Clicked() && i != sel {
				out = i
			}
		}
	})
	return out, out != sel
}

// stepper is a number between − and +, in one piece: the number after a
// click.
func stepper(c *ui.Context, v int, label string) int {
	t := c.Theme()
	out := v
	ui.Row(c).Radius(8).AlignItems(ui.Center).Background(t.Text.Alpha(0.06)).Children(func() {
		step := func(sign, tip string, d int) {
			b := ui.ButtonBase(c).Label(tip).Width(30).Height(28).Radius(8)
			if b.Hovered() {
				b.Background(t.Text.Alpha(0.06))
			}
			b.Children(func() { ui.Text(c, sign).FontSize(15) })
			if b.Clicked() {
				out = v + d
			}
		}
		step("−", L("小一号", "Smaller"), -1)
		ui.Text(c, itoa(v)).FontSize(13).MinWidth(24).TextAlign(ui.Center).Label(label)
		step("+", L("大一号", "Larger"), 1)
	})
	return out
}

// settingRows counts the rows of the settings shown so far, that a line
// parts each from the one above.
var settingRows int

// settingLine is the hairline over every row of the settings but the
// first.
func settingLine(c *ui.Context) {
	if settingRows++; settingRows > 1 {
		ui.Box(c).FillWidth().Height(1).Background(c.Theme().Border.Alpha(0.6))
	}
}

// setting is a row of the settings: a label, what it does, its control.
func setting(c *ui.Context, label, help string, control func()) {
	t := c.Theme()
	settingLine(c)
	ui.Row(c).FillWidth().Gap(16).PaddingY(11).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Shrink(1).Gap(2).Children(func() {
			ui.Text(c, label).FontSize(13)
			if help != "" {
				ui.Text(c, help).FontSize(12).TextColor(t.TextMuted)
			}
		})
		control()
	})
}

// settingStack is a row of the settings whose control is too wide to sit
// beside its label: under it.
func settingStack(c *ui.Context, label, help string, body func()) {
	t := c.Theme()
	settingLine(c)
	ui.Column(c).FillWidth().Gap(10).PaddingY(11).AlignItems(ui.Start).Children(func() {
		ui.Column(c).Gap(2).Children(func() {
			ui.Text(c, label).FontSize(13)
			if help != "" {
				ui.Text(c, help).FontSize(12).TextColor(t.TextMuted)
			}
		})
		body()
	})
}

// settingLink is a row of the settings that opens a page: an icon, what
// the page is for, and ›.
func settingLink(c *ui.Context, icon *ui.SVG, label, help string) bool {
	t := c.Theme()
	settingLine(c)
	r := ui.Row(c).FillWidth().Gap(12).PaddingY(11).PaddingX(4).Radius(8).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if r.Hovered() {
		r.Background(t.Text.Alpha(0.04))
	}
	r.Children(func() {
		ui.Icon(c, icon).FontSize(18).TextColor(t.Accent)
		ui.Column(c).Grow(1).Shrink(1).Gap(2).Children(func() {
			ui.Text(c, label).FontSize(13)
			if help != "" {
				ui.Text(c, help).FontSize(12).TextColor(t.TextMuted)
			}
		})
		ui.Icon(c, iconChevronRight).FontSize(15).TextColor(t.TextMuted)
	})
	return r.Clicked()
}

func section(c *ui.Context, title string) {
	ui.Text(c, title).FontSize(15).Bold().Padding(18, 0, 4, 0)
}

// languageChoice, shellChoice and requireChoice are settings the welcome
// asks too.
func (a *app) languageChoice(c *ui.Context) {
	setting(c, L("语言", "Language"), "", func() {
		if choice(c, &a.st.Language, [2]string{"", L("跟随系统", "System")}, [2]string{"zh", "中文"}, [2]string{"en", "English"}) {
			a.save()
			a.applyLanguage()
		}
	})
}

func (a *app) shellChoice(c *ui.Context) {
	setting(c, L("新终端用", "New terminals run"), "", func() {
		def := a.defaultShell()
		ui.MenuButton(c, def.Name, func(m *ui.Menu) {
			for _, sh := range a.shells {
				if m.Item(sh.Name).Radio(sh.ID == def.ID).Chosen() {
					a.setDefaultShell(sh.ID)
				}
			}
		})
	})
}

func (a *app) requireChoice(c *ui.Context) {
	setting(c, requireLabel(), L("防止露出真实 IP。在国外不用代理就关掉。", "Keeps your real IP hidden. Without a proxy abroad, turn it off."), func() {
		if ui.Switch(c, &a.requireSel).Label(requireLabel()).Changed() {
			a.st.SetRequireProxy(a.requireSel)
			a.save()
		}
	})
}

// / shortcutList is Nook's keys, with what they do.
func shortcutList() [][2]string {
	return [][2]string{
		{"Ctrl+Shift+P", L("命令面板", "Command palette")},
		{"Ctrl+Shift+T", L("新建终端", "New terminal")},
		{"Ctrl+Shift+W", L("结束终端（5 秒内可撤销）", "End the terminal (undo within 5 seconds)")},
		{"Ctrl+Shift+D / E", L("向右 / 向下分屏", "Split right / down")},
		{"Ctrl+Tab / Ctrl+Shift+Tab", L("下一个 / 上一个标签", "Next / previous tab")},
		{"Ctrl+Shift+S", L("显示 / 隐藏左侧栏", "Show / hide the sidebar")},
		{"Ctrl+Shift+F", L("在终端里找", "Find in the terminal")},
		{"Ctrl+Shift+B", L("右侧面板", "The right panel")},
		{"Ctrl+Shift+,", L("设置", "Settings")},
		{"Ctrl+C / Ctrl+V", L("复制 / 粘贴（截图也能贴给 AI）", "Copy / paste (screenshots too, for the AI)")},
		{"Ctrl+= / Ctrl+- / Ctrl+0", L("终端的字放大 / 缩小 / 还原", "The terminal's text larger / smaller / reset")},
	}
}

// setProxy sets the proxy given by hand, "" for the one found.
func (a *app) setProxy(s string) {
	s = strings.TrimSpace(s)
	if s == a.st.Proxy {
		return
	}
	if _, port, err := net.SplitHostPort(s); s != "" && (err != nil || !validPort(port)) {
		a.notice = L("代理地址要写成 主机:端口，比如 127.0.0.1:7890", "Write the proxy as host:port, as 127.0.0.1:7890")
		return
	}
	a.st.Proxy = s
	a.save()
	a.live.configure(a.st)
}

// addTool adds a command to the AI tools.
func (a *app) addTool(name string) {
	name = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), ".exe"))
	if name == "" || !validTool(name) || slices.Contains(a.st.AITools, name) {
		return
	}
	a.st.MoreAITools = append(a.st.MoreAITools, name)
	a.st.AITools = append(a.st.AITools, name)
	a.save()
	a.live.configure(a.st)
}

// welcome is the first run's page: what Nook found, and the few choices.
func (a *app) welcome(c *ui.Context) {
	if a.st.Onboarded {
		return
	}
	t := c.Theme()
	open := true
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(560).Gap(10).Children(func() {
			ui.Text(c, L("欢迎使用 Nook", "Welcome to Nook")).FontSize(18).Bold()
			ui.Text(c, L("替你管住联网的 AI 终端。", "The AI terminal that keeps its networking in your hands.")).FontSize(13).TextColor(t.TextMuted)
			settingRows = 0
			a.languageChoice(c)
			tools := a.installedTools()
			found := L("没找到。装好后在终端里输入名字就行。", "None found. Once installed, type its name in a terminal.")
			if len(tools) > 0 {
				found = strings.Join(tools, L("、", ", "))
			}
			setting(c, L("找到的 AI", "AI found"), found, func() {})
			a.shellChoice(c)
			p := a.net.proxy
			netText := L("没找到代理", "No proxy found")
			if p.Works() {
				netText = L("代理已开：", "Proxy on: ") + p.Addr()
			}
			if tun := netguard.Carrying(a.net.tunnels); len(tun) > 0 {
				netText += L("；虚拟网卡已开：", "; tunnel on: ") + tun[0].Name
			}
			setting(c, L("网络", "Network"), netText, func() {})
			a.requireChoice(c)
			ui.Row(c).FillWidth().Padding(8, 0, 0, 0).Children(func() {
				ui.Text(c, L("以后都能在设置里改", "All of this can change later, in the settings")).FontSize(12).TextColor(t.TextMuted).Grow(1)
				if ui.PrimaryButton(c, L("开始使用", "Start")).Clicked() {
					open = false
				}
			})
		})
	})
	if !open {
		a.st.Onboarded = true
		a.save()
	}
}

func validPort(s string) bool {
	n, err := strconv.Atoi(s)
	return err == nil && n > 0 && n < 65536
}
