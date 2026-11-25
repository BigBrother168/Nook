package main

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// The window's columns: the sidebar, the middle and the right panel, each
// edge dragged to size; in the middle, the terminals and, beside them, a
// file, a comparison, a page or a conversation, each kind at the width
// the user left it.

// Widths when nothing was chosen, in DIPs, and the narrowest and widest.
const (
	sidebarDefault, sidebarMin, sidebarMax = 248, 180, 480
	sideDefault, sideMin, sideMax          = 340, 260, 720
)

// viewerDefaults is the terminals' share of the middle, in percent, by
// what shows beside them: a comparison and a page take more room.
var viewerDefaults = map[string]float32{"file": 50, "diff": 40, "web": 45, "conv": 50}

// resizer is the edge of a column: dragging it changes *width, which
// grows as the pointer moves right (sign 1) or left (-1).
func (a *app) resizer(c *ui.Context, key string, width *float32, sign, lo, hi, reset float32) {
	t := c.Theme()
	c.Key("edge:" + key)
	// A line of one DIP, in a band of five the pointer takes.
	bar := ui.Row(c).Width(5).FillHeight().Justify(ui.Center).Cursor(ui.CursorResizeColumn).Tooltip(L("拖动调宽，双击还原", "Drag to resize; double-click to reset"))
	dx, _, dragging := bar.Dragged()
	line, lw := ui.Transparent, float32(1)
	if dragging || bar.Hovered() {
		line, lw = t.Accent.Alpha(0.6), 3
	}
	bar.Children(func() { ui.Box(c).Width(lw).FillHeight().Background(line) })
	if dragging && dx != 0 {
		*width = min(max(*width+sign*dx, lo), hi)
	}
	if _, _, ended := bar.DragEnded(); ended {
		a.save()
	}
	if bar.DoubleClicked() {
		*width = reset
		a.save()
	}
}

// viewerKind is what shows beside the terminals, "" for nothing.
func (a *app) viewerKind() string {
	switch {
	case a.diff != nil && a.viewing == "diff:"+a.diff.ch.Path:
		return "diff"
	case a.file != nil && a.viewing == "file:"+a.file.path:
		return "file"
	case a.currentPage() != nil:
		return "web"
	}
	if _, ok := a.convByKey(a.viewing); ok {
		return "conv"
	}
	return ""
}

// viewer builds what shows beside the terminals.
func (a *app) viewer(c *ui.Context, kind string) {
	a.trackViewer(kind)
	ui.Column(c).Fill().Background(a.cardColor(c.Theme())).Transition(panelIn).Children(func() {
		a.itemTabs(c)
		switch kind {
		case "diff":
			a.diffPage(c)
		case "file":
			a.filePage(c)
		case "web":
			a.browser(c)
		case "conv":
			conv, _ := a.convByKey(a.viewing)
			a.convView(c, conv)
		}
	})
}

// viewerButtons are the buttons every viewer has at its top right: one
// widening it over the terminals and back, one putting it away. Put away,
// it stays open as it was, a button in the tab bar away; closing it is
// the × on its small tab.
func (a *app) viewerButtons(c *ui.Context) {
	icon, tip := iconMaximize, L("放大（终端先让开）", "Widen over the terminals")
	if a.viewerMax {
		icon, tip = iconMinimize, L("还原，和终端并排", "Back beside the terminals")
	}
	if a.current() != nil && iconButton(c, icon, tip).Clicked() {
		a.viewerMax = !a.viewerMax
	}
	hide := func() { a.update(a.hideViewer) } // once the frame is built
	if iconButton(c, iconPutAway, L("收起（Esc）", "Put away (Esc)")).Clicked() {
		hide()
	}
	// Esc puts it away, unless a terminal or a field has the keyboard:
	// their programs take Esc (Claude Code stops with it).
	if !a.termFocus {
		c.OnWindowShortcut(0, ui.KeyEscape, hide)
	}
}

// middle is the tab bar, then the terminals, with what shows beside them.
func (a *app) middle(c *ui.Context) {
	t := c.Theme()
	// A video put aside, as another viewer shows, stops playing.
	if f := a.file; f != nil && f.player != nil && a.viewing != "file:"+f.path && f.player.Playing() {
		f.player.Pause()
	}
	ui.Column(c).Grow(1).FillHeight().Children(func() {
		a.tabBar(c)
		if a.notice != "" {
			ui.Row(c).FillWidth().PaddingX(12).Height(30).Gap(8).AlignItems(ui.Center).Background(t.Warning.Alpha(0.12)).Children(func() {
				ui.Text(c, a.notice).FontSize(12).TextColor(t.Text).SingleLine().Shrink(1)
				ui.Spacer(c)
				if iconButton(c, iconX, L("关闭提示", "Dismiss")).Clicked() {
					a.notice = ""
				}
			})
		}
		terminals := func() {
			ui.Column(c).Fill().Children(func() {
				if tb := a.current(); tb != nil {
					a.pane(c, tb, tb.root)
				} else {
					a.empty(c)
				}
			})
		}
		kind := a.viewerKind()
		// The terminals and what shows beside them: one card, floating on
		// the window's color, a gap all around.
		start, end := float32(8), float32(8)
		if !a.st.SidebarHidden {
			start = 3 // the sidebar's edge makes the rest of the gap
		}
		if a.side {
			end = 3
		}
		ui.Column(c).Grow(1).FillWidth().Padding(0, end, 8, start).Children(func() {
			a.card(c, ui.Column(c).Fill()).Children(func() {
				switch {
				case kind == "":
					terminals()
				case a.viewerMax || a.current() == nil:
					// With no terminal to sit beside, it takes the middle.
					a.viewer(c, kind)
				default:
					if a.st.ViewerSplit == nil {
						a.st.ViewerSplit = map[string]float32{}
					}
					p, ok := a.st.ViewerSplit[kind]
					if !ok {
						p = viewerDefaults[kind]
					}
					before := p
					ui.SplitPercent(c, &p, terminals, func() { a.viewer(c, kind) }).SplitReset(viewerDefaults[kind]).Fill()
					if p != before {
						a.st.ViewerSplit[kind] = p
						a.layoutChanged = true
					}
				}
			})
		})
		a.toastView(c)
	})
}

// middleMin is the narrowest the terminals get: the right panel gives way
// first.
const middleMin = 420

// body is the sidebar, the middle and the right panel, in width, the
// body's (0 before the first frame: all fits). When not all fits, the
// right panel shows narrower; the widths the user chose stay as chosen.
func (a *app) body(c *ui.Context, width float32) {
	if a.st.SidebarWidth == 0 {
		a.st.SidebarWidth = sidebarDefault
	}
	if a.st.SideWidth == 0 {
		a.st.SideWidth = sideDefault
	}
	const edge = 5 // a resizer
	room := float32(1 << 20)
	if width > 0 {
		room = width - middleMin
	}
	if !a.st.SidebarHidden {
		room -= a.st.SidebarWidth + edge
	}
	var sideW float32
	if a.side {
		sideW = max(min(a.st.SideWidth, room-edge), sideMin)
	}
	// The middle's width, for the tab bar to fit its tabs to.
	a.middleW = 0
	if width > 0 {
		a.middleW = width
		if !a.st.SidebarHidden {
			a.middleW -= a.st.SidebarWidth + edge
		}
		if a.side {
			a.middleW -= sideW + edge
		}
	}
	if !a.st.SidebarHidden {
		ui.Column(c).Width(a.st.SidebarWidth).FillHeight().Shrink(0).Children(func() { a.sidebar(c) })
		a.resizer(c, "sidebar", &a.st.SidebarWidth, 1, sidebarMin, sidebarMax, sidebarDefault)
	}
	a.middle(c)
	if a.side {
		// An edge dragged sets the width chosen.
		w := sideW
		a.resizer(c, "side", &w, -1, sideMin, sideMax, sideDefault)
		if w != sideW {
			a.st.SideWidth = w
		}
		ui.Column(c).Width(sideW).FillHeight().Shrink(0).Transition(panelIn).Children(func() { a.sidePanel(c) })
	}
}

// toggleSidebar hides the sidebar, giving the terminals its room, or
// shows it again; Nook remembers which.
func (a *app) toggleSidebar() {
	a.st.SidebarHidden = !a.st.SidebarHidden
	a.save()
}

// panelIn is how a panel comes in: a short fade, from a little to the
// right; it goes at once. Nothing moves while the desktop asks for less
// motion.
var panelIn = ui.ElementTransition{Duration: 150 * time.Millisecond, Enter: &ui.Motion{X: 14}, ExitDuration: -1, Colors: true}
