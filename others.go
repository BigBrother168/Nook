package main

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"nook/core/netguard"
)

// AI tools Nook did not start, told by the program they run in and by
// what each of their processes does, as the components a desktop app
// starts as it opens, which go online with no conversation open.

// otherApp is a program running AI tools, and those tools.
type otherApp struct {
	name  string
	short string // what the user should know of it, in a line
	note  string // and at length
	tools []otherAI
}

// appOf names the program o runs in, with a word about it.
func appOf(o otherAI) (name, short, note string) {
	path := strings.ToLower(o.parentPath)
	switch {
	case strings.Contains(path, `\openai.codex_`) || strings.EqualFold(o.parent, "ChatGPT.exe") && strings.Contains(strings.ToLower(o.name), "codex"):
		return L("ChatGPT 桌面版", "ChatGPT desktop"),
			L("一打开就自己联网，跟有没有对话无关。", "Online as soon as it opens, conversation or not."),
			L("这些是 ChatGPT 桌面版自带的 Codex 组件：桌面版一打开就会启动它们并一直联网（登录、同步、等云端派任务），跟你有没有开对话无关。要它们停下，得在托盘里彻底退出 ChatGPT。", "These are the Codex components ChatGPT desktop starts as it opens; they stay online (signing in, syncing, waiting for tasks from the cloud) whether or not a conversation is open. To stop them, quit ChatGPT from the tray.")
	case strings.Contains(path, `\anthropicclaude`) || strings.Contains(path, `\anthropic\claude`) || strings.EqualFold(o.parent, "Claude.exe"):
		return L("Claude 桌面版", "Claude desktop"), "",
			L("Claude 桌面版自己启动的 Claude Code，跟着桌面版联网。", "Claude Code started by Claude desktop, online with it.")
	}
	return programName(o.parent), "", ""
}

// roleOf says what a process of an AI tool does, from how it was
// started: in a word or two, and at length.
func roleOf(o otherAI) (short, long string) {
	cmd := " " + strings.ToLower(o.cmd) + " "
	switch {
	case strings.Contains(cmd, " app-server "):
		return L("后台服务", "Background service"), L("桌面版的聊天、登录、插件都靠它", "The app's chats, sign-in and plugins run on it")
	case strings.Contains(cmd, " exec-server ") && strings.Contains(cmd, "--remote"):
		return L("云端通道", "Cloud channel"), L("一直连着 OpenAI 云端，等云端的任务派到这台电脑上执行", "Kept open to OpenAI, for tasks the cloud sends to run on this computer")
	case strings.Contains(cmd, " exec-server "):
		return L("执行服务", "Execution service"), L("替桌面版在这台电脑上跑命令", "Runs commands here for the app")
	case strings.Contains(cmd, " mcp-server ") || strings.Contains(cmd, " mcp "):
		return L("MCP 服务", "MCP server"), ""
	case strings.Contains(cmd, " exec "):
		return L("一次性任务", "One-off task"), "codex exec"
	}
	return "", ""
}

// groupOthers groups AI tools by the program they run in; the same tool
// doing the same thing twice shows once, its connections added.
func groupOthers(others []otherAI) []otherApp {
	var apps []otherApp
	at := map[string]int{}
	for _, o := range others {
		name, short, note := appOf(o)
		i, ok := at[name]
		if !ok {
			i = len(apps)
			at[name] = i
			apps = append(apps, otherApp{name: name, short: short, note: note})
		}
		g := &apps[i]
		merged := false
		for j := range g.tools {
			r1, _ := roleOf(g.tools[j])
			r2, _ := roleOf(o)
			if g.tools[j].name == o.name && r1 == r2 {
				for k := range o.routes {
					g.tools[j].routes[k] += o.routes[k]
				}
				merged = true
			}
		}
		if !merged {
			g.tools = append(g.tools, o)
		}
	}
	return apps
}

// otherGroup shows a program running AI tools: one line for each tool,
// what it does and how it connects, more on hover.
func (a *app) otherGroup(c *ui.Context, g otherApp) {
	t := c.Theme()
	c.Key("app:" + g.name)
	card := ui.Column(c).FillWidth().Margin(4, 0).Padding(8, 10).Gap(4).Radius(8).Border(1, t.Border)
	if g.note != "" {
		card.Tooltip(g.note)
	}
	card.Children(func() {
		ui.Text(c, g.name).FontSize(13).Bold().Wrap()
		for _, o := range g.tools {
			c.Key(o.pid)
			short, long := roleOf(o)
			name := strings.TrimSuffix(o.name, ".exe")
			if short != "" {
				name += " · " + short
			}
			r := ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center)
			if long != "" {
				r.Tooltip(long)
			}
			r.Children(func() {
				ui.Text(c, name).FontSize(12).Grow(1).Shrink(1).SingleLine()
				if strings.Contains(o.cmd, "--analytics-default-enabled") {
					ui.Text(c, L("统计", "analytics")).FontSize(10).TextColor(t.Warning).Padding(0, 4).Radius(4).Background(t.Warning.Alpha(0.14)).Tooltip(L("启动参数里开着使用统计（--analytics-default-enabled）", "Started with usage analytics on (--analytics-default-enabled)"))
				}
				col := t.TextMuted
				if o.routes[netguard.Direct] > 0 {
					col = t.Danger
				}
				ui.Text(c, shortRoutes(o.routes)).FontSize(11).TextColor(col).Shrink(0)
			})
		}
		if g.short != "" {
			ui.Text(c, g.short).FontSize(11).TextColor(t.TextMuted).Wrap()
		}
	})
}

// shortRoutes says how many connections, and how they go, in a few
// characters: 2 条 · 代理.
func shortRoutes(r netguard.Routes) string {
	total := r.Total()
	if total == 0 {
		return L("没连接", "idle")
	}
	how := L("代理", "proxy")
	switch {
	case r[netguard.Direct] > 0:
		how = L("有直连", "direct!")
	case r[netguard.ViaTUN] == total:
		how = L("虚拟网卡", "tunnel")
	case r[netguard.ViaProxy]+r[netguard.ViaTUN] < total:
		how = L("本机", "local")
	}
	return itoa(total) + L(" 条 · ", " · ") + how
}
