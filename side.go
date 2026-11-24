package main

import (
	"os"
	"time"

	"github.com/egoist/mygo/ui"
)

// The right panel: the changes the AI made, the files, the previews, for
// the project in use; and the network.

// The right panel's tabs.
const (
	sideChanges = iota
	sideFiles
	sidePreviews
	sideNet
)

// sideTabs are the right panel's tabs' names.
func sideTabs() []string {
	return []string{L("改动", "Changes"), L("文件", "Files"), L("预览", "Previews"), L("联网", "Network")}
}

// sideIcons are the right panel's tabs' icons, in sideTabs' order.
var sideIcons = []*ui.SVG{iconUndo, iconFile, iconGlobe, iconShield}

// toggleSide shows or hides the right panel, at tab tab.
func (a *app) toggleSide(tab int) {
	if a.side && a.sideTab == tab {
		a.side = false
		return
	}
	a.side, a.sideTab = true, tab
}

// sidePanel builds the right panel.
func (a *app) sidePanel(c *ui.Context) {
	t := c.Theme()
	proj := a.currentProject()
	ui.Column(c).Fill().Background(a.panelColor(t, "panel")).Children(func() {
		// Four tabs, alike in width: an icon and a word each.
		ui.Row(c).FillWidth().Height(44).PaddingStart(6).PaddingEnd(4).Gap(2).AlignItems(ui.Center).Children(func() {
			for i, name := range sideTabs() {
				c.Key(i)
				b := ui.ButtonBase(c).Label(name).Grow(1).Shrink(1).Basis(0).Height(32).Radius(7)
				if i == a.sideTab {
					b.Background(t.Selection)
				} else if b.Hovered() {
					b.Background(t.SurfaceHover)
				}
				b.Children(func() {
					ui.Row(c).FillWidth().Justify(ui.Center).Gap(5).AlignItems(ui.Center).Children(func() {
						fg := t.Text
						if i != a.sideTab {
							fg = t.TextMuted
						}
						ui.Icon(c, sideIcons[i]).FontSize(13).TextColor(fg)
						ui.Text(c, name).FontSize(12).TextColor(fg).SingleLine()
						// The changes tab counts the files changed since the last snapshot.
						if pc := a.changes[proj]; i == sideChanges && pc != nil && len(pc.changes) > 0 {
							ui.Text(c, itoa(len(pc.changes))).FontSize(10).Bold().TextColor(t.AccentText).Padding(0, 5).Radius(7).Background(t.Accent)
						}
						// The network tab turns red while an AI shows the real IP.
						if i == sideNet && len(a.leaks()) > 0 {
							ui.Box(c).Width(7).Height(7).Radius(4).Background(t.Danger)
						}
					})
				})
				if b.Clicked() {
					a.sideTab = i
				}
			}
			if iconButton(c, iconX, L("收起右侧面板（Ctrl+Shift+B）", "Hide the right panel (Ctrl+Shift+B)")).Clicked() {
				a.side = false
			}
		})
		switch a.sideTab {
		case sideChanges:
			a.changesTab(c, proj)
		case sideFiles:
			a.filesTab(c, proj)
		case sidePreviews:
			a.previewTab(c, proj)
		default:
			a.netTab(c)
		}
	})
}

// onTick runs on the main thread as the monitor looks, every two
// seconds: the right panel follows what changes on disk.
func (a *app) onTick() {
	if a.layoutChanged {
		a.layoutChanged = false
		a.save()
	}
	if a.side {
		proj := a.currentProject()
		if proj != "" {
			if pc := a.projChanges(proj); !pc.busy && time.Since(pc.read) > 4*time.Second {
				a.refreshChanges(proj)
			}
			if a.sideTab == 1 {
				a.fileRoot(proj).refresh()
			}
		}
	}
	// A file shown is shown again when it changes.
	if f := a.file; f != nil && f.done && f.player == nil && a.viewing == "file:"+f.path {
		if fi, err := os.Stat(f.path); err == nil && !fi.ModTime().Equal(f.mod) {
			f.mod = fi.ModTime()
			a.loadFile(f)
		}
	}
}
