package main

import (
	"net"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

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

// openWeb shows address raw in the middle; why not, in the notice.
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

// showWeb shows u in the middle: an address typed or found, or an HTML
// file of the project's (only the files tab opens file: addresses).
func (a *app) showWeb(u *url.URL) {
	if !local(u) {
		if a.st.Locked {
			a.notice = L("联网锁锁着，不打开外部网页", "The network lock is on: pages of the internet do not open")
			return
		}
		if why := a.webBlocked(); why != "" {
			a.notice = why
			return
		}
	}
	if a.web == nil {
		// The browser takes its arguments once, as it starts.
		proxy := ""
		if p := a.net.proxy; p.Works() {
			proxy = p.URL()
		}
		os.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", webArgs(proxy))
		// Private, as a browser's private window: cookies, history and what
		// pages store stay in memory and go as Nook quits.
		w, err := a.win.NewWebView(mygo.WebViewOptions{URL: u.String(), Page: mygo.PageOptions{Untrusted: true, Session: mygo.NewEphemeralSession()}})
		if err != nil {
			a.notice = L("无法打开预览：", "Cannot open the preview: ") + err.Error()
			return
		}
		a.web, a.webProxy = w, proxy
		// A link to the internet is held to the same rules as an address
		// typed.
		w.Page().OnWillNavigate(func(e *mygo.NavigateEvent) {
			if u, err := url.Parse(e.URL); err == nil && (u.Scheme == "http" || u.Scheme == "https") && !local(u) {
				why := a.webBlocked()
				if a.st.Locked {
					why = L("联网锁锁着，不打开外部网页", "The network lock is on: pages of the internet do not open")
				}
				if why != "" {
					e.PreventDefault()
					a.notice = why
				}
			}
		})
		w.Page().OnDidNavigate(func(u string) { a.win.Update(func() { a.webURL = u }) })
	} else if err := a.web.Page().LoadURL(u.String()); err != nil {
		a.notice = L("无法打开：", "Cannot open: ") + err.Error()
		return
	}
	a.webURL = u.String()
	a.viewing = "web:"
}

// webBlocked says why a page of the internet must not open now, "" when
// it may: it would go out without the proxy, from this computer's own IP.
func (a *app) webBlocked() string {
	tunnel := len(netguard.Carrying(a.net.tunnels)) > 0
	switch {
	case a.web != nil && a.webProxy == "" && a.st.Requires() && !tunnel:
		return L("预览是在代理打开之前启动的，不会走代理：重开 Nook 后再打开外部网页", "Previews started before the proxy and would not use it: reopen Nook to open pages of the internet")
	case a.web == nil && !a.net.proxy.Works() && a.st.Requires() && !tunnel:
		return L("没有代理，上网也不走虚拟网卡：为了不用真实 IP 连出去，不打开外部网页", "No proxy, and the internet does not go through a tunnel: pages of the internet stay closed, not to connect from your real IP")
	}
	return ""
}

// lockedWeb closes a page of the internet as the network lock locks.
func (a *app) lockedWeb() {
	if a.web == nil || a.webURL == "" {
		return
	}
	if u, err := url.Parse(a.webURL); err == nil && !local(u) {
		a.web.Page().LoadURL("about:blank")
		a.webURL = ""
		if a.viewing == "web:" {
			a.closeMainView()
		}
	}
}

// previewTab is the right panel's tab of previews, for project proj.
func (a *app) previewTab(c *ui.Context, proj string) {
	t := c.Theme()
	ui.Column(c).FillWidth().Padding(10).Gap(8).Children(func() {
		ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
			in := ui.TextInput(c, &a.webInput).Placeholder(L("网址，或要搜的词", "An address, or words to search")).Grow(1)
			submitted := in.Submitted()
			if smallButton(c, L("打开", "Open"), false).Clicked() || submitted {
				a.openWeb(a.webInput)
			}
		})
		ui.Text(c, L("输入词语用 DuckDuckGo 搜。外部网页只走代理，像无痕窗口，不留记录。", "Words are searched with DuckDuckGo. Pages of the internet go only through the proxy, privately, keeping nothing.")).FontSize(11).TextColor(t.TextMuted)
		list := a.previews(proj)
		if len(list) == 0 {
			ui.Text(c, L("运行开发服务器（比如 npm run dev）后，终端里出现的 localhost 地址会自动列在这里。", "Run a development server (as npm run dev): the localhost addresses its terminal prints show here.")).FontSize(12).TextColor(t.TextMuted).Padding(8, 0, 0, 0)
		}
		for _, u := range list {
			c.Key(u)
			row := ui.Row(c).FillWidth().Height(28).PaddingX(6).Gap(6).AlignItems(ui.Center).Radius(6).Cursor(ui.CursorPointer)
			if a.viewing == "web:" && a.webURL == u {
				row.Background(t.Selection)
			} else if row.Hovered() {
				row.Background(t.SurfaceHover)
			}
			row.Children(func() {
				ui.Icon(c, iconGlobe).FontSize(13).TextColor(t.TextMuted)
				ui.Text(c, strings.TrimPrefix(u, "http://")).FontSize(12).SingleLine().Shrink(1).Grow(1)
				if iconButton(c, iconX, L("从列表中去掉", "Remove from the list")).Clicked() {
					a.removePreview(proj, u)
				}
			})
			if row.Clicked() {
				a.openWeb(u)
			}
		}
	})
}

// webPage is the middle when a page shows.
func (a *app) webPage(c *ui.Context) {
	t := c.Theme()
	page := a.web.Page()
	external := false
	if u, err := url.Parse(a.webURL); err == nil {
		external = !local(u)
	}
	ui.Column(c).Grow(1).FillWidth().Children(func() {
		ui.Row(c).FillWidth().PaddingX(10).PaddingY(6).Gap(4).AlignItems(ui.Center).Children(func() {
			if iconButton(c, iconBack, L("后退", "Back")).Clicked() {
				page.GoBack()
			}
			if iconButton(c, iconForward, L("前进", "Forward")).Clicked() {
				page.GoForward()
			}
			if iconButton(c, iconReload, L("刷新", "Reload")).Clicked() {
				page.Reload()
			}
			if external {
				ui.Icon(c, iconGlobe).FontSize(13).TextColor(t.Warning).Tooltip(L("外部网页，经代理联网", "A page of the internet, through the proxy"))
			}
			ui.Text(c, a.webURL).FontSize(12).TextColor(t.TextMuted).SingleLine().Shrink(1).Grow(1).Padding(0, 6)
			a.viewerButtons(c)
		})
		ui.WebView(c, a.web).Grow(1).FillWidth()
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
