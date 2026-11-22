package main

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

func (a *app) view(c *ui.Context) {
	a.runLater()
	if th := a.windowTheme(c); th != nil {
		c.SetTheme(th)
	}
	a.shortcuts(c)
	t := c.Theme()
	root := ui.Column(c).Fill().Background(t.Background)
	root.Children(func() {
		a.windowBackdrop(c, root)
		a.topBar(c)
		body := ui.Row(c).FillWidth().Grow(1)
		width := body.Bounds().W // the last frame's
		body.Children(func() { a.body(c, width) })
	})
	a.palette(c)
	a.confirmDialog(c)
	a.themePicker(c)
	a.welcome(c)
}

// shortcuts are the window's keys. Control and a letter alone belong to
// the terminal (Ctrl+C, Ctrl+D…), so Nook's take Shift too, as Windows
// Terminal's do.
func (a *app) shortcuts(c *ui.Context) {
	cs := ui.Ctrl | ui.Shift
	c.OnWindowShortcut(cs, ui.KeyT, func() { a.openTab(a.currentProject(), a.defaultShell()) })
	c.OnWindowShortcut(cs, ui.KeyW, func() {
		if s := a.focused(); s != nil {
			a.closeSession(s)
		}
	})
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

// currentProject is the project of the focused terminal, else the last one.
func (a *app) currentProject() string {
	if s := a.focused(); s != nil {
		return s.proj
	}
	return a.st.LastProject
}

// statusDot shows a session's status as a colored dot, or icon when it has
// none.
func statusDot(c *ui.Context, status string, icon *ui.SVG) {
	t := c.Theme()
	if col, ok := statusColor(t, status); ok {
		ui.Box(c).Width(8).Height(8).Radius(4).Background(col).Margin(0, 3)
		return
	}
	if icon != nil {
		ui.Icon(c, icon).FontSize(14).TextColor(t.TextMuted)
	}
}

// tabBar shows the tabs, the new-tab button with its menu of shells, and
// the split buttons.
func (a *app) tabBar(c *ui.Context) {
	t := c.Theme()
	var closing *session
	ui.Row(c).FillWidth().Height(38).PaddingX(6).Gap(4).AlignItems(ui.Center).Background(a.panelColor(t, "tabbar")).Children(func() {
		ui.Row(c).Shrink(1).Gap(2).AlignItems(ui.Center).Clip().Children(func() {
			for i, tb := range a.tabs {
				c.Key(tb.id)
				s := tb.focus
				if s == nil {
					continue
				}
				sel := i == a.cur
				r := ui.Row(c).Height(30).PaddingStart(10).PaddingEnd(4).Gap(6).AlignItems(ui.Center).Radius(6).MaxWidth(240).Shrink(1).Cursor(ui.CursorPointer)
				switch {
				case sel:
					r.Background(t.Background)
				case r.Hovered():
					r.Background(t.SurfaceHover)
				}
				r.Drag(tb)
				if r.Dragging() {
					r.Opacity(0.4)
				}
				a.dragTarget(c, r, tb, nil)
				label := a.termLabel(s)
				if n := len(tb.root.leaves()); n > 1 {
					label = label + " +" + itoa(n-1)
				}
				// The whole name, which a narrow tab cuts.
				tip := label + " · " + s.dir
				if st := statusText(s.status); st != "" {
					tip = st + " · " + tip
				}
				r.Tooltip(tip)
				r.Children(func() {
					statusDot(c, s.status, nil)
					txt := ui.Text(c, label).FontSize(12).SingleLine().Shrink(1)
					if !sel {
						txt.TextColor(t.TextMuted)
					}
					if iconButton(c, iconX, L("关闭（Ctrl+Shift+W）", "Close (Ctrl+Shift+W)")).Clicked() {
						closing = s
					}
				})
				if r.Clicked() && closing == nil {
					a.cur = i
					a.pendingFocus = tb.focus
				}
			}
		})
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
		// Splitting takes a terminal.
		if a.current() != nil {
			if iconButton(c, iconSplitRight, L("向右分屏（Ctrl+Shift+D）", "Split right (Ctrl+Shift+D)")).Clicked() {
				a.split(false)
			}
			if iconButton(c, iconSplitDown, L("向下分屏（Ctrl+Shift+E）", "Split down (Ctrl+Shift+E)")).Clicked() {
				a.split(true)
			}
		}
		if iconButton(c, iconPanelRight, L("改动、文件和预览（Ctrl+Shift+B）", "Changes, files and previews (Ctrl+Shift+B)")).Clicked() {
			a.toggleSide(a.sideTab)
		}
	})
	if closing != nil {
		a.closeSession(closing)
	}
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
		box := ui.Column(c).Fill()
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

// empty is shown when no terminal is open.
func (a *app) empty(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).FillWidth().Center().Gap(12).Children(func() {
		ui.Text(c, L("没有打开的终端", "No terminal open")).FontSize(16).Bold()
		ui.Text(c, L("Ctrl+Shift+T 新建终端，或在左边添加项目文件夹", "Ctrl+Shift+T opens a terminal; or add a project folder on the left")).FontSize(13).TextColor(t.TextMuted)
		if ui.PrimaryButton(c, L("新建终端", "New terminal")).Clicked() {
			a.openTab(a.st.LastProject, a.defaultShell())
		}
	})
}

// iconButton is a small borderless button showing an icon.
func iconButton(c *ui.Context, icon *ui.SVG, tip string) ui.Element {
	b := ui.ButtonBase(c).Label(tip).Padding(4).Radius(5).Tooltip(tip)
	b.Children(func() { ui.Icon(c, icon).FontSize(14) })
	return b
}

// goProject goes to a terminal of the project at path, or opens one.
func (a *app) goProject(path string) {
	for _, s := range a.sessions() {
		if s.proj == path {
			a.show(s)
			return
		}
	}
	a.openTab(path, a.defaultShell())
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
