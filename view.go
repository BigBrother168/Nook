package main

import (
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"nook/core/history"
)

func (a *app) view(c *ui.Context) {
	a.runLater()
	a.fixCurrent()
	a.termFocus = false
	if th := a.windowTheme(c); th != nil {
		c.SetTheme(th)
	}
	a.shortcuts(c)
	t := c.Theme()
	root := ui.Column(c).Fill().Background(a.chromeColor(t))
	a.winW, a.winH = root.Bounds().W, root.Bounds().H
	root.Children(func() {
		a.windowBackdrop(c, root)
		a.topBar(c)
		body := ui.Row(c).FillWidth().Grow(1)
		width := body.Bounds().W // the last frame's
		body.Children(func() { a.body(c, width) })
	})
	a.palette(c)
	a.settingsDialog(c)
	a.confirmDialog(c)
	a.closeDialog(c)
	a.themePicker(c)
	a.welcome(c)
}

// shortcuts are the window's keys. Control and a letter alone belong to
// the terminal (Ctrl+C, Ctrl+D…), so Nook's take Shift too, as Windows
// Terminal's do.
func (a *app) shortcuts(c *ui.Context) {
	cs := ui.Ctrl | ui.Shift
	c.OnWindowShortcut(cs, ui.KeyT, func() { a.openTab(a.currentProject(), a.defaultShell()) })
	c.OnWindowShortcut(cs, ui.KeyW, func() { a.endSession(a.focused()) })
	c.OnWindowShortcut(cs, ui.KeyD, func() { a.split(false) })
	c.OnWindowShortcut(cs, ui.KeyE, func() { a.split(true) })
	c.OnWindowShortcut(ui.Ctrl, ui.KeyTab, func() { a.cycle(1) })
	c.OnWindowShortcut(cs, ui.KeyTab, func() { a.cycle(-1) })
	c.OnWindowShortcut(ui.Ctrl, ui.KeyPageDown, func() { a.cycle(1) })
	c.OnWindowShortcut(ui.Ctrl, ui.KeyPageUp, func() { a.cycle(-1) })
	c.OnWindowShortcut(cs, ui.KeyP, a.openPalette)
	c.OnWindowShortcut(cs, ui.KeyB, func() { a.toggleSide(a.sideTab) })
	c.OnWindowShortcut(cs, ui.KeyComma, a.openSettings)
	c.OnWindowShortcut(cs, ui.KeyS, a.toggleSidebar)
	c.OnWindowShortcut(cs, ui.KeyF, a.openFind)
}

// currentProject is the project of the focused terminal, else the one in
// use.
func (a *app) currentProject() string {
	if s := a.focused(); s != nil {
		return s.proj
	}
	return a.group
}

// statusDot shows a terminal's status without words, which its tooltip
// says: turning while the AI works, a yellow dot while it waits for you,
// green when done, until looked at, red for an error; icon (or nothing)
// otherwise.
func statusDot(c *ui.Context, s *session, icon *ui.SVG) {
	t := c.Theme()
	status := s.status
	if status == "done" && !s.unread {
		status = ""
	}
	if status == "working" {
		ui.Box(c).Width(14).Height(14).Center().Shrink(0).Children(func() {
			ui.Spinner(c).FontSize(12).TextColor(t.Accent)
		})
		return
	}
	if col, ok := statusColor(t, status); ok {
		ui.Box(c).Width(14).Height(14).Center().Shrink(0).Children(func() {
			ui.Box(c).Width(8).Height(8).Radius(4).Background(col)
		})
		return
	}
	if icon != nil {
		ui.Box(c).Width(14).Height(14).Center().Shrink(0).Children(func() {
			ui.Icon(c, icon).FontSize(14).TextColor(t.TextMuted)
		})
	}
}

// tabBar shows the tabs, the new-tab button with its menu of shells, and
// the split buttons.
func (a *app) tabBar(c *ui.Context) {
	t := c.Theme()
	var hiding, chosen *tab
	var ending *session
	var back *viewItem
	bar := ui.Row(c).FillWidth().Height(38).PaddingX(6).Gap(4).AlignItems(ui.Center).Background(a.panelColor(t, "tabbar"))
	width := bar.Bounds().W // the last frame's
	bar.Children(func() {
		// As many tabs as fit at a readable width; the others, in a menu.
		shown, cur := a.shownTabs(), a.current()
		visible, more := shown, []*tab(nil)
		if a.middleW > 0 {
			width = a.middleW
		}
		if fit := int((width - 150) / tabMinWidth); width > 0 && len(shown) > max(fit, 1) {
			n := max(fit-1, 1)
			visible = slices.Clone(shown[:n])
			if cur != nil && !slices.Contains(visible, cur) {
				visible[n-1] = cur
			}
			for _, tb := range shown {
				if !slices.Contains(visible, tb) {
					more = append(more, tb)
				}
			}
		}
		ui.Row(c).Shrink(1).Gap(2).AlignItems(ui.Center).Clip().Children(func() {
			for _, tb := range visible {
				c.Key(tb.id)
				s := tb.focus
				if s == nil {
					continue
				}
				sel := tb == cur
				r := ui.Row(c).Height(30).PaddingStart(8).PaddingEnd(4).Gap(6).AlignItems(ui.Center).Radius(6).MinWidth(tabMinWidth).MaxWidth(240).Shrink(1).Cursor(ui.CursorPointer)
				switch {
				case sel:
					r.Background(t.Selection)
				case r.Hovered():
					r.Background(t.SurfaceHover)
				}
				r.Drag(tb)
				if r.Dragging() {
					r.Opacity(0.4)
				}
				a.dragTarget(c, r, tb, nil)
				label := a.tabLabel(tb)
				r.Tooltip(a.termTip(s, label))
				r.Children(func() {
					statusDot(c, s, nil)
					if a.renaming == s {
						a.renameField(c, s)
					} else {
						txt := ui.Text(c, label).FontSize(12).SingleLine().Shrink(1).Grow(1)
						if !sel {
							txt.TextColor(t.TextMuted)
						}
					}
					if iconButton(c, iconX, L("收起（终端继续运行）", "Put away (it keeps running)")).Clicked() {
						hiding = tb
					}
				})
				r.ContextMenu(func(m *ui.Menu) {
					if m.Item(L("重命名", "Rename")).Chosen() {
						a.renaming, a.renameText, a.renameFocused = s, a.termLabel(s), false
					}
					if m.Item(L("收起", "Put away")).Chosen() {
						hiding = tb
					}
					if m.Item(L("结束", "End")).Trailing("Ctrl+Shift+W").Chosen() {
						ending = s
					}
				})
				if r.Clicked() && hiding == nil {
					chosen = tb
				}
			}
		})
		if len(more) > 0 {
			b := ui.ButtonBase(c).Label(L("更多终端", "More terminals")).Padding(5, 8).Radius(6).Tooltip(L("放不下的终端", "Terminals that do not fit"))
			b.Children(func() {
				ui.Text(c, "⋯ "+itoa(len(more))).FontSize(12).TextColor(t.TextMuted)
			})
			b.Menu(func(m *ui.Menu) {
				for _, tb := range more {
					if m.Item(a.tabLabel(tb)).Chosen() {
						chosen = tb
					}
				}
			})
		}
		newTab := ui.ButtonBase(c).Label(L("新建终端", "New terminal")).Padding(6).Radius(6).Tooltip(L("新建终端（Ctrl+Shift+T）", "New terminal (Ctrl+Shift+T)"))
		newTab.Children(func() {
			ui.Row(c).Gap(2).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconPlus).FontSize(15)
				ui.Icon(c, iconChevron).FontSize(11).TextColor(t.TextMuted)
			})
		})
		newTab.Menu(func(m *ui.Menu) {
			def := a.defaultShell()
			for _, sh := range a.shells {
				if m.Item(sh.Name).Trailing(defaultMark(sh.ID == def.ID)).Chosen() {
					a.openTab(a.currentProject(), sh)
				}
			}
			m.Separator()
			m.Submenu(L("默认 shell", "Default shell"), func(m *ui.Menu) {
				for _, sh := range a.shells {
					if m.Item(sh.Name).Radio(sh.ID == def.ID).Chosen() {
						a.setDefaultShell(sh.ID)
					}
				}
			})
		})
		ui.Spacer(c)
		if it := a.putAway(); it != nil {
			back = it
			a.putAwayButton(c, *it, &back)
		}
		// Splitting takes a terminal.
		if a.current() != nil {
			if iconButton(c, iconSplitRight, L("向右分屏（Ctrl+Shift+D）", "Split right (Ctrl+Shift+D)")).Clicked() {
				a.split(false)
			}
			if iconButton(c, iconSplitDown, L("向下分屏（Ctrl+Shift+E）", "Split down (Ctrl+Shift+E)")).Clicked() {
				a.split(true)
			}
		}
		if iconButton(c, iconPanelRight, L("右侧面板（Ctrl+Shift+B）", "Right panel (Ctrl+Shift+B)")).Clicked() {
			a.toggleSide(a.sideTab)
		}
	})
	switch {
	case hiding != nil:
		a.hideTab(hiding)
	case ending != nil:
		a.endSession(ending)
	case chosen != nil:
		a.choose(chosen)
	case back != nil:
		a.showItem(*back)
	}
}

// putAwayButton is the tab bar's way back to what was put away beside the
// terminals: its icon and name, and how many more wait with it. Clicked,
// *back stays set; else it is cleared.
func (a *app) putAwayButton(c *ui.Context, it viewItem, back **viewItem) {
	t := c.Theme()
	icon, col, name := a.itemLook(t, it)
	b := ui.ButtonBase(c).Label(name).Height(28).PaddingX(9).Radius(7).Border(1, t.Border).MaxWidth(220).Shrink(1).
		Tooltip(L("收起了，点一下回来", "Put away: click to bring it back"))
	if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	b.Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icon).FontSize(13).TextColor(col)
			ui.Text(c, name).FontSize(12).SingleLine().Shrink(1)
			if n := len(a.items) - 1; n > 0 {
				ui.Text(c, "+"+itoa(n)).FontSize(11).TextColor(t.TextMuted)
			}
		})
	})
	if !b.Clicked() {
		*back = nil
	}
}

// renameField is the name of terminal s being typed, in its tab: Enter
// or a click elsewhere keeps it, Esc leaves it as it was; emptied, the
// terminal takes its own name again.
func (a *app) renameField(c *ui.Context, s *session) {
	in := ui.TextInput(c, &a.renameText).AutoFocus().Width(150).Label(L("终端的名字", "The terminal's name"))
	a.termFocus = true // Esc is the field's
	done := func(keep bool) {
		if keep {
			s.name = strings.TrimSpace(a.renameText)
		}
		a.renaming = nil
		a.pendingFocus = s
	}
	focused := in.Focused()
	switch {
	case in.Submitted():
		done(true)
	case a.renameFocused && !focused:
		done(true)
	}
	a.renameFocused = a.renameFocused || focused
	c.OnWindowShortcut(0, ui.KeyEscape, func() { done(false) })
}

// tabMinWidth is the narrowest a tab gets: its dot and a few characters.
const tabMinWidth = 104

// tabLabel is what a tab says: its terminal's name, and how many more its
// split holds.
func (a *app) tabLabel(tb *tab) string {
	label := a.termLabel(tb.focus)
	if n := len(tb.root.leaves()); n > 1 {
		label += " +" + itoa(n-1)
	}
	return label
}

// termTip is a terminal's tooltip: its whole name, what it is doing, and
// where.
func (a *app) termTip(s *session, label string) string {
	tip := label + " · " + s.dir
	st := s.status
	if st == "done" && !s.unread {
		st = ""
	}
	if w := statusText(st); w != "" {
		tip = w + " · " + tip
	}
	return tip
}

func defaultMark(on bool) string {
	if on {
		return L("默认", "Default")
	}
	return ""
}

// pane builds a tab's layout: a terminal, or a split of two panes.
func (a *app) pane(c *ui.Context, tb *tab, p *pane) {
	t := c.Theme()
	c.Key(p.id)
	if s := p.s; s != nil {
		split := tb.root.s == nil
		// A margin of the terminal's own color keeps its text off the
		// card's edge.
		box := ui.Column(c).Fill().Padding(6, 4, 2, 8).Background(a.termBg(t))
		if s.transparent {
			bg := t.Background
			if d := a.currentDesign(); d != nil {
				bg = ui.Hex(d.TermBg).Alpha(panelAlpha(d))
			}
			box.Background(bg)
		}
		if split {
			col := t.Border
			if tb.focus == s {
				col = t.Accent
			}
			box.Border(1, col)
		}
		box.Children(func() {
			if s.find.open {
				a.findBar(c, s)
			}
			v := terminal.View(c, s.term).Bind(&s.handle).Fill()
			if a.pendingFocus == s {
				s.handle.Focus(c)
				a.pendingFocus = nil
			}
			if s.handle.FocusWithin(c) {
				tb.focus = s
				s.unread = false
				a.termFocus = true
			}
			if s.find.open {
				a.termFocus = true // its find bar takes Esc
			}
			_ = v
		})
		return
	}
	first := func() { a.pane(c, tb, p.a) }
	second := func() { a.pane(c, tb, p.b) }
	if p.vertical {
		ui.SplitVerticalPercent(c, &p.ratio, first, second).SplitReset(50).Fill()
	} else {
		ui.SplitPercent(c, &p.ratio, first, second).SplitReset(50).Fill()
	}
}

// empty is the middle when the project in use has no terminal showing:
// the project, a button to open one, and its last conversations, to read
// or go on with.
func (a *app) empty(c *ui.Context) {
	t := c.Theme()
	proj := a.group
	ui.Column(c).Grow(1).FillWidth().Center().Children(func() {
		ui.Column(c).Width(360).Gap(18).Children(func() {
			ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
				if proj != "" {
					avatar(c, a.projectName(proj), 40)
				} else {
					ui.Box(c).Width(40).Height(40).Radius(10).Background(t.SurfaceHover).Center().Children(func() {
						ui.Icon(c, iconTerminal).FontSize(20).TextColor(t.TextMuted)
					})
				}
				ui.Column(c).Grow(1).Shrink(1).Gap(2).Children(func() {
					name, sub := L("没有打开的终端", "No terminal open"), L("新建一个，或者在左边添加项目", "Open one, or add a project on the left")
					if proj != "" {
						name, sub = a.projectName(proj), proj
					}
					ui.Text(c, name).FontSize(17).Bold().SingleLine()
					ui.Text(c, sub).FontSize(12).TextColor(t.TextMuted).SingleLine()
				})
			})
			if ui.PrimaryButton(c, L("新建终端", "New terminal")).Tooltip("Ctrl+Shift+T").FillWidth().Clicked() {
				a.openTab(proj, a.defaultShell())
			}
			convs := a.closedConvs(proj, proj == "")
			if len(convs) == 0 {
				return
			}
			ui.Column(c).FillWidth().Gap(2).Children(func() {
				ui.Text(c, L("最近的对话", "Recent conversations")).FontSize(12).Bold().TextColor(t.TextMuted).Padding(0, 4, 4, 4)
				for _, conv := range convs[:min(len(convs), 4)] {
					c.Key("recent:" + conv.Key())
					row := ui.Row(c).FillWidth().Height(34).PaddingX(10).Gap(8).AlignItems(ui.Center).Radius(7).Cursor(ui.CursorPointer)
					if row.Hovered() {
						row.Background(t.SurfaceHover)
					}
					row.Children(func() {
						ui.Icon(c, iconHistory).FontSize(13).TextColor(t.TextMuted)
						ui.Text(c, a.convTitle(conv)).FontSize(13).SingleLine().Shrink(1).Grow(1)
						ui.Text(c, history.Names(conv.Tool)).FontSize(11).TextColor(t.TextMuted).SingleLine()
					})
					if row.Clicked() {
						a.openConv(conv.Key(), proj)
					}
				}
			})
		})
	})
}

// openConv shows conversation key, of project group, beside the
// terminals, the project in use.
func (a *app) openConv(key, group string) {
	a.leavePages()
	a.setGroup(group)
	a.viewing, a.picked = key, key
}

// iconButton is a small borderless button showing an icon.
func iconButton(c *ui.Context, icon *ui.SVG, tip string) ui.Element {
	b := ui.ButtonBase(c).Label(tip).Padding(4).Radius(5).Tooltip(tip)
	b.Children(func() { ui.Icon(c, icon).FontSize(14) })
	return b
}

// pickProject asks for a folder and adds it as a project.
func (a *app) pickProject() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: L("选择项目文件夹", "Choose a project folder"), Directory: true})
	if err != nil || len(paths) == 0 {
		return
	}
	a.win.Update(func() { a.addProject(paths[0]) })
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}
