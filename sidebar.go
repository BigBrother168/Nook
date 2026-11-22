package main

import (
	"hash/fnv"
	"strings"
	"unicode"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"nook/core/config"
	"nook/core/history"
)

// The sidebar: the projects, each with its terminals and the
// conversations of those closed, then the terminals of no project.

// avatarColors are the colors of the projects' letters.
var avatarColors = []string{"#c2410c", "#0f766e", "#4338ca", "#a16207", "#be185d", "#15803d", "#0369a1", "#7e22ce"}

// avatar is a project's first letter on a color of its own.
func avatar(c *ui.Context, name string, size float32) {
	h := fnv.New32a()
	h.Write([]byte(name))
	letter := "?"
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			letter = strings.ToUpper(string(r))
			break
		}
	}
	ui.Box(c).Width(size).Height(size).Radius(size / 4).Background(ui.Hex(avatarColors[h.Sum32()%uint32(len(avatarColors))])).Center().Shrink(0).Children(func() {
		ui.Text(c, letter).FontSize(size * 0.46).Bold().TextColor(ui.Hex("#ffffff"))
	})
}

func (a *app) sidebar(c *ui.Context) {
	t := c.Theme()
	open := a.sessions()
	ui.Column(c).Fill().Background(a.panelColor(t, "sidebar")).Children(func() {
		ui.Row(c).FillWidth().PaddingStart(16).PaddingEnd(8).Height(42).Gap(2).AlignItems(ui.Center).Children(func() {
			ui.Text(c, L("项目", "Projects")).FontSize(12).Bold().TextColor(t.TextMuted)
			if n := len(a.st.Projects); n > 0 {
				ui.Text(c, itoa(n)).FontSize(11).TextColor(t.TextMuted).Padding(0, 6)
			}
			ui.Spacer(c)
			if iconButton(c, iconSettings, L("设置（Ctrl+Shift+,）", "Settings (Ctrl+Shift+,)")).Clicked() {
				a.openSettings()
			}
		})
		ui.Scroll(c).Grow(1).FillWidth().Children(func() {
			ui.Column(c).FillWidth().Gap(1).PaddingX(8).PaddingY(2).Children(func() {
				for _, p := range a.st.Projects {
					c.Key("p:" + p.Path)
					a.projectRow(c, p)
					for _, s := range open {
						if s.proj == p.Path {
							a.sessionRow(c, s)
						}
					}
					a.historyGroup(c, p.Path, a.closedConvs(p.Path, false))
					ui.Box(c).Height(6)
				}
				var loose []*session
				for _, s := range open {
					if s.proj == "" || !a.hasProject(s.proj) {
						loose = append(loose, s)
					}
				}
				looseConvs := a.closedConvs("", true)
				if len(loose) > 0 || len(looseConvs) > 0 {
					ui.Text(c, L("其他终端", "Other terminals")).FontSize(12).Bold().TextColor(t.TextMuted).Padding(10, 8, 4, 8)
					for _, s := range loose {
						a.sessionRow(c, s)
					}
					a.historyGroup(c, "", looseConvs)
				}
			})
		})
		ui.Column(c).FillWidth().Padding(10).Gap(8).Children(func() {
			add := ui.ButtonBase(c).Label(L("添加项目", "Add a project")).FillWidth().Height(34).Radius(8).Border(1, t.Border).Cursor(ui.CursorPointer)
			if add.Hovered() {
				add.Background(t.SurfaceHover)
			}
			add.Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, iconPlus).FontSize(14).TextColor(t.TextMuted)
					ui.Text(c, L("添加项目", "Add a project")).FontSize(12).TextColor(t.TextMuted)
				})
			})
			if add.Clicked() {
				go a.pickProject()
			}
			if len(a.st.Projects) == 0 {
				ui.Text(c, L("项目就是一个文件夹：终端在那里打开，AI 在那里干活。", "A project is a folder: terminals open there, and AI tools work there.")).FontSize(11).TextColor(t.TextMuted)
			}
		})
	})
}

func (a *app) hasProject(path string) bool {
	for _, p := range a.st.Projects {
		if p.Path == path {
			return true
		}
	}
	return false
}

func (a *app) projectRow(c *ui.Context, p config.Project) {
	t := c.Theme()
	cur := a.focused()
	active := cur != nil && cur.proj == p.Path
	row := ui.Row(c).FillWidth().PaddingX(8).PaddingY(6).Gap(10).AlignItems(ui.Center).Radius(8).Cursor(ui.CursorPointer)
	hovered := row.Hovered()
	if active || hovered {
		row.Background(t.SurfaceHover)
	}
	row.Tooltip(p.Path)
	row.Children(func() {
		avatar(c, p.Name, 28)
		ui.Column(c).Grow(1).Shrink(1).Gap(1).Children(func() {
			ui.Text(c, p.Name).FontSize(13).Bold().SingleLine()
			ui.Text(c, p.Path).FontSize(11).TextColor(t.TextMuted).SingleLine()
		})
		if hovered || active {
			if iconButton(c, iconPlus, L("在这里新建终端", "New terminal here")).Clicked() {
				a.openTab(p.Path, a.defaultShell())
			}
		}
	})
	row.ContextMenu(func(m *ui.Menu) {
		m.Submenu(L("新建终端", "New terminal"), func(m *ui.Menu) {
			for _, sh := range a.shells {
				if m.Item(sh.Name).Chosen() {
					a.openTab(p.Path, sh)
				}
			}
		})
		if m.Item(L("在资源管理器中打开", "Open in Explorer")).Chosen() {
			mygo.Shell.OpenPath(p.Path)
		}
		m.Separator()
		if m.Item(L("从列表中移除", "Remove from the list")).Chosen() {
			a.removeProject(p.Path)
		}
	})
	if row.Clicked() {
		a.goProject(p.Path)
	}
}

func (a *app) sessionRow(c *ui.Context, s *session) {
	t := c.Theme()
	c.Key(s.id)
	focused := a.focused() == s
	row := ui.Row(c).FillWidth().Height(30).PaddingStart(18).PaddingEnd(6).Gap(8).AlignItems(ui.Center).Radius(7).Cursor(ui.CursorPointer)
	// Dragged up or down, it takes its tab there.
	tb := a.tabOf(s)
	row.Drag(tb)
	if row.Dragging() {
		row.Opacity(0.4) // where it comes from, as its copy follows the pointer
	}
	hovered := row.Hovered()
	if focused {
		row.Background(t.Selection)
	} else if hovered {
		row.Background(t.SurfaceHover)
	}
	// Within its project in the sidebar: dropped on another project's
	// terminal it would move only in the tab bar.
	a.dragTarget(c, row, tb, func(from *tab) bool { return a.sideGroup(tabProject(from)) == a.sideGroup(s.proj) })
	closing := false
	row.Children(func() {
		statusDot(c, s.status, iconTerminal)
		// The name keeps its room; the status gives way.
		ui.Text(c, a.termLabel(s)).FontSize(12).SingleLine().Shrink(0).MaxWidth(130)
		if st := statusText(s.status); st != "" {
			ui.Text(c, st).FontSize(11).TextColor(t.TextMuted).SingleLine().Shrink(1)
		}
		ui.Spacer(c)
		if hovered && iconButton(c, iconX, L("关闭终端", "Close the terminal")).Clicked() {
			closing = true
		}
	})
	row.ContextMenu(func(m *ui.Menu) {
		if m.Item(L("关闭终端", "Close the terminal")).Chosen() {
			closing = true
		}
	})
	switch {
	case closing:
		a.closeSession(s)
	case row.Clicked():
		a.show(s)
	}
}

// historyGroup is a project's closed conversations, under a header that
// folds them.
func (a *app) historyGroup(c *ui.Context, proj string, convs []config.Conversation) {
	if len(convs) == 0 {
		return
	}
	t := c.Theme()
	if a.histClosed == nil {
		a.histClosed = map[string]bool{}
	}
	folded := a.histClosed[proj]
	c.Key("h:" + proj)
	head := ui.Row(c).FillWidth().Height(26).PaddingStart(18).PaddingEnd(8).Gap(6).AlignItems(ui.Center).Radius(7).Cursor(ui.CursorPointer)
	if head.Hovered() {
		head.Background(t.SurfaceHover)
	}
	head.Children(func() {
		chev := ui.Icon(c, iconChevron).FontSize(12).TextColor(t.TextMuted)
		if folded {
			chev.Rotate(-90)
		}
		ui.Icon(c, iconHistory).FontSize(13).TextColor(t.TextMuted)
		ui.Text(c, L("历史对话", "Past conversations")).FontSize(12).TextColor(t.TextMuted).Grow(1)
		ui.Text(c, itoa(len(convs))).FontSize(11).TextColor(t.TextMuted)
	})
	if head.Clicked() {
		a.histClosed[proj] = !folded
	}
	if folded {
		return
	}
	for _, conv := range convs {
		a.convRow(c, conv, proj)
	}
}

// convRow is a closed conversation in the sidebar, in the history of
// group (convDrag), where it can be dragged up or down.
func (a *app) convRow(c *ui.Context, conv config.Conversation, group string) {
	t := c.Theme()
	key := conv.Key()
	c.Key("c:" + key)
	row := ui.Column(c).FillWidth().PaddingStart(40).PaddingEnd(8).PaddingY(5).Gap(1).Radius(7).Cursor(ui.CursorPointer)
	switch {
	case a.viewing == key:
		row.Background(t.Selection)
	case row.Hovered():
		row.Background(t.SurfaceHover)
	}
	row.Drag(convDrag{conv.Ref, group})
	if row.Dragging() {
		row.Opacity(0.4)
	}
	// Another of its history dropped on it takes its place.
	takes := func(d convDrag) bool { return d.group == group && d.ref != conv.Ref }
	if d, ok := ui.DragOver[convDrag](row); ok && takes(d) {
		row.Background(dropColor(c))
	}
	if d, ok := ui.Drop[convDrag](row); ok && takes(d) {
		a.afterFrame(func() { a.moveConv(d.ref, conv.Ref) })
	}
	row.Tooltip(conv.Dir)
	row.Children(func() {
		ui.Text(c, a.convTitle(conv)).FontSize(12).SingleLine()
		ui.Text(c, conv.Seen.Local().Format("01-02 15:04")+" · "+history.Names(conv.Tool)).FontSize(11).TextColor(t.TextMuted).SingleLine()
	})
	row.ContextMenu(func(m *ui.Menu) {
		if m.Item(L("继续这个对话（会联网）", "Continue this conversation (goes online)")).Chosen() {
			a.resume(conv)
		}
		m.Separator()
		if m.Item(L("从列表中移除", "Remove from the list")).Chosen() {
			a.removeConv(conv.Ref)
		}
	})
	if row.Clicked() {
		a.viewing = key
	}
}
