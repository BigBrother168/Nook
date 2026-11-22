package main

import (
	"github.com/egoist/mygo/ui"
)

// The window's columns: the sidebar, the middle and the right panel, each
// edge dragged to size; in the middle, the terminals and, beside them, a
// file, a comparison, a page or a conversation, each kind at the width
// the user left it.

// Widths when nothing was chosen, in DIPs, and the narrowest and widest.
const (
	sidebarDefault, sidebarMin, sidebarMax = 248, 180, 480
	sideDefault, sideMin, sideMax          = 330, 240, 720
	netDefault, netMin, netMax             = 340, 280, 640
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
	line, lw := t.Border, float32(1)
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
	case a.web != nil && a.viewing == "web:":
		return "web"
	}
	if _, ok := a.convByKey(a.viewing); ok {
		return "conv"
	}
	return ""
}

// viewer builds what shows beside the terminals.
func (a *app) viewer(c *ui.Context, kind string) {
	ui.Column(c).Fill().Background(c.Theme().Background).Children(func() {
		switch kind {
		case "diff":
			a.diffPage(c)
		case "file":
			a.filePage(c)
		case "web":
			a.webPage(c)
		case "conv":
			conv, _ := a.convByKey(a.viewing)
			a.convView(c, conv)
		}
	})
}

// viewerButtons are the buttons every viewer has at its top right: one
// widening it over the terminals and back, one closing it.
func (a *app) viewerButtons(c *ui.Context) {
	icon, tip := iconMaximize, L("放大（终端先让开）", "Widen over the terminals")
	if a.viewerMax {
		icon, tip = iconMinimize, L("还原，和终端并排", "Back beside the terminals")
	}
	if a.current() != nil && iconButton(c, icon, tip).Clicked() {
		a.viewerMax = !a.viewerMax
	}
	if iconButton(c, iconX, L("关闭", "Close")).Clicked() {
		a.closeMainView()
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
		if a.viewing == "settings" {
			a.settingsPage(c)
			return
		}
		if a.viewing == "designer" {
			a.designerPage(c)
			return
		}
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
}

// middleMin is the narrowest the terminals get: the panels beside them
// give way first.
const middleMin = 420

// body is the sidebar, the middle, the right panel and the network panel,
// in width, the body's (0 before the first frame: all fits). When not all
// fits, the network panel, opened to be read, shows in place of the
// right panel, which comes back as it closes, and a panel still too wide
// shows narrower; the widths the user chose stay as chosen.
func (a *app) body(c *ui.Context, width float32) {
	if a.st.SidebarWidth == 0 {
		a.st.SidebarWidth = sidebarDefault
	}
	if a.st.SideWidth == 0 {
		a.st.SideWidth = sideDefault
	}
	if a.st.NetWidth == 0 {
		a.st.NetWidth = netDefault
	}
	const edge = 5 // a resizer
	room := float32(1 << 20)
	if width > 0 {
		room = width - middleMin
	}
	if !a.st.SidebarHidden {
		room -= a.st.SidebarWidth + edge
	}
	fit := func(want, least float32) float32 { return max(min(want, room-edge), least) }
	var netW, sideW float32
	if a.netOpen {
		netW = fit(a.st.NetWidth, netMin)
		room -= netW + edge
	}
	side := a.side && (!a.netOpen || room-edge >= sideMin)
	if side {
		sideW = fit(a.st.SideWidth, sideMin)
	}
	// An edge dragged sets the width chosen.
	drag := func(key string, shown float32, chosen *float32, lo, hi, reset float32) {
		w := shown
		a.resizer(c, key, &w, -1, lo, hi, reset)
		if w != shown {
			*chosen = w
		}
	}
	if !a.st.SidebarHidden {
		ui.Column(c).Width(a.st.SidebarWidth).FillHeight().Shrink(0).Children(func() { a.sidebar(c) })
		a.resizer(c, "sidebar", &a.st.SidebarWidth, 1, sidebarMin, sidebarMax, sidebarDefault)
	}
	a.middle(c)
	if side {
		drag("side", sideW, &a.st.SideWidth, sideMin, sideMax, sideDefault)
		ui.Column(c).Width(sideW).FillHeight().Shrink(0).Children(func() { a.sidePanel(c) })
	}
	if a.netOpen {
		drag("net", netW, &a.st.NetWidth, netMin, netMax, netDefault)
		a.netPanel(c, netW)
	}
}

// toggleSidebar hides the sidebar, giving the terminals its room, or
// shows it again; Nook remembers which.
func (a *app) toggleSidebar() {
	a.st.SidebarHidden = !a.st.SidebarHidden
	a.save()
}
