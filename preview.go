package main

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"nook/core/netguard"
	"nook/core/stream"
)

// Previews: each project keeps the local addresses its terminals print,
// as a development server's, and shows one in the middle with a click.
// Pages of the internet show too, through the proxy only, never with the
// network lock locked, and what they keep is cleared when Nook quits.

// maxPreviews is how many addresses a project keeps.
const maxPreviews = 8

// addPreview keeps an address a terminal of project proj printed, the
// latest first.
func (a *app) addPreview(proj, u string) {
	for i := range a.st.Projects {
		p := &a.st.Projects[i]
		if p.Path != proj {
			continue
		}
		if len(p.Previews) > 0 && p.Previews[0] == u {
			return
		}
		p.Previews = append([]string{u}, slices.DeleteFunc(p.Previews, func(x string) bool { return x == u })...)
		if len(p.Previews) > maxPreviews {
			p.Previews = p.Previews[:maxPreviews]
		}
		a.save()
		return
	}
	// A terminal of no project: kept until Nook quits.
	if !slices.Contains(a.loosePreviews, u) {
		a.loosePreviews = append([]string{u}, a.loosePreviews...)
	}
}

// previews returns the addresses of project proj.
func (a *app) previews(proj string) []string {
	for _, p := range a.st.Projects {
		if p.Path == proj {
			return p.Previews
		}
	}
	return a.loosePreviews
}

func (a *app) removePreview(proj, u string) {
	for i := range a.st.Projects {
		if p := &a.st.Projects[i]; p.Path == proj {
			p.Previews = slices.DeleteFunc(p.Previews, func(x string) bool { return x == u })
			a.save()
			return
		}
	}
	a.loosePreviews = slices.DeleteFunc(a.loosePreviews, func(x string) bool { return x == u })
}

// local reports whether u is an address of this computer.
func local(u *url.URL) bool {
	if u.Scheme == "file" {
		return true
	}
	h := u.Hostname()
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && (ip.IsLoopback() || ip.IsUnspecified())
}

// parseAddress reads what the user typed as an address.
func parseAddress(s string) (*url.URL, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, false
	}
	if !strings.Contains(s, "://") {
		host, _, _ := strings.Cut(s, "/")
		if strings.HasPrefix(host, "localhost") || strings.HasPrefix(host, "127.") || strings.HasPrefix(host, "[::1]") {
			s = "http://" + s
		} else {
			s = "https://" + s
		}
	}
	u, err := url.Parse(stream.NormalizeLocal(s))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, false
	}
	return u, true
}

// webArgs are the arguments of the browser the previews show in: the
// proxy, which pages of the internet go through (this computer's own
// addresses never do), and no networking of its own, as updates and
// reputation checks.
func webArgs(proxy string) string {
	args := "--disable-background-networking --disable-component-update --disable-domain-reliability --disable-sync --no-pings --disable-features=msSmartScreenProtection,msEdgeShoppingAssistant,msEdgeCollections"
	if proxy != "" {
		args += " --proxy-server=" + proxy
	}
	return args
}

// openWeb shows what was typed or clicked in the previews tab: the page
// open at it, else a new page; why not, in the notice.
func (a *app) openWeb(raw string) {
	u, ok := webTarget(raw)
	if !ok {
		return
	}
	a.showWeb(u)
}

// webTarget is where what was typed in the address bar goes: the address,
// or words searched with DuckDuckGo, which keeps no record of who
// searches (a page of the internet: through the proxy, never with the
// lock on).
func webTarget(raw string) (*url.URL, bool) {
	q := strings.TrimSpace(raw)
	if q == "" {
		return nil, false
	}
	if u, ok := parseAddress(q); ok && !isWords(q) {
		return u, true
	}
	return &url.URL{Scheme: "https", Host: "duckduckgo.com", Path: "/", RawQuery: url.Values{"q": {q}}.Encode()}, true
}

// isWords tells words to search from an address: a space, or no dot,
// port or scheme in what would be the host ("golang", not "go.dev" nor
// "localhost:3000").
func isWords(s string) bool {
	if strings.ContainsAny(s, " \t") {
		return true
	}
	if strings.Contains(s, "://") {
		return false
	}
	host, _, _ := strings.Cut(s, "/")
	return !strings.ContainsAny(host, ".:[") && !strings.EqualFold(host, "localhost")
}

// The previews' browser works as a browser does, its pages in tabs beside
// the terminals; it is private as a browser's private window is: what
// pages keep stays in memory, shared among them, and goes as Nook quits;
// pages of the internet go through the proxy only, and never open with
// the network lock locked. A link that would open a window, which the
// system's browser would take, without the proxy, opens a tab instead.

// maxPages is how many pages stay open at once.
const maxPages = 8

// webPage is a page open in the previews' browser: a tab of its own.
type webPage struct {
	id      int
	view    *mygo.WebView
	url     string
	title   string
	loading bool
	gone    *time.Timer // closed: it goes as this runs out, unless taken back
}

func (p *webPage) key() string { return "web:" + itoa(p.id) }

// blank reports whether the page shows nothing yet: a new one.
func (p *webPage) blank() bool { return p.url == "" || p.url == "about:blank" }

// name is what the page's tab says: its title, else its host.
func (p *webPage) name() string {
	if t := strings.TrimSpace(p.title); t != "" && t != "about:blank" {
		return t
	}
	if p.blank() {
		return L("新网页", "New page")
	}
	if u, err := url.Parse(p.url); err == nil && u.Host != "" {
		return u.Host
	}
	return p.url
}

// pageByKey is the page item key names, nil for none.
func (a *app) pageByKey(key string) *webPage {
	for _, p := range a.pages {
		if p.key() == key {
			return p
		}
	}
	return nil
}

// currentPage is the page showing beside the terminals, nil for none.
func (a *app) currentPage() *webPage {
	if !strings.HasPrefix(a.viewing, "web:") {
		return nil
	}
	return a.pageByKey(a.viewing)
}

// lockNotice is why no page of the internet opens.
func lockNotice() string {
	return L("联网锁锁着，不打开外部网页", "The network lock is on: pages of the internet do not open")
}

// mayOpen says why u must not open now, "" when it may: pages of the
// internet only with the lock open, and through the proxy.
func (a *app) mayOpen(u *url.URL) string {
	if local(u) || u.Scheme == "file" || u.Scheme == "about" {
		return ""
	}
	if a.st.Locked {
		return lockNotice()
	}
	return a.webBlocked()
}

// showWeb shows u beside the terminals: the page open at it, else a new
// page. An address typed or found, a link of a conversation, or an HTML
// file of the project's (only the files tab opens file: addresses).
func (a *app) showWeb(u *url.URL) {
	for _, it := range a.items {
		if it.page != nil && it.page.url == u.String() {
			a.showItem(it)
			return
		}
	}
	if p := a.newPage(u); p != nil {
		a.showPage(p)
	}
}

// showPage shows page p beside the terminals.
func (a *app) showPage(p *webPage) {
	a.viewing, a.webAddrFocus = p.key(), false
}

// newPage opens u in a page of its own; nil, and why in the notice, when
// it cannot.
func (a *app) newPage(u *url.URL) *webPage {
	if why := a.mayOpen(u); why != "" {
		a.notice = why
		return nil
	}
	if len(a.pages) >= maxPages {
		a.notice = fmt.Sprintf(L("最多同时开 %d 个网页，先关掉一个", "At most %d pages at once: close one first"), maxPages)
		return nil
	}
	if !a.webStarted {
		// The browser takes its arguments once, as it starts.
		proxy := ""
		if p := a.net.proxy; p.Works() {
			proxy = p.URL()
		}
		os.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", webArgs(proxy))
		a.webProxy, a.webSession, a.webStarted = proxy, mygo.NewEphemeralSession(), true
	}
	w, err := a.win.NewWebView(mygo.WebViewOptions{URL: u.String(), Page: mygo.PageOptions{Untrusted: true, Session: a.webSession}})
	if err != nil {
		a.notice = L("无法打开预览：", "Cannot open the preview: ") + err.Error()
		return nil
	}
	a.nextPage++
	p := &webPage{id: a.nextPage, view: w, url: u.String(), loading: true}
	a.pages = append(a.pages, p)
	pg := w.Page()
	// A link to the internet is held to the same rules as an address
	// typed.
	pg.OnWillNavigate(func(e *mygo.NavigateEvent) {
		if u, err := url.Parse(e.URL); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
			if why := a.mayOpen(u); why != "" {
				e.PreventDefault()
				a.win.Update(func() { a.notice = why })
				return
			}
		}
		a.win.Update(func() { p.loading = true })
	})
	pg.OnDidNavigate(func(s string) {
		a.win.Update(func() {
			p.url = s
			if a.currentPage() == p {
				a.webAddr = s
			}
		})
	})
	pg.OnPageTitleUpdated(func(e *mygo.TitleEvent) { a.win.Update(func() { p.title = e.Title }) })
	pg.OnDidFinishLoad(func() { a.win.Update(func() { p.loading = false }) })
	pg.OnDidFailLoad(func(*mygo.LoadError) { a.win.Update(func() { p.loading = false }) })
	// A link that would open a window opens a tab; one a script opens on
	// its own, nothing.
	pg.SetWindowOpenHandler(func(req mygo.WindowOpenRequest) *mygo.WindowOptions {
		if req.UserInitiated {
			a.win.Update(func() {
				if u, err := url.Parse(req.URL); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
					if np := a.newPage(u); np != nil {
						a.showPage(np)
					}
				}
			})
		}
		return nil
	})
	return p
}

// newBlankPage opens an empty page, its address bar waiting for one.
func (a *app) newBlankPage() {
	if p := a.newPage(&url.URL{Scheme: "about", Opaque: "blank"}); p != nil {
		p.url = "about:blank"
		a.showPage(p)
	}
}

// goPage takes page p to what was typed in its address bar: an address,
// or words to search.
func (a *app) goPage(p *webPage, raw string) {
	u, ok := webTarget(raw)
	if !ok {
		return
	}
	if why := a.mayOpen(u); why != "" {
		a.notice = why
		return
	}
	if err := p.view.Page().LoadURL(u.String()); err != nil {
		a.notice = L("无法打开：", "Cannot open: ") + err.Error()
		return
	}
	p.url, p.title, p.loading = u.String(), "", true
	a.webAddrFocus = false
}

// dropPage ends page p: its tab goes from every project, the page from
// the browser.
func (a *app) dropPage(p *webPage) {
	if p.gone != nil {
		p.gone.Stop()
	}
	drop := func(x viewItem) bool { return x.page == p }
	a.items = slices.DeleteFunc(a.items, drop)
	for g, v := range a.views {
		v.items = slices.DeleteFunc(v.items, drop)
		a.views[g] = v
	}
	a.pages = slices.DeleteFunc(a.pages, func(x *webPage) bool { return x == p })
	if !p.view.IsDestroyed() {
		p.view.Destroy()
	}
}

// webBlocked says why a page of the internet must not open now, "" when
// it may: it would go out without the proxy, from this computer's own IP.
func (a *app) webBlocked() string {
	tunnel := len(netguard.Carrying(a.net.tunnels)) > 0
	switch {
	case a.webStarted && a.webProxy == "" && a.st.Requires() && !tunnel:
		return L("预览比代理先启动，没走代理：重开 Nook 再打开外部网页", "Previews started before the proxy: reopen Nook to open pages of the internet")
	case !a.webStarted && !a.net.proxy.Works() && a.st.Requires() && !tunnel:
		return L("没有代理：外部网页不打开，免得露出真实 IP", "No proxy: pages of the internet stay closed, not to show your real IP")
	}
	return ""
}

// lockedWeb closes the pages of the internet as the network lock locks.
func (a *app) lockedWeb() {
	for _, p := range slices.Clone(a.pages) {
		if u, err := url.Parse(p.url); err == nil && !p.blank() && !local(u) && u.Scheme != "file" {
			p.view.Page().LoadURL("about:blank")
			a.dropPage(p)
		}
	}
	if strings.HasPrefix(a.viewing, "web:") && a.currentPage() == nil {
		a.viewing = ""
	}
}

// previewTab is the right panel's tab of previews, for project proj: an
// address or words to open in a new page, the pages open, the local
// addresses the terminals printed.
func (a *app) previewTab(c *ui.Context, proj string) {
	t := c.Theme()
	ui.Column(c).FillWidth().Padding(10).Gap(8).Children(func() {
		ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
			in := ui.TextInput(c, &a.webInput).Placeholder(L("网址或搜索", "Address or search")).Grow(1)
			submitted := in.Submitted()
			if smallButton(c, L("打开", "Open"), false).Clicked() || submitted {
				a.openWeb(a.webInput)
				a.webInput = ""
			}
		})
		ui.Text(c, L("无痕 · 只走代理 · 词语用 DuckDuckGo 搜", "Private · proxy only · words searched with DuckDuckGo")).FontSize(11).TextColor(t.TextMuted)
		row := func(key string, chosen bool, children func()) ui.Element {
			c.Key(key)
			r := ui.Row(c).FillWidth().Height(30).PaddingX(6).Gap(7).AlignItems(ui.Center).Radius(7).Cursor(ui.CursorPointer)
			if chosen {
				r.Background(t.Selection)
			} else if r.Hovered() {
				r.Background(t.SurfaceHover)
			}
			r.Children(children)
			return r
		}
		var open []viewItem
		for _, it := range a.items {
			if it.page != nil {
				open = append(open, it)
			}
		}
		if len(open) > 0 {
			ui.Text(c, L("开着的网页", "Pages open")).FontSize(12).Bold().TextColor(t.TextMuted).Padding(8, 0, 0, 2)
			for _, it := range open {
				p := it.page
				closing := false
				r := row("page:"+p.key(), a.viewing == p.key(), func() {
					if p.loading {
						ui.Spinner(c).FontSize(12).TextColor(t.Accent)
					} else {
						ui.Icon(c, iconGlobe).FontSize(13).TextColor(t.TextMuted)
					}
					ui.Text(c, p.name()).FontSize(12).SingleLine().Shrink(1).Grow(1)
					closing = iconButton(c, iconX, L("关掉这个网页", "Close this page")).Clicked()
				})
				switch {
				case closing:
					key := p.key()
					a.update(func() { a.closeView(key) })
				case r.Clicked():
					a.update(func() { a.showItem(it) })
				}
			}
			if smallButton(c, L("新开一个网页", "New page"), false).Clicked() {
				a.newBlankPage()
			}
		}
		list := a.previews(proj)
		ui.Text(c, L("终端里出现的地址", "Addresses the terminals printed")).FontSize(12).Bold().TextColor(t.TextMuted).Padding(8, 0, 0, 2)
		if len(list) == 0 {
			ui.Text(c, L("跑起 npm run dev，地址会出现在这里。", "Run npm run dev: its address shows here.")).FontSize(12).TextColor(t.TextMuted).Padding(0, 2)
		}
		for _, u := range list {
			cur := a.currentPage()
			r := row(u, cur != nil && cur.url == u, func() {
				ui.Icon(c, iconGlobe).FontSize(13).TextColor(t.TextMuted)
				ui.Text(c, strings.TrimPrefix(u, "http://")).FontSize(12).SingleLine().Shrink(1).Grow(1)
				if iconButton(c, iconX, L("从列表中去掉", "Remove from the list")).Clicked() {
					a.removePreview(proj, u)
				}
			})
			if r.Clicked() {
				a.openWeb(u)
			}
		}
	})
}

// browser is the page showing beside the terminals, under its buttons:
// back, forward, reload (stop while it loads), its address, a new page.
func (a *app) browser(c *ui.Context) {
	p := a.currentPage()
	if p == nil || p.view.IsDestroyed() {
		return // closed as the frame built
	}
	page := p.view.Page()
	external := false
	if u, err := url.Parse(p.url); err == nil {
		external = !p.blank() && !local(u) && u.Scheme != "file"
	}
	ui.Column(c).Grow(1).FillWidth().Children(func() {
		ui.Row(c).FillWidth().PaddingX(10).PaddingY(6).Gap(4).AlignItems(ui.Center).Children(func() {
			if iconButton(c, iconBack, L("后退", "Back")).Clicked() {
				page.GoBack()
			}
			if iconButton(c, iconForward, L("前进", "Forward")).Clicked() {
				page.GoForward()
			}
			if p.loading {
				if iconButton(c, iconX, L("停止", "Stop")).Clicked() {
					page.Stop()
					p.loading = false
				}
			} else if iconButton(c, iconReload, L("刷新", "Reload")).Clicked() {
				page.Reload()
			}
			a.addressBar(c, p, external)
			if iconButton(c, iconPlus, L("新开一个网页", "New page")).Clicked() {
				a.newBlankPage()
			}
			a.viewerButtons(c)
		})
		ui.WebView(c, p.view).Grow(1).FillWidth()
	})
}

// addressBar is page p's address, to type another or words to search
// (Enter); Esc puts back the page's. A new page's waits for typing.
func (a *app) addressBar(c *ui.Context, p *webPage, external bool) {
	t := c.Theme()
	c.Key("addr:" + p.key())
	if !a.webAddrFocus {
		a.webAddr = p.url
		if p.blank() {
			a.webAddr = ""
		}
	}
	ui.Row(c).Grow(1).Shrink(1).Height(28).PaddingStart(8).Gap(4).Margin(0, 4).AlignItems(ui.Center).Radius(7).Background(t.SurfaceHover).Children(func() {
		if external {
			ui.Icon(c, iconGlobe).FontSize(13).TextColor(t.Warning).Tooltip(L("外部网页，经代理联网", "A page of the internet, through the proxy"))
		}
		in := ui.TextInputBase(c, &a.webAddr).Placeholder(L("网址或搜索", "Address or search")).FontSize(12).Grow(1).Shrink(1).
			Label(L("网址或搜索", "Address or search")).
			OnTextKey(func(k ui.TextKey) bool {
				if k.Key == ui.KeyEscape {
					a.webAddr = p.url
					return true
				}
				return false
			})
		if p.blank() {
			in.AutoFocus()
		}
		if in.Submitted() && strings.TrimSpace(a.webAddr) != "" {
			a.goPage(p, a.webAddr)
		}
		a.webAddrFocus = in.Focused()
		if a.webAddrFocus {
			a.termFocus = true // Esc is the field's
		}
	})
}

// clearWebData deletes what the preview's browser kept on disk: Nook
// 1.0.0 browsed in a profile kept there (history, cookies, caches), and
// the browser's own files of the last run, whose pages kept nothing. It
// runs as Nook starts, before any preview, the only Nook running.
func clearWebData() {
	if dir := os.Getenv("LOCALAPPDATA"); dir != "" {
		_ = os.RemoveAll(filepath.Join(dir, "Nook", "WebView2"))
	}
}
