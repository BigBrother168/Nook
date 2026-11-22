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

// openSettings shows the settings in the middle.
func (a *app) openSettings() {
	a.proxyInput, a.accentInput, a.fontInput = a.st.Proxy, a.st.Accent, a.st.TermFont
	a.viewing = "settings"
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

// choice is one of a few options, as buttons side by side.
func choice(c *ui.Context, value *string, options ...[2]string) bool {
	t := c.Theme()
	changed := false
	ui.Row(c).Gap(4).Children(func() {
		for _, o := range options {
			c.Key(o[0])
			b := ui.ButtonBase(c).Label(o[1]).Padding(5, 12).Radius(6).Border(1, t.Border)
			if *value == o[0] {
				b.Background(t.Selection)
			} else if b.Hovered() {
				b.Background(t.SurfaceHover)
			}
			b.Children(func() { ui.Text(c, o[1]).FontSize(12) })
			if b.Clicked() && *value != o[0] {
				*value, changed = o[0], true
			}
		}
	})
	return changed
}

// setting is a row of the settings: a label, what it does, its control.
func setting(c *ui.Context, label, help string, control func()) {
	t := c.Theme()
	ui.Row(c).FillWidth().Gap(16).PaddingY(8).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, label).FontSize(13)
			if help != "" {
				ui.Text(c, help).FontSize(12).TextColor(t.TextMuted)
			}
		})
		control()
	})
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
	var opts [][2]string
	for _, sh := range a.shells {
		opts = append(opts, [2]string{sh.ID, sh.Name})
	}
	setting(c, L("默认 shell", "Default shell"), L("新建终端时用的命令行", "What a new terminal runs"), func() {
		id := a.defaultShell().ID
		if choice(c, &id, opts...) {
			a.setDefaultShell(id)
		}
	})
}

func (a *app) requireChoice(c *ui.Context) {
	setting(c, requireLabel(), L("在国内一定要打开：没有代理也没有虚拟网卡时，AI 不能启动，不会用真实 IP 连出去。在国外不用代理就关掉。", "In China keep it on: with no proxy and no tunnel, AI tools cannot start, and nothing connects from your real IP. Outside China without a proxy, turn it off."), func() {
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
		{"Ctrl+Shift+W", L("关闭终端", "Close the terminal")},
		{"Ctrl+Shift+D / E", L("向右 / 向下分屏", "Split right / down")},
		{"Ctrl+Tab / Ctrl+Shift+Tab", L("下一个 / 上一个标签", "Next / previous tab")},
		{"Ctrl+Shift+S", L("显示 / 隐藏左侧栏", "Show / hide the sidebar")},
		{"Ctrl+Shift+F", L("在终端里找（包括翻上去的内容）", "Find in the terminal, its scrollback too")},
		{"Ctrl+Shift+B", L("右侧面板：改动、文件、预览", "Right panel: changes, files, previews")},
		{"Ctrl+Shift+,", L("设置", "Settings")},
		{"Ctrl+C / Ctrl+V", L("有选中文字时复制 / 粘贴（截图粘贴给 AI）", "Copy a selection / paste (a screenshot goes to the AI)")},
		{"Ctrl+= / Ctrl+- / Ctrl+0", L("放大 / 缩小 / 还原当前终端的字", "Larger / smaller / reset the terminal's text")},
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
			ui.Text(c, L("替你管住联网的 AI 命令行终端：中间跑原版 Claude Code、Codex 这些 AI 命令行，联网的每一步都看得见、由你决定。", "The AI terminal that keeps its networking in your hands: the AI command-line tools themselves, as Claude Code and Codex, run inside; you see and decide every step they take online.")).FontSize(13).TextColor(t.TextMuted)
			a.languageChoice(c)
			tools := a.installedTools()
			found := L("没找到。装好 Claude Code、Codex 这类 AI 命令行后，在终端里直接输入它的名字就行。", "None found. Install an AI command-line tool, as Claude Code or Codex, then type its name in a terminal.")
			if len(tools) > 0 {
				found = strings.Join(tools, L("、", ", "))
			}
			setting(c, L("找到的 AI 命令行", "AI tools found"), found, func() {})
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
				ui.Text(c, L("这些以后都能在设置里改（Ctrl+Shift+,）。", "Change these any time in the settings (Ctrl+Shift+,).")).FontSize(12).TextColor(t.TextMuted).Grow(1)
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
