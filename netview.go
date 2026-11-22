package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"nook/core/netguard"
)

var (
	iconLock   = lucide(`<rect width="18" height="11" x="3" y="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/>`)
	iconUnlock = lucide(`<rect width="18" height="11" x="3" y="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 9.9-1"/>`)
)

func requireLabel() string { return L("没有代理时，不让 AI 启动", "No proxy, no AI") }

// topBar shows the project, and the proxy, the AI tools running and the
// network lock, each a click away from its details or its switch.
func (a *app) topBar(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).FillWidth().Height(36).PaddingX(12).Gap(8).AlignItems(ui.Center).Background(a.panelColor(t, "titlebar")).Children(func() {
		tip := L("隐藏侧边栏（Ctrl+Shift+S）", "Hide the sidebar (Ctrl+Shift+S)")
		if a.st.SidebarHidden {
			tip = L("显示侧边栏（Ctrl+Shift+S）", "Show the sidebar (Ctrl+Shift+S)")
		}
		if iconButton(c, iconPanelLeft, tip).Clicked() {
			a.toggleSidebar()
		}
		ui.Text(c, "Nook").FontSize(13).Bold()
		if s := a.focused(); s != nil && s.proj != "" {
			ui.Text(c, "— "+a.projectName(s.proj)).FontSize(13).TextColor(t.TextMuted).SingleLine()
		}
		ui.Spacer(c)

		col, text := a.proxyStatus(t)
		if chip(c, col, nil, text, L("代理状态，点开看详情", "The proxy; click for details")).Clicked() {
			a.netOpen = !a.netOpen
		}

		n, direct := a.net.running()
		aiTip := L("AI 的连接都走代理，真实 IP 就不会露出去。点开看每个 AI。", "AI tools connecting only through the proxy keep your real IP hidden. Click for each one.")
		switch elsewhere := a.net.elsewhereDirect(); {
		case direct > 0:
			col, text = t.Danger, fmt.Sprintf(L("AI 有直连 · %d 条", "AI going direct · %d"), direct)
			aiTip = L("有 AI 的连接没走代理，会露出真实 IP。点开看是哪个。", "An AI tool connects around the proxy, showing your real IP. Click to see which.")
		case elsewhere > 0:
			col, text = t.Danger, fmt.Sprintf(L("其他 AI 有直连 · %d 条", "AI elsewhere going direct · %d"), elsewhere)
			aiTip = L("Nook 外面有 AI 的连接没走代理，会露出真实 IP。点开看是哪个。", "An AI tool outside Nook connects around the proxy, showing your real IP. Click to see which.")
		case n > 0 && a.net.online() == 0:
			col, text = t.TextMuted, fmt.Sprintf(L("AI 还没联网 · %d", "AI not online yet · %d"), n)
			aiTip = L("AI 在运行，还没有连接。连上后这里会说走没走代理。", "AI tools run, with no connections yet. Once they connect, this says whether through the proxy.")
		case n > 0:
			col, text = t.Success, fmt.Sprintf(L("AI 都走代理 · %d", "AI via proxy · %d"), n)
		case len(a.net.others) > 0 && a.net.online() > 0:
			col, text = t.Success, L("AI 都走代理", "AI via proxy")
		default:
			col, text = t.TextMuted, L("没有 AI 在运行", "No AI running")
		}
		if chip(c, col, nil, text, aiTip).Clicked() {
			a.netOpen = !a.netOpen
		}

		lock, lockText, lockCol := iconUnlock, L("联网未锁", "Network unlocked"), t.TextMuted
		if a.st.Locked {
			lock, lockText, lockCol = iconLock, L("联网已锁", "Network locked"), t.Danger
		}
		if chip(c, lockCol, lock, lockText, L("锁上后，AI 工具一律不能启动", "Locked, no AI tool can start")).Clicked() {
			a.toggleLock()
		}
	})
}

// proxyStatus is the proxy chip's color and words.
func (a *app) proxyStatus(t *ui.Theme) (ui.Color, string) {
	p := a.net.proxy
	switch {
	case p.Works():
		return t.Success, fmt.Sprintf(L("代理已开 · %d", "Proxy on · %d"), p.Port)
	case len(netguard.Carrying(a.net.tunnels)) > 0:
		return t.Success, L("虚拟网卡已开", "Tunnel on")
	case a.st.Requires():
		return t.Danger, L("代理没开", "Proxy off")
	}
	return t.TextMuted, L("没用代理", "No proxy")
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
		a.netOpen = true
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

// netPanel shows the network: the proxy and the tunnel, the switch that
// keeps AI tools from starting without them, Nook's own connections, and
// every AI tool's.
func (a *app) netPanel(c *ui.Context, width float32) {
	t := c.Theme()
	muted := func(s string) { ui.Text(c, s).FontSize(12).TextColor(t.TextMuted).Wrap() }
	heading := func(s string) { ui.Text(c, s).FontSize(12).Bold().TextColor(t.TextMuted).Margin(12, 0, 2, 0) }
	line := func(s string, col ui.Color) { ui.Text(c, s).FontSize(13).TextColor(col).Wrap() }
	ui.Column(c).Width(width).FillHeight().Shrink(0).Background(t.Surface).Children(func() {
		ui.Row(c).FillWidth().Height(40).PaddingX(14).AlignItems(ui.Center).Children(func() {
			ui.Text(c, L("联网", "Network")).FontSize(14).Bold()
			ui.Spacer(c)
			if iconButton(c, iconX, L("关闭", "Close")).Clicked() {
				a.netOpen = false
			}
		})
		ui.Scroll(c).Grow(1).FillWidth().Children(func() {
			ui.Column(c).FillWidth().PaddingX(14).PaddingY(4).Gap(4).Children(func() {
				a.ipVerdict(c)
				// status is a line of the panel: a label, and how it is.
				status := func(label, value string, col ui.Color, tip string) {
					r := ui.Row(c).FillWidth().PaddingY(5).Gap(10).AlignItems(ui.Center)
					if tip != "" {
						r.Tooltip(tip)
					}
					r.Children(func() {
						ui.Text(c, label).FontSize(12).TextColor(t.TextMuted).Width(76).Shrink(0)
						ui.Text(c, value).FontSize(13).TextColor(col).Grow(1).Shrink(1).Wrap()
					})
				}
				p := a.net.proxy
				tip, _ := sourceText(p.Source)
				switch {
				case p.Port == 0:
					status(L("代理", "Proxy"), L("没有找到", "Not found"), t.TextMuted, tip)
				case p.Works():
					status(L("代理", "Proxy"), L("已开 · ", "On · ")+p.Addr(), t.Success, tip)
				default:
					status(L("代理", "Proxy"), L("没开 · ", "Off · ")+p.Addr(), t.Danger, tip)
				}
				tun := tunTip(len(netguard.Carrying(a.net.tunnels)) > 0)
				if len(a.net.tunnels) == 0 {
					status(L("虚拟网卡", "Tunnel"), L("没开", "Off"), t.TextMuted, tun)
				}
				for _, tu := range a.net.tunnels {
					if tu.Internet {
						status(L("虚拟网卡", "Tunnel"), L("已开 · ", "On · ")+tu.Name, t.Success, tun)
					} else {
						status(L("虚拟网卡", "Tunnel"), L("开着，上网不走它", "On, unused")+" · "+tu.Name, t.TextMuted, tun)
					}
				}
				// Without a tunnel carrying the internet, the advice shows.
				if len(netguard.Carrying(a.net.tunnels)) == 0 {
					muted(tun)
				}
				if a.net.self == 0 {
					status(L("Nook 自己", "Nook"), L("不联网", "Offline"), t.Success, L("Nook 自己没有任何连接", "Nook itself has no connections"))
				} else {
					status(L("Nook 自己", "Nook"), L("有 ", "")+conns(a.net.self), t.Danger, "")
				}

				ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Margin(10, 0, 0, 0).Tooltip(L("在国内一定要打开，防止用真实 IP 连出去；在国外不用代理就关掉。有代理时，终端和 AI 都会自动带上它。", "In China keep it on, so that nothing connects from your real IP; outside China without a proxy, turn it off. With a proxy, terminals and AI tools take it on their own.")).Children(func() {
					// The words switch it too, as a checkbox's label does.
					if ui.Text(c, requireLabel()).FontSize(13).Grow(1).Cursor(ui.CursorPointer).Clicked() {
						a.requireSel = !a.requireSel
						a.st.SetRequireProxy(a.requireSel)
						a.save()
					}
					if ui.Switch(c, &a.requireSel).Label(requireLabel()).Changed() {
						a.st.SetRequireProxy(a.requireSel)
						a.save()
					}
				})
				muted(L("在国内一定要打开。", "In China, keep it on."))

				heading(L("Nook 里的 AI", "AI tools in Nook"))
				if len(a.net.ai) == 0 {
					line(L("没有在运行", "None running"), t.TextMuted)
				}
				for _, s := range a.sessions() {
					u, ok := a.net.ai[s.id]
					if !ok {
						continue
					}
					c.Key(s.id)
					row := ui.Column(c).FillWidth().Padding(6, 8).Radius(6).Cursor(ui.CursorPointer).Tooltip(L("一个 AI 会同时开好几条连接：跟模型对话、登录、统计、插件各用各的，所以比对话多；都走代理就没事。点一下跳到这个终端。", "One AI tool keeps several connections at once: the model, signing in, statistics, plugins, each its own; all through the proxy is fine. Click to go to this terminal."))
					if row.Hovered() {
						row.Background(t.SurfaceHover)
					}
					row.Children(func() {
						// As the tab and the sidebar call it, and where.
						ui.Text(c, a.termLabel(s)+" · "+a.projectLabel(s)).FontSize(13).SingleLine()
						routesLine(c, u.routes)
					})
					if row.Clicked() {
						a.show(s)
					}
				}
				if n, _ := a.net.running(); n > 0 && a.st.Locked {
					if smallButton(c, L("结束 Nook 里正在运行的 AI", "End the AI tools running in Nook"), false).Clicked() {
						a.endAI()
					}
					muted(L("锁只拦新启动的；已经在运行的，由你决定要不要结束。", "The lock stops new ones only; you decide whether to end those running."))
				}

				ui.Text(c, L("其他地方的 AI", "AI tools elsewhere")).FontSize(12).Bold().TextColor(t.TextMuted).Margin(12, 0, 2, 0).Tooltip(L("不是 Nook 开的，只能看，管不了。", "Not started by Nook: shown, not controlled."))
				if len(a.net.others) == 0 {
					line(L("没有", "None"), t.TextMuted)
				}
				for _, g := range groupOthers(a.net.others) {
					a.otherGroup(c, g)
				}
			})
		})
	})
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
			card.Tooltip(L("Nook 看的是 TCP 连接。在代理软件里打开 TUN 模式，Nook 看不到的流量也会走代理。", "Nook watches TCP connections. With TUN mode on in your proxy app, what Nook cannot see goes through the proxy too."))
		}
	}
	card.Children(func() {
		ui.Text(c, title).FontSize(14).Bold().TextColor(col).Wrap()
		if len(leaks) == 0 {
			if col == t.Success {
				ui.Text(c, L("AI 的连接都走代理，没有直连。", "AI tools connect only through the proxy, none direct.")).FontSize(12).TextColor(t.TextMuted).Wrap()
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
			ui.Text(c, L("Nook 外面的 AI 管不了，可以把它退出。", "AI tools outside Nook are not Nook's to stop; you may quit them.")).FontSize(12).TextColor(t.TextMuted).Wrap()
		}
		if !tunOn {
			ui.Text(c, L("在代理软件里打开 TUN 模式，能把这些也兜进代理。", "TUN mode in your proxy app takes these through the proxy too.")).FontSize(12).TextColor(t.TextMuted).Wrap()
		}
	})
}

// tunTip suggests the tunnel, which takes every program through the
// proxy, those that skip the proxy settings too.
func tunTip(on bool) string {
	if on {
		return L("建议一直开着：所有程序都会走代理，比只开代理更稳。", "Keep it on: every program goes through the proxy, more reliably than with the proxy alone.")
	}
	return L("建议在代理软件里打开 TUN 模式：所有程序都会走代理，比只开代理更稳。", "Turn on TUN mode in your proxy app: every program goes through the proxy, more reliably than with the proxy alone.")
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
