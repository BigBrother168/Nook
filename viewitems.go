package main

import (
	"path/filepath"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"
)

// What shows beside the terminals: files, comparisons, web pages and
// conversations, several at once, each a small tab over them; opening one
// keeps the others. Each project has its own.

// viewItem is one thing open beside the terminals.
type viewItem struct {
	key  string // as a.viewing names it
	file *fileView
	diff *diffView
	page *webPage
}

// maxItems is how many stay open beside the terminals: opening more
// closes the oldest not shown, a web page excepted, which only its ×
// closes.
const maxItems = 12

// trackViewer keeps what shows (of kind) among the things open.
func (a *app) trackViewer(kind string) {
	it := viewItem{key: a.viewing}
	switch kind {
	case "file":
		it.file = a.file
	case "diff":
		it.diff = a.diff
	case "web":
		it.page = a.currentPage()
	}
	if i := slices.IndexFunc(a.items, func(x viewItem) bool { return x.key == it.key }); i >= 0 {
		a.items[i] = it
		return
	}
	a.items = append(a.items, it)
	for len(a.items) > maxItems {
		i := slices.IndexFunc(a.items, func(x viewItem) bool { return x.key != a.viewing && x.page == nil })
		if i < 0 {
			break
		}
		a.dropItem(i)
	}
}

// dropItem forgets item i, stopping what it plays.
func (a *app) dropItem(i int) {
	if f := a.items[i].file; f != nil && f.player != nil && f != a.file {
		f.player.Close()
		f.player = nil
	}
	a.items = slices.Delete(a.items, i, i+1)
}

// showItem shows item it beside the terminals.
func (a *app) showItem(it viewItem) {
	a.viewing = it.key
	switch {
	case it.file != nil:
		if a.file != it.file {
			a.file = it.file
			if it.file.player == nil && (it.file.kind == kindVideo || it.file.kind == kindAudio) {
				a.loadFile(it.file)
			}
		}
	case it.diff != nil:
		a.diff = it.diff
	case it.page != nil:
		a.webAddrFocus = false
	}
}

// closeItem closes what key names; when it showed, the one opened before
// shows, else the terminals have the middle again.
func (a *app) closeItem(key string) {
	i := slices.IndexFunc(a.items, func(x viewItem) bool { return x.key == key })
	if i >= 0 {
		if f := a.items[i].file; f != nil && f.player != nil {
			f.player.Close()
			f.player = nil
		}
		a.items = slices.Delete(a.items, i, i+1)
	}
	if a.viewing != key {
		return
	}
	if len(a.items) > 0 {
		a.showItem(a.items[min(max(i-1, 0), len(a.items)-1)])
		return
	}
	a.viewing, a.file, a.diff = "", nil, nil
}

// hideViewer puts away what shows beside the terminals: it stays open as
// it was, a page in its place, and the tab bar's button brings it back.
func (a *app) hideViewer() {
	if a.viewerKind() == "" {
		return
	}
	a.hidden, a.viewing = a.viewing, ""
	if s := a.focused(); s != nil {
		a.pendingFocus = s
	}
}

// putAway is what the tab bar's button brings back as nothing shows
// beside the terminals, things being open: what was put away, else the
// last opened; nil when nothing waits.
func (a *app) putAway() *viewItem {
	if len(a.items) == 0 || a.viewerKind() != "" {
		return nil
	}
	i := slices.IndexFunc(a.items, func(x viewItem) bool { return x.key == a.hidden })
	if i < 0 {
		i = len(a.items) - 1
	}
	return &a.items[i]
}

// closeView closes what key names, from its small tab. A page closed can
// come back for a few seconds, as it was: the browser keeps it until
// then.
func (a *app) closeView(key string) {
	i := slices.IndexFunc(a.items, func(x viewItem) bool { return x.key == key })
	if i < 0 {
		return
	}
	it, group := a.items[i], a.group
	a.closeItem(key)
	p := it.page
	if p == nil {
		return
	}
	p.gone = time.AfterFunc(pageGrace, func() { a.update(func() { a.dropPage(p) }) })
	a.toastFor(L("网页关了", "Page closed"), pageGrace, func() {
		if !slices.Contains(a.pages, p) || a.group != group || slices.ContainsFunc(a.items, func(x viewItem) bool { return x.page == p }) {
			return
		}
		p.gone.Stop()
		p.gone = nil
		a.items = slices.Insert(a.items, min(i, len(a.items)), it)
		a.showItem(it)
	})
}

// pageGrace is how long a page closed can be taken back.
const pageGrace = 5 * time.Second

// closeItems closes those open that drop reports.
func (a *app) closeItems(drop func(viewItem) bool) {
	for _, it := range slices.Clone(a.items) {
		if drop(it) {
			a.closeItem(it.key)
		}
	}
}

// itemTabs is the row of things open, over the one showing: a click
// shows one, its × closes it.
func (a *app) itemTabs(c *ui.Context) {
	if len(a.items) == 0 {
		return
	}
	t := c.Theme()
	var show *viewItem
	var closing string
	ui.Row(c).FillWidth().Height(34).PaddingX(6).Gap(2).AlignItems(ui.Center).Clip().BorderWidth(0, 0, 1, 0).BorderColor(t.Border.Alpha(0.6)).Children(func() {
		for _, it := range a.items {
			c.Key("item:" + it.key)
			sel := it.key == a.viewing
			r := ui.Row(c).Height(26).PaddingStart(8).PaddingEnd(3).Gap(5).AlignItems(ui.Center).Radius(6).MaxWidth(200).Shrink(1).Cursor(ui.CursorPointer)
			switch {
			case sel:
				r.Background(t.Selection)
			case r.Hovered():
				r.Background(t.SurfaceHover)
			}
			icon, col, name := a.itemLook(t, it)
			r.Tooltip(name)
			r.Children(func() {
				ui.Icon(c, icon).FontSize(13).TextColor(col)
				txt := ui.Text(c, name).FontSize(12).SingleLine().Shrink(1)
				if !sel {
					txt.TextColor(t.TextMuted)
				}
				if iconButton(c, iconX, L("关闭", "Close")).Clicked() {
					closing = it.key
				}
			})
			if r.Clicked() && closing == "" {
				show = &it
			}
		}
	})
	// Closing or showing another changes what the rest of this frame,
	// already building what shows, would find gone: it waits for the
	// frame's end.
	switch {
	case closing != "":
		key := closing
		a.update(func() { a.closeView(key) })
	case show != nil:
		it := *show
		a.update(func() { a.showItem(it) })
	}
}

// itemLook is how a thing open shows in its tab: an icon, its color, a
// name.
func (a *app) itemLook(t *ui.Theme, it viewItem) (*ui.SVG, ui.Color, string) {
	switch {
	case it.file != nil:
		icon, col := kindIcon(t, it.file.kind)
		return icon, col, filepath.Base(it.file.path)
	case it.diff != nil:
		return iconUndo, t.Warning, filepath.Base(it.diff.ch.Path)
	case it.page != nil:
		return iconGlobe, t.TextMuted, it.page.name()
	}
	if conv, ok := a.convByKey(it.key); ok {
		return iconHistory, t.TextMuted, a.convTitle(conv)
	}
	return iconFile, t.TextMuted, it.key
}
