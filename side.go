package main

import (
	"os"
	"time"

	"github.com/egoist/mygo/ui"
)

// The right panel: the changes the AI made, the files, and the previews,
// for the project of the terminal in use.

// sideTabs are the right panel's tabs.
func sideTabs() []string {
	return []string{L("改动", "Changes"), L("文件", "Files"), L("预览", "Previews")}
}

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
		ui.Row(c).FillWidth().Height(38).PaddingX(8).Gap(2).AlignItems(ui.Center).Children(func() {
			for i, name := range sideTabs() {
				c.Key(i)
				b := ui.ButtonBase(c).Label(name).Padding(5, 10).Radius(6)
				if i == a.sideTab {
					b.Background(t.Background)
				} else if b.Hovered() {
					b.Background(t.SurfaceHover)
				}
				b.Children(func() {
					ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
						txt := ui.Text(c, name).FontSize(12)
						if i != a.sideTab {
							txt.TextColor(t.TextMuted)
						}
						// The changes tab counts the files changed since the last snapshot.
						if pc := a.changes[proj]; i == 0 && pc != nil && len(pc.changes) > 0 {
							ui.Text(c, itoa(len(pc.changes))).FontSize(10).Bold().TextColor(t.AccentText).Padding(0, 5).Radius(7).Background(t.Accent)
						}
					})
				})
				if b.Clicked() {
					a.sideTab = i
				}
			}
			ui.Spacer(c)
			if iconButton(c, iconX, L("收起右侧面板（Ctrl+Shift+B）", "Hide the right panel (Ctrl+Shift+B)")).Clicked() {
				a.side = false
			}
		})
		ui.Divider(c)
		switch a.sideTab {
		case 0:
			a.changesTab(c, proj)
		case 1:
			a.filesTab(c, proj)
		default:
			a.previewTab(c, proj)
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
