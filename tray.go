package main

import (
	"bytes"
	_ "embed"
	"image"
	"image/color"
	"image/png"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"nook/core/lang"
	"nook/core/netguard"
)

// Nook lives in the notification area, as a desk pet does: closing the
// window can put it there, the terminals and AI tools going on, and only
// quitting ends them all. It is still one process: nothing runs after it,
// and nothing starts it with Windows.

//go:embed resources/icon.png
var appIcon []byte

// appBitmap is the app's icon, for the about page.
var appBitmap = sync.OnceValue(func() *ui.Bitmap {
	b, _ := ui.DecodeBitmap(appIcon)
	return b
})

// What the × of the window does (Settings.CloseAction).
const (
	closeAsk  = ""
	closeTray = "tray"
	closeQuit = "quit"
)

// trayIcons are the tray's icon, plain and with a dot for the AI tools'
// state: working, or waiting for the user.
var trayIcons = makeTrayIcons()

func makeTrayIcons() map[string][]byte {
	out := map[string][]byte{}
	src, err := png.Decode(bytes.NewReader(appIcon))
	if err != nil {
		return out
	}
	base := shrink(src, 32)
	out[""] = encodePNG(base)
	out["working"] = encodePNG(withDot(base, color.NRGBA{0x3b, 0x82, 0xf6, 0xff}))
	out["blocked"] = encodePNG(withDot(base, color.NRGBA{0xf5, 0x9e, 0x0b, 0xff}))
	return out
}

// shrink scales img down to size×size, averaging the pixels each one
// covers.
func shrink(img image.Image, size int) *image.NRGBA {
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			x0, x1 := b.Min.X+x*b.Dx()/size, b.Min.X+(x+1)*b.Dx()/size
			y0, y1 := b.Min.Y+y*b.Dy()/size, b.Min.Y+(y+1)*b.Dy()/size
			var r, g, bl, a, n uint64
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					pr, pg, pb, pa := img.At(xx, yy).RGBA() // premultiplied
					r, g, bl, a, n = r+uint64(pr), g+uint64(pg), bl+uint64(pb), a+uint64(pa), n+1
				}
			}
			if n == 0 || a == 0 {
				continue
			}
			// Back from premultiplied to straight alpha.
			out.SetNRGBA(x, y, color.NRGBA{
				R: uint8(r * 0xff / a), G: uint8(g * 0xff / a), B: uint8(bl * 0xff / a), A: uint8(a / n >> 8),
			})
		}
	}
	return out
}

// withDot returns img with a dot of c at its lower right.
func withDot(img *image.NRGBA, c color.NRGBA) *image.NRGBA {
	out := image.NewNRGBA(img.Bounds())
	copy(out.Pix, img.Pix)
	size := img.Bounds().Dx()
	r := float64(size) * 0.22
	cx, cy := float64(size)-r-0.5, float64(size)-r-0.5
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := float64(x)+0.5-cx, float64(y)+0.5-cy
			switch d := dx*dx + dy*dy; {
			case d <= r*r:
				out.SetNRGBA(x, y, c)
			case d <= (r+1.5)*(r+1.5): // a ring of the background, to set it off
				out.SetNRGBA(x, y, color.NRGBA{0xff, 0xff, 0xff, 0xff})
			}
		}
	}
	return out
}

func encodePNG(img image.Image) []byte {
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

// startTray puts Nook's icon in the notification area.
func (a *app) startTray() {
	t, err := mygo.NewTray(mygo.TrayOptions{ID: "nook", Icon: trayIcons[""], ToolTip: "Nook"})
	if err != nil {
		a.probe.log("tray: %v", err)
		return
	}
	a.tray = t
	t.OnClick(func() { a.win.Update(a.showWindow) })
	a.syncTray()
}

// syncTray shows the AI tools' state on the tray icon, and the menu as
// the settings are.
func (a *app) syncTray() {
	if a.tray == nil {
		return
	}
	working, blocked := 0, 0
	for _, s := range a.sessions() {
		switch s.status {
		case "working":
			working++
		case "blocked":
			blocked++
		}
	}
	state, tip := "", "Nook"
	switch {
	case blocked > 0:
		state, tip = "blocked", "Nook · "+itoa(blocked)+L(" 个 AI 等你确认", " AI waiting for you")
	case working > 0:
		state, tip = "working", "Nook · "+itoa(working)+L(" 个 AI 工作中", " AI working")
	}
	if _, direct := a.net.running(); direct+a.net.elsewhereDirect() > 0 {
		tip += L(" · ⚠ 有 AI 直连", " · ⚠ AI going direct")
	}
	if a.st.Locked {
		tip += L(" · 联网已锁", " · network locked")
	}
	if state != a.trayState {
		a.trayState = state
		a.tray.SetIcon(trayIcons[state], false)
	}
	if tip != a.trayTip {
		a.trayTip = tip
		a.tray.SetToolTip(tip)
	}
	menu := a.trayMenuKey()
	if menu != a.trayMenu {
		a.trayMenu = menu
		a.tray.SetMenu(a.trayMenuItems())
	}
}

func (a *app) trayMenuKey() string {
	return a.st.CloseAction + "|" + map[bool]string{true: "locked"}[a.st.Locked] + "|" + map[bool]string{true: "en"}[lang.English]
}

// trayMenuItems is the tray's menu.
func (a *app) trayMenuItems() *mygo.Menu {
	do := func(fn func()) func(*mygo.MenuItem, *mygo.Window) {
		return func(*mygo.MenuItem, *mygo.Window) { a.win.Update(fn) }
	}
	lock := L("锁上联网（AI 不能启动）", "Lock the network (no AI can start)")
	if a.st.Locked {
		lock = L("解开联网锁", "Unlock the network")
	}
	closeItem := func(label, action string) *mygo.MenuItem {
		return &mygo.MenuItem{Label: label, Type: mygo.MenuItemRadio, Checked: a.st.CloseAction == action,
			Click: do(func() { a.st.CloseAction = action; a.save(); a.syncTray() })}
	}
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: L("显示窗口", "Show the window"), Click: do(a.showWindow)},
		{Label: L("设置…", "Settings…"), Click: do(func() { a.showWindow(); a.openSettings() })},
		{Label: lock, Click: do(func() { a.toggleLock(); a.syncTray() })},
		{Label: L("点窗口的 × 时", "The window's ×"), Type: mygo.MenuItemSubmenu, Submenu: []*mygo.MenuItem{
			closeItem(L("每次都问", "Asks each time"), closeAsk),
			closeItem(L("收进托盘", "Hide in the tray"), closeTray),
			closeItem(L("退出 Nook", "Quit Nook"), closeQuit),
		}},
		{Type: mygo.MenuItemSeparator},
		{Label: L("退出 Nook（结束所有终端和 AI）", "Quit Nook (ends every terminal and AI tool)"), Click: do(a.quit)},
	})
}

func (a *app) showWindow() {
	a.win.Show()
	a.win.Focus()
}

// onClose decides what the window's × does: put Nook in the tray, quit,
// or ask which.
func (a *app) onClose(e *mygo.CloseEvent) {
	if a.quitting || a.tray == nil {
		a.quitting = true
		return
	}
	e.PreventDefault()
	switch a.st.CloseAction {
	case closeTray:
		a.toTray()
	case closeQuit:
		a.quit()
	default:
		a.update(func() {
			a.showWindow()
			a.closeAsking, a.closeRemember = true, false
		})
	}
}

// closeDialog asks, as the window's × is clicked, whether to put Nook in
// the tray or to quit, in the window's own look; each choice says what it
// does, and can be remembered.
func (a *app) closeDialog(c *ui.Context) {
	if !a.closeAsking {
		return
	}
	t := c.Theme()
	open, chosen := true, ""
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(360).Gap(10).Children(func() {
			ui.Text(c, L("关闭 Nook", "Close Nook")).FontSize(15).Bold().Padding(0, 0, 4, 0)
			option := func(id, title, sub string, first bool) {
				c.Key(id)
				b := ui.ButtonBase(c).Label(title).FillWidth().Padding(10, 14).Radius(9).Border(1, t.Border).Justify(ui.Start)
				switch {
				case first:
					b.Background(t.Accent.Alpha(0.14)).Border(1, t.Accent.Alpha(0.55))
				case b.Hovered():
					b.Background(t.SurfaceHover)
				}
				b.Children(func() {
					ui.Column(c).Gap(2).Children(func() {
						ui.Text(c, title).FontSize(13).Bold()
						ui.Text(c, sub).FontSize(12).TextColor(t.TextMuted)
					})
				})
				if b.Clicked() {
					chosen = id
				}
			}
			option(closeTray, L("收进托盘", "Hide in the tray"), L("终端和 AI 继续运行", "Terminals and AI keep running"), true)
			option(closeQuit, L("退出 Nook", "Quit Nook"), L("全部结束", "Everything ends"), false)
			ui.Row(c).FillWidth().AlignItems(ui.Center).Margin(4, 0, 0, 0).Children(func() {
				ui.Checkbox(c, &a.closeRemember, L("记住，下次不再问", "Remember; do not ask again"))
				ui.Spacer(c)
				if smallButton(c, L("取消", "Cancel"), false).Clicked() {
					open = false
				}
			})
		})
	})
	if !open {
		a.closeAsking = false
		return
	}
	if chosen == "" {
		return
	}
	a.closeAsking = false
	if a.closeRemember {
		a.st.CloseAction = chosen
		a.save()
		a.syncTray()
	}
	if chosen == closeTray {
		a.toTray()
	} else {
		a.quit()
	}
}

// toTray hides the window; the terminals go on.
func (a *app) toTray() {
	a.win.Hide()
	if !a.toldTray {
		a.toldTray = true
		mygo.NewNotification(mygo.NotificationOptions{
			Title: L("Nook 在托盘里继续运行", "Nook goes on in the tray"),
			Body:  L("终端还在运行，点托盘图标回来。", "Terminals keep running; click the tray icon to come back."),
		}).Show()
	}
}

// quit ends Nook: the AI tools first, asked as the user would, then
// every terminal, then the window.
func (a *app) quit() {
	if a.quitting {
		return
	}
	a.quitting = true
	a.win.Hide()
	ending := a.aiTerminals(a.everySession())
	go func() {
		endGently(ending)
		a.win.Update(func() {
			// What pages of the internet kept goes with Nook.
			if a.webStarted {
				mygo.App.ClearBrowsingData()
			}
			mygo.App.Quit()
		})
	}()
}

// aiTerm is a terminal with AI tools running, to end them gently.
type aiTerm struct {
	s     *session
	procs map[uint32]time.Time // the tools' processes, by when they started
}

// aiTerminals returns those of ss with AI tools running.
func (a *app) aiTerminals(ss []*session) []aiTerm {
	var out []aiTerm
	for _, s := range ss {
		u, ok := a.net.ai[s.id]
		if !ok || s.term == nil {
			continue
		}
		t := aiTerm{s: s, procs: map[uint32]time.Time{}}
		for _, pid := range u.pids {
			t.procs[pid] = netguard.Started(pid)
		}
		out = append(out, t)
	}
	return out
}

// endGently asks the AI tools to end, with Ctrl+C as the user would, and
// waits for them a little. A tool ended outright may count it as a
// failure: Claude Code killed early twice turns its full screen off.
func endGently(ts []aiTerm) {
	alive := func() bool {
		for _, t := range ts {
			for pid, started := range t.procs {
				if s := netguard.Started(pid); !s.IsZero() && s.Equal(started) {
					return true
				}
			}
		}
		return false
	}
	for i := 0; i < 3 && alive(); i++ {
		for _, t := range ts {
			t.s.term.Send([]byte{0x03})
		}
		time.Sleep(300 * time.Millisecond)
	}
	for deadline := time.Now().Add(3 * time.Second); alive() && time.Now().Before(deadline); {
		time.Sleep(100 * time.Millisecond)
	}
}
