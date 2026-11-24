package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"nook/core/netguard"
)

var (
	iconLock   = lucide(`<rect width="18" height="11" x="3" y="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/>`)
	iconUnlock = lucide(`<rect width="18" height="11" x="3" y="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 9.9-1"/>`)
)

func requireLabel() string { return L("没有代理时，不让 AI 启动", "No proxy, no AI") }

// topBarHeight is the top bar's height, which is the window's title bar.
const topBarHeight = 38

// topBar is the window's title bar: the project in use, then what matters
// most, whether the real IP shows, a click away from the network tab, and
// the network lock. The window's buttons sit at its right; it drags the
// window.
func (a *app) topBar(c *ui.Context) {
	t := c.Theme()
	bar := c.TitleBar()
	bg := a.panelColor(t, "titlebar")
	a.windowButtons(bg.Over(a.chromeColor(t)))
	row := ui.Row(c).FillWidth().Height(max(bar.Height, topBarHeight)).PaddingStart(max(bar.Left, 10)).PaddingEnd(bar.Right + 8).Gap(8).AlignItems(ui.Center).Background(bg).DragWindow()
	row.Children(func() {
		tip := L("隐藏侧边栏（Ctrl+Shift+S）", "Hide the sidebar (Ctrl+Shift+S)")
		if a.st.SidebarHidden {
			tip = L("显示侧边栏（Ctrl+Shift+S）", "Show the sidebar (Ctrl+Shift+S)")
		}
		if iconButton(c, iconPanelLeft, tip).Clicked() {
			a.toggleSidebar()
		}
		ui.Text(c, "Nook").FontSize(13).Bold()
		where := ""
		if a.group != "" {
			where = a.projectName(a.group)
		}
		if where != "" {
			ui.Text(c, "— "+where).FontSize(13).TextColor(t.TextMuted).SingleLine().Shrink(1)
		}
		ui.Spacer(c)
		col, text, vtip := a.verdict(t)
		if chip(c, col, nil, text, vtip).Clicked() {
			a.leavePages()
			a.toggleSide(sideNet)
		}
		lock, lockCol, lockTip := iconUnlock, t.TextMuted, L("联网锁：锁上后 AI 不能启动", "Network lock: locked, no AI can start")
		if a.st.Locked {
			lock, lockCol, lockTip = iconLock, t.Danger, L("联网已锁，AI 不能启动。点一下解开", "Locked: no AI can start. Click to unlock")
		}
		b := ui.ButtonBase(c).Label(lockTip).Padding(6).Radius(7).Tooltip(lockTip)
		if b.Hovered() {
			b.Background(t.SurfaceHover)
		}
		b.Children(func() { ui.Icon(c, lock).FontSize(15).TextColor(lockCol) })
		if b.Clicked() {
			a.toggleLock()
			a.syncTray()
		}
	})
}

// windowButtons draws the window's buttons for a title bar of color bg:
// light on a dark one, dark on a light one.
func (a *app) windowButtons(bg ui.Color) {
	want := 2
	if luminance(bg) < 0.5 {
		want = 1
	}
	if a.win == nil || a.winDark == want {
		return
	}
	a.winDark = want
	theme := mygo.ThemeLight
	if want == 1 {
		theme = mygo.ThemeDark
	}
	a.win.SetWindowButtonsTheme(theme)
}

// verdict is the top bar's word on the network: whether the real IP
// shows, in color, and the details.
func (a *app) verdict(t *ui.Theme) (ui.Color, string, string) {
	p := a.net.proxy
	tunnel := len(netguard.Carrying(a.net.tunnels)) > 0
	proxy := L("没用代理", "No proxy")
	switch {
	case p.Works():
		proxy = L("代理 ", "Proxy ") + p.Addr()
	case tunnel:
		proxy = L("虚拟网卡已开", "Tunnel on")
	}
	n, direct := a.net.running()
	switch {
	case direct > 0 || a.net.elsewhereDirect() > 0:
		return t.Danger, L("真实 IP 可能露了", "Your real IP may show"), L("有 AI 没走代理，点开看是哪个", "An AI goes around the proxy; click to see which")
	case a.st.Requires() && !p.Works() && !tunnel:
		return t.Danger, L("代理没开", "Proxy off"), L("没有代理，AI 不能启动", "With no proxy, no AI can start")
	case a.net.online() > 0:
		return t.Success, L("真实 IP 没露", "Real IP hidden"), proxy + L(" · AI 都走代理", " · AI all through the proxy")
	case n > 0:
		return t.TextMuted, L("AI 还没联网", "AI not online yet"), proxy
	case p.Works() || tunnel:
		return t.Success, L("代理已开", "Proxy on"), proxy
	}
	return t.TextMuted, L("没用代理", "No proxy"), L("点开看联网", "Click for the network")
}

// chip is a rounded status in the top bar: a dot (or icon) in col, and
// words, in col too when it warns or says nothing runs, else in the
// text's color, which reads on any background.
func chip(c *ui.Context, col ui.Color, icon *ui.SVG, text, tip string) ui.Element {
	t := c.Theme()
	r := ui.Row(c).Height(26).PaddingX(10).Gap(6).AlignItems(ui.Center).Radius(13).Border(1, t.Border).Cursor(ui.CursorPointer).Tooltip(tip)
	if r.Hovered() {
		r.Background(t.SurfaceHover)
	}
	r.Children(func() {
		if icon != nil {
			ui.Icon(c, icon).FontSize(13).TextColor(col)
		} else {
			ui.Box(c).Width(7).Height(7).Radius(4).Background(col)
		}
		textCol := t.Text
		if col == t.Danger || col == t.TextMuted {
			textCol = col
		}
		ui.Text(c, text).FontSize(12).TextColor(textCol).SingleLine()
	})
	return r
}

// toggleLock locks or unlocks the network. The guard reads the lock from
// the settings each time a tool starts. AI tools already running are not
// ended unasked: the panel lists them, with a button to end them.
func (a *app) toggleLock() {
	a.st.Locked = !a.st.Locked
	a.save()
	if a.st.Locked {
		a.lockedWeb()
	}
	if n, _ := a.net.running(); a.st.Locked && n > 0 {
		a.side, a.sideTab = true, sideNet
	}
}

// sourceText says where Nook found the proxy (netguard's Source).
func sourceText(source string) (string, bool) {
	switch source {
	case "手动设置":
		return L("手动设置的", "Set by hand"), true
	case "系统代理":
		return L("从 Windows 系统代理找到的", "From Windows' system proxy"), true
	case "环境变量":
		return L("从环境变量找到的", "From environment variables"), true
	case "常用端口":
		return L("在常用代理端口上找到的", "On a common proxy port"), true
	}
	return "", false
}

// programNames are what the parents of AI tools are called, in words.
var programNames = map[string]string{
	"pwsh.exe":              "PowerShell",
	"powershell.exe":        "Windows PowerShell",
	"cmd.exe":               "cmd",
	"bash.exe":              "Git Bash",
	"windowsterminal.exe":   "Windows Terminal",
	"explorer.exe":          "Explorer",
	"code.exe":              "VS Code",
	"cursor.exe":            "Cursor",
	"chatgpt.exe":           "ChatGPT",
	"claude.exe":            "Claude",
	"openconsole.exe":       "OpenConsole",
	"windowsterminalx.exe":  "Windows Terminal",
	"wezterm-gui.exe":       "WezTerm",
	"alacritty.exe":         "Alacritty",
	"nook.exe":              "Nook",
	"node.exe":              "Node",
	"idea64.exe":            "IntelliJ IDEA",
	"goland64.exe":          "GoLand",
	"pycharm64.exe":         "PyCharm",
	"webstorm64.exe":        "WebStorm",
	"windowsterminal-x.exe": "Windows Terminal",
}

// programName says what started an AI tool.
func programName(exe string) string {
	if n, ok := programNames[strings.ToLower(exe)]; ok {
		return n
	}
	return strings.TrimSuffix(exe, ".exe")
}

// netTab is the right panel's network tab: first whether the real IP
// shows; the proxy, the tunnel and Nook itself in a line; the switch that
// keeps AI tools from starting without a proxy; then every AI tool, in
// Nook and elsewhere, a line each.
func (a *app) netTab(c *ui.Context) {
	t := c.Theme()
	heading := func(s, tip string) {
		h := ui.Text(c, s).FontSize(12).Bold().TextColor(t.TextMuted).Margin(14, 0, 4, 0)
		if tip != "" {
			h.Tooltip(tip)
		}
	}
	ui.Scroll(c).Grow(1).FillWidth().Children(func() {
		ui.Column(c).FillWidth().PaddingX(12).PaddingY(10).Gap(2).Children(func() {
			a.ipVerdict(c)
			a.netLine(c)
			ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Margin(10, 0, 0, 0).Tooltip(L("没有代理也没有虚拟网卡时，AI 不能启动，不会用真实 IP 连出去。在国外不用代理就关掉。", "With no proxy and no tunnel, no AI tool starts, and nothing connects from your real IP. Outside China without a proxy, turn it off.")).Children(func() {
				ui.Column(c).Grow(1).Shrink(1).Gap(1).Children(func() {
					// The words switch it too, as a checkbox's label does.
					if ui.Text(c, requireLabel()).FontSize(13).Cursor(ui.CursorPointer).Clicked() {
						a.requireSel = !a.requireSel
						a.st.SetRequireProxy(a.requireSel)
						a.save()
					}
					ui.Text(c, L("防止露出真实 IP", "Keeps your real IP hidden")).FontSize(11).TextColor(t.TextMuted)
				})
				if ui.Switch(c, &a.requireSel).Label(requireLabel()).Changed() {
					a.st.SetRequireProxy(a.requireSel)
					a.save()
				}
			})

			heading(L("Nook 里的 AI", "AI in Nook"), "")
			if len(a.net.ai) == 0 {
				ui.Text(c, L("没有在运行", "None running")).FontSize(12).TextColor(t.TextMuted).PaddingY(4)
			}
			for _, s := range a.sessions() {
				u, ok := a.net.ai[s.id]
				if !ok {
					continue
				}
				c.Key(s.id)
				if aiRow(c, a.termLabel(s), a.projectLabel(s), u.routes, L("点一下跳到这个终端", "Click to go to this terminal")).Clicked() {
					a.show(s)
				}
			}
			if n, _ := a.net.running(); n > 0 && a.st.Locked {
				if smallButton(c, L("结束 Nook 里的 AI", "End the AI in Nook"), false).Tooltip(L("锁只拦新启动的", "The lock stops new ones only")).Clicked() {
					a.endAI()
				}
			}

			heading(L("其他地方的 AI", "AI elsewhere"), L("不是 Nook 开的：看得见，管不了", "Not started by Nook: shown, not controlled"))
			if len(a.net.others) == 0 {
				ui.Text(c, L("没有", "None")).FontSize(12).TextColor(t.TextMuted).PaddingY(4)
			}
			for _, g := range groupOthers(a.net.others) {
				a.otherGroup(c, g)
			}
		})
	})
}

// netLine says in one line where the proxy is, whether the tunnel is on,
// and that Nook itself stays offline; more on hover.
func (a *app) netLine(c *ui.Context) {
	t := c.Theme()
	p := a.net.proxy
	part := func(text string, col ui.Color, tip string) {
		tx := ui.Text(c, text).FontSize(12).TextColor(col).SingleLine()
		if tip != "" {
			tx.Tooltip(tip)
		}
	}
	dot := func() { ui.Text(c, "·").FontSize(12).TextColor(t.TextMuted) }
	ui.Row(c).FillWidth().Wrap().Gap(6).AlignItems(ui.Center).Padding(4, 2).Children(func() {
		tip, _ := sourceText(p.Source)
		switch {
		case p.Port == 0:
			part(L("没找到代理", "No proxy found"), t.TextMuted, tip)
		case p.Works():
			part(L("代理 ", "Proxy ")+p.Addr(), t.Success, tip)
		default:
			part(L("代理没开 ", "Proxy off ")+p.Addr(), t.Danger, tip)
		}
		dot()
		carrying := len(netguard.Carrying(a.net.tunnels)) > 0
		tun := tunTip(carrying)
		switch {
		case carrying:
			part(L("虚拟网卡已开", "Tunnel on"), t.Success, tun)
		case len(a.net.tunnels) > 0:
			part(L("虚拟网卡没在用", "Tunnel unused"), t.TextMuted, tun)
		default:
			part(L("虚拟网卡没开", "No tunnel"), t.TextMuted, tun)
		}
		dot()
		if a.net.self == 0 {
			part(L("Nook 不联网", "Nook offline"), t.Success, L("Nook 自己没有任何连接", "Nook itself has no connections"))
		} else {
			part(L("Nook 有 ", "Nook: ")+conns(a.net.self), t.Danger, "")
		}
	})
}

// aiRow is an AI tool in the network tab: its name, where it runs, and
// its connections at the right, green through the proxy, red direct.
func aiRow(c *ui.Context, name, where string, r netguard.Routes, tip string) ui.Element {
	t := c.Theme()
	row := ui.Row(c).FillWidth().Height(30).PaddingX(8).Gap(8).AlignItems(ui.Center).Radius(6)
	if tip != "" {
		row.Cursor(ui.CursorPointer).Tooltip(routesText(r) + L("。", ". ") + tip)
		if row.Hovered() {
			row.Background(t.SurfaceHover)
		}
	} else {
		row.Tooltip(routesText(r))
	}
	row.Children(func() {
		ui.Text(c, name).FontSize(13).SingleLine().Shrink(1)
		if where != "" {
			ui.Text(c, where).FontSize(11).TextColor(t.TextMuted).SingleLine().Shrink(2)
		}
		ui.Spacer(c)
		countChip(c, r)
	})
	return row
}

// countChip says how many connections, colored by where they go.
func countChip(c *ui.Context, r netguard.Routes) {
	t := c.Theme()
	total := r.Total()
	proxy, tun := r[netguard.ViaProxy], r[netguard.ViaTUN]
	col, text := t.TextMuted, L("没连接", "idle")
	switch {
	case r[netguard.Direct] > 0:
		col, text = t.Danger, fmt.Sprintf(L("%d 条 · 直连 %d", "%d · %d direct"), total, r[netguard.Direct])
	case total > 0 && tun == total:
		col, text = t.Success, fmt.Sprintf(L("%d 条 · 走虚拟网卡", "%d · via tunnel"), total)
	case total > 0 && proxy+tun == total:
		col, text = t.Success, fmt.Sprintf(L("%d 条 · 走代理", "%d · via proxy"), total)
	case total > 0:
		text = fmt.Sprintf(L("%d 条 · 连本机", "%d · local"), total)
	}
	ui.Text(c, text).FontSize(12).TextColor(col).Shrink(0)
}

// leak is an AI tool connecting straight out, around the proxy: where
// the real IP shows.
type leak struct {
	who string // "codex · Nook", "ChatGPT 桌面版"
	n   int    // its direct connections
	s   *session
}

// leaks returns the AI tools going direct, those in Nook first.
func (a *app) leaks() []leak {
	var out []leak
	for _, s := range a.sessions() {
		if u, ok := a.net.ai[s.id]; ok && u.routes[netguard.Direct] > 0 {
			out = append(out, leak{strings.Join(u.tools, L("、", ", ")) + " · " + a.projectLabel(s), u.routes[netguard.Direct], s})
		}
	}
	for _, g := range groupOthers(a.net.others) {
		n, tools := 0, []string{}
		for _, o := range g.tools {
			if d := o.routes[netguard.Direct]; d > 0 {
				n += d
				if tool := strings.TrimSuffix(o.name, ".exe"); !slices.Contains(tools, tool) {
					tools = append(tools, tool)
				}
			}
		}
		if n > 0 {
			out = append(out, leak{who: strings.Join(tools, L("、", ", ")) + " · " + g.name, n: n})
		}
	}
	return out
}

// ipVerdict says first what matters most: whether any AI tool shows the
// real IP, connecting around the proxy.
func (a *app) ipVerdict(c *ui.Context) {
	t := c.Theme()
	leaks := a.leaks()
	tunOn := len(netguard.Carrying(a.net.tunnels)) > 0
	col, title := t.TextMuted, L("现在没有 AI 在联网", "No AI tool online now")
	switch {
	case len(leaks) > 0:
		col, title = t.Danger, L("⚠ 真实 IP 可能露出去了", "⚠ Your real IP may show")
	case a.net.online() > 0: // no connection yet: nothing went through the proxy either
		col, title = t.Success, L("✓ 真实 IP 没露出去", "✓ Your real IP stays hidden")
	}
	card := ui.Column(c).FillWidth().Margin(6, 0, 6, 0).Padding(10, 12).Gap(6).Radius(8).Background(col.Alpha(0.12)).Border(1, col.Alpha(0.45))
	if len(leaks) == 0 && col == t.Success {
		if tunOn {
			card.Tooltip(L("虚拟网卡开着：所有程序的流量都走代理。", "The tunnel is on: every program's traffic goes through the proxy."))
		} else {
			card.Tooltip(L("Nook 看的是 TCP 连接；打开 TUN 模式更稳妥", "Nook watches TCP connections; TUN mode is safer still"))
		}
	}
	card.Children(func() {
		ui.Text(c, title).FontSize(14).Bold().TextColor(col).Wrap()
		if len(leaks) == 0 {
			if col == t.Success {
				ui.Text(c, L("AI 都走代理，没有直连", "AI all through the proxy, none direct")).FontSize(12).TextColor(t.TextMuted).Wrap()
			}
			return
		}
		inNook, outside := false, false
		for _, l := range leaks {
			outside = outside || l.s == nil
			line := ui.Text(c, fmt.Sprintf(L("%s：%s没走代理", "%s: %s around the proxy"), l.who, conns(l.n))).FontSize(13).Wrap()
			if l.s != nil {
				inNook = true
				line.Cursor(ui.CursorPointer).Tooltip(L("点一下跳到这个终端", "Click to go to this terminal"))
				if line.Clicked() {
					a.show(l.s)
				}
			}
		}
		if inNook && smallButton(c, L("结束 Nook 里的 AI", "End the AI tools in Nook"), true).Clicked() {
			a.endAI()
		}
		if outside {
			ui.Text(c, L("Nook 外面的 AI 管不了，可以把它退出", "AI outside Nook is not Nook's to stop; quit it")).FontSize(12).TextColor(t.TextMuted).Wrap()
		}
		if !tunOn {
			ui.Text(c, L("打开 TUN 模式，能把这些也兜进代理", "TUN mode takes these through the proxy too")).FontSize(12).TextColor(t.TextMuted).Wrap()
		}
	})
}

// tunTip suggests the tunnel, which takes every program through the
// proxy, those that skip the proxy settings too.
func tunTip(on bool) string {
	if on {
		return L("建议一直开着：所有程序都会走代理，比只开代理更稳。", "Keep it on: every program goes through the proxy, more reliably than with the proxy alone.")
	}
	return L("建议在代理软件里打开 TUN 模式，所有程序都走代理", "Turn on TUN mode in your proxy app: every program goes through the proxy")
}

// routesText says where connections go, in a few words.
func routesText(r netguard.Routes) string {
	total := r.Total()
	if total == 0 {
		return L("暂时没有连接", "No connections now")
	}
	proxy, tun := r[netguard.ViaProxy], r[netguard.ViaTUN]
	switch total {
	case proxy:
		return conns(total) + L("，都走代理", ", all through the proxy")
	case tun:
		return conns(total) + L("，都走虚拟网卡", ", all through the tunnel")
	case proxy + tun:
		return conns(total) + L("，都走代理或虚拟网卡", ", all through the proxy or the tunnel")
	}
	names := [...]string{L("走代理", "proxy"), L("走虚拟网卡", "tunnel"), L("连本机", "this computer"), L("连局域网", "local network"), L("直连", "direct")}
	var parts []string
	for i, v := range r {
		if v > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", names[i], v))
		}
	}
	return conns(total) + L("：", ": ") + strings.Join(parts, L("、", ", "))
}

// routesLine says where connections go; in red when some go direct.
func routesLine(c *ui.Context, r netguard.Routes) {
	t := c.Theme()
	col := t.TextMuted
	if r[netguard.Direct] > 0 {
		col = t.Danger
	}
	ui.Text(c, routesText(r)).FontSize(12).TextColor(col)
}
