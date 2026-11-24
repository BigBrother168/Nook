package main

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"nook/core/history"
)

// The command palette (Ctrl+Shift+P): Nook's commands, its terminals, its
// projects and the closed conversations, found by typing.

// paletteItem is a row of the palette.
type paletteItem struct {
	label, hint string
	icon        *ui.SVG
	run         func()
}

// paletteItems returns everything the palette finds, commands first.
func (a *app) paletteItems() []paletteItem {
	lock := L("锁上联网（AI 不能启动）", "Lock the network (no AI can start)")
	if a.st.Locked {
		lock = L("解开联网锁", "Unlock the network")
	}
	items := []paletteItem{
		{L("新建终端", "New terminal"), "Ctrl+Shift+T", iconPlus, func() { a.openTab(a.currentProject(), a.defaultShell()) }},
		{L("向右分屏", "Split right"), "Ctrl+Shift+D", iconSplitRight, func() { a.split(false) }},
		{L("向下分屏", "Split down"), "Ctrl+Shift+E", iconSplitDown, func() { a.split(true) }},
		{L("结束当前终端", "End this terminal"), "Ctrl+Shift+W", iconStop, func() { a.endSession(a.focused()) }},
		{L("添加项目文件夹", "Add a project folder"), "", iconFolderPlus, func() { go a.pickProject() }},
		{L("显示 / 隐藏左侧栏", "Show / hide the sidebar"), "Ctrl+Shift+S", iconPanelLeft, a.toggleSidebar},
		{L("在终端里找", "Find in the terminal"), "Ctrl+Shift+F", iconTerminal, a.openFind},
		{L("设置", "Settings"), "Ctrl+Shift+,", iconSettings, a.openSettings},
		{L("终端换个口味", "Terminals: another flavor"), L("这套配色搭的下一种", "the next that goes with the look"), iconPalette, a.nextTermFlavor},
		{L("改动：AI 改了哪些文件，保留或撤销", "Changes: what the AI changed, to keep or undo"), "Ctrl+Shift+B", iconUndo, func() { a.side, a.sideTab = true, sideChanges }},
		{L("文件：浏览项目文件", "Files: browse the project"), "", iconFile, func() { a.side, a.sideTab = true, sideFiles }},
		{L("预览：打开 localhost 网页", "Previews: open localhost pages"), "", iconGlobe, func() { a.side, a.sideTab = true, sidePreviews }},
		{L("联网：真实 IP 有没有露", "Network: whether your real IP shows"), "", iconGlobe, func() { a.side, a.sideTab = true, sideNet }},
		{lock, L("联网锁", "Network lock"), iconGlobe, func() { a.toggleLock(); a.syncTray() }},
	}
	if a.tray != nil {
		items = append(items, paletteItem{L("收进托盘", "Hide in the tray"), L("终端和 AI 继续运行", "Terminals and AI tools go on"), nil, a.toTray})
	}
	items = append(items, paletteItem{L("退出 Nook", "Quit Nook"), L("结束所有终端和 AI", "Ends every terminal and AI tool"), nil, a.quit})
	for _, sh := range a.shells {
		items = append(items, paletteItem{L("新建终端：", "New terminal: ") + sh.Name, "", iconTerminal, func() { a.openTab(a.currentProject(), sh) }})
	}
	for _, s := range a.sessions() {
		hint := a.projectLabel(s)
		if st := statusText(s.status); st != "" {
			hint += " · " + st
		}
		items = append(items, paletteItem{L("终端：", "Terminal: ") + a.termLabel(s), hint, iconTerminal, func() { a.show(s) }})
	}
	for _, p := range a.st.Projects {
		items = append(items, paletteItem{L("项目：", "Project: ") + p.Name, p.Path, iconFolder, func() { a.goProject(p.Path) }})
	}
	for _, conv := range a.st.Conversations {
		if a.holder(conv.Ref) != nil {
			continue
		}
		key := conv.Key()
		items = append(items, paletteItem{L("对话：", "Conversation: ") + a.convTitle(conv), history.Names(conv.Tool) + L(" · 已关闭 · ", " · closed · ") + conv.Dir, iconHistory, func() { a.openConv(key, a.sideGroup(conv.Project)) }})
	}
	return items
}

// matches reports whether every word of query is in the item, in any case.
func (it paletteItem) matches(query string) bool {
	text := strings.ToLower(it.label + " " + it.hint)
	for _, w := range strings.Fields(strings.ToLower(query)) {
		if !strings.Contains(text, w) {
			return false
		}
	}
	return true
}

// openPalette shows the palette, empty.
func (a *app) openPalette() {
	a.paletteOpen, a.paletteQuery, a.paletteSel = true, "", 0
}

// palette builds the palette while it is open.
func (a *app) palette(c *ui.Context) {
	if !a.paletteOpen {
		return
	}
	t := c.Theme()
	var found []paletteItem
	for _, it := range a.paletteItems() {
		if it.matches(a.paletteQuery) {
			found = append(found, it)
		}
	}
	if a.paletteQuery != a.paletteLast {
		a.paletteLast, a.paletteSel = a.paletteQuery, 0
	}
	a.paletteSel = min(max(a.paletteSel, 0), max(len(found)-1, 0))
	run := func(it paletteItem) {
		a.paletteOpen = false
		it.run()
	}
	ui.Modal(c, &a.paletteOpen, func() {
		ui.Column(c).Width(560).MaxWidth(560).Gap(8).Children(func() {
			ui.TextInput(c, &a.paletteQuery).Placeholder(L("输入命令、终端、项目或对话…", "Type a command, terminal, project or conversation…")).AutoFocus().FillWidth().
				OnTextKey(func(k ui.TextKey) bool {
					switch k.Key {
					case ui.KeyDown:
						a.paletteSel++
					case ui.KeyUp:
						a.paletteSel--
					case ui.KeyEnter:
						if a.paletteSel < len(found) {
							run(found[a.paletteSel])
						}
					default:
						return false
					}
					return true
				})
			ui.Scroll(c).FillWidth().MaxHeight(380).Children(func() {
				ui.Column(c).FillWidth().Gap(1).Children(func() {
					if len(found) == 0 {
						ui.Text(c, L("没有找到", "Not found")).FontSize(13).TextColor(t.TextMuted).Padding(8)
					}
					for i, it := range found {
						c.Key(i)
						row := ui.Row(c).FillWidth().Height(32).PaddingX(10).Gap(8).AlignItems(ui.Center).Radius(6).Cursor(ui.CursorPointer)
						if i == a.paletteSel {
							row.Background(t.Selection)
						} else if row.Hovered() {
							row.Background(t.SurfaceHover)
						}
						row.Children(func() {
							if it.icon != nil {
								ui.Icon(c, it.icon).FontSize(14).TextColor(t.TextMuted)
							} else {
								ui.Box(c).Width(14)
							}
							ui.Text(c, it.label).FontSize(13).SingleLine().Shrink(1)
							ui.Spacer(c)
							if it.hint != "" {
								ui.Text(c, it.hint).FontSize(12).TextColor(t.TextMuted).SingleLine().Shrink(1)
							}
						})
						if row.Clicked() {
							run(it)
						}
					}
				})
			})
		})
	})
}
