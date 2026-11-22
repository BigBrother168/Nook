// Nook: a terminal for AI command-line tools that keeps their network use
// in view. Stage 1: projects, tabs, splits and shells; every terminal runs
// in a ConPTY Nook owns and ends with Nook.
package main

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"nook/core/checkpoint"
	"nook/core/config"
	"nook/core/lang"
	"nook/core/netguard"
	"nook/core/pty"
)

// app is the window's state; the view reads it and actions change it, on
// the main thread.
type app struct {
	win    *mygo.Window
	st     config.Settings
	shells []shell

	tabs         []*tab
	cur          int
	pendingFocus *session // to get the focus in the next build
	notice       string   // a problem to tell, shown above the terminals
	nextID       int
	noSave       bool // tests: keep the settings in memory

	// start starts a terminal; newSession, or a test's.
	start func(proj, dir string, sh shell) (*session, error)

	net        netState // what the monitor last saw
	live       live     // the terminals, for the monitor
	shimDir    string   // the guards' folder, first in terminals' PATH
	netOpen    bool     // the network panel shows
	requireSel bool     // the panel's "no proxy, no AI" switch

	convs    convCache // titles and what was said, read from the tools' files
	later    []func()  // changes to lists made as they were built, as a row dropped: run as the next frame starts
	viewing  string    // the closed conversation shown (its Ref.Key), "" for the terminals
	quitting bool      // Nook quits: the terminals end, their conversations stay listed

	tray                         *mygo.Tray
	trayState, trayTip, trayMenu string // what the tray shows, to change only what changed
	toldTray                     bool   // the first time in the tray was told

	proxyInput, toolInput string // what the settings' fields hold

	themeKey      string          // what themeCache was made of
	themeCache    *ui.Theme       // the window's theme as chosen
	viewerMax     bool            // the viewer takes the whole middle
	layoutChanged bool            // a split moved: the settings are saved at the next tick
	settingsTab   int             // the settings' section shown
	themeQuery    string          // the search of Ghostty's themes
	accentInput   string          // the accent typed in the settings
	fontInput     string          // the terminals' font typed in the settings
	themeFor      string          // "palette" or "term": what the theme list chooses, "" closed
	histClosed    map[string]bool // projects whose closed conversations are folded

	paletteOpen               bool // the command palette shows
	paletteQuery, paletteLast string
	paletteSel                int // the row chosen, among those found

	side        bool                    // the right panel shows
	sideTab     int                     // its tab: changes, files, previews
	changes     map[string]*projChanges // the changes tab, by project
	snapped     map[string]time.Time    // the last snapshot of each project
	confirm     *confirmAsk             // a question waiting for a yes
	diff        *diffView               // the comparison in the middle
	file        *fileView               // the file in the middle
	fileRoots   map[string]*fileNode    // the files tab, by project
	fileQuery   string                  // what the files tab finds
	fileIndexes map[string]*fileIndex   // every file of a project, for finding

	designPart  string     // the part of the look the drawing board colors
	designColor ui.Color   // its color, as the picker edits it
	bgPath      string     // the picture behind the look, as loaded
	bgBmp       *ui.Bitmap // and decoded
	mdOpts      markdown.Options

	web           *mygo.WebView // the previews' browser, made at the first
	webProxy      string        // the proxy it took as it started
	webURL        string        // the address it shows
	webInput      string        // the address typed in the previews tab
	loosePreviews []string      // the addresses of terminals of no project

	probe *probe
	raw   *rawLog
}

// version is Nook's, as mygo.json says.
const version = "1.0.0"

func main() {
	// One Nook: opened again, as from the Start menu while it sits in the
	// tray, it shows the window it has. Tests run their own.
	single := os.Getenv("NOOK_PROBE") == ""
	if single && !mygo.App.RequestSingleInstanceLock() {
		return
	}
	if single {
		clearWebData()
	}
	st := config.Load()
	orderConversations(&st)
	checkpoint.Dir = st.BackupDir        // where copies for undoing go
	lang.Set(st.Language, lang.System()) // before any text, the shells' names too
	mygo.App.SetLanguages("en", "zh-Hans")
	a := &app{
		st:     st,
		shells: detectShells(),
		cur:    -1,
		probe:  newProbe(os.Getenv("NOOK_PROBE")),
		raw:    &rawLog{},
	}
	a.start = a.newSession
	a.mdOpts.OnLink = a.followLink
	a.netOpen = os.Getenv("NOOK_PANEL") != "" // tests: open the network panel
	if path := os.Getenv("NOOK_RAW"); path != "" {
		a.raw.f, _ = os.Create(path)
	}
	if n := pty.CleanOrphans(); n > 0 {
		a.probe.log("ended %d orphaned consoles", n)
	}
	a.probe.log("bundled-conpty %v", pty.UsingBundledConPTY())

	// The network, before the first terminal, which takes the proxy.
	a.live.configure(a.st)
	a.net = a.scan()
	if a.st.RequireProxy == nil {
		// First run: a proxy running, or the internet through a tunnel,
		// means AI tools are reached through one here; none, that they
		// are reached directly.
		a.st.SetRequireProxy(a.net.proxy.Works() || len(netguard.Carrying(a.net.tunnels)) > 0)
		a.save()
	}
	a.requireSel = a.st.Requires()
	a.shimDir = installShims(a.st.AITools)
	a.probe.log("require %v proxy %s (%s) listening %v shims %q", a.st.Requires(), a.net.proxy.Addr(), a.net.proxy.Source, a.net.proxy.Listening, a.shimDir)

	mygo.App.WhenReady(func() {
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:     "Nook",
			Width:     1200,
			Height:    760,
			MinWidth:  640,
			MinHeight: 360,
			StateKey:  "main",
			Content:   ui.View(a.view),
		})
		// Closing the window ends every terminal: Nook keeps nothing running.
		a.win.OnClose(a.onClose)
		a.win.OnClosed(func() {
			a.quitting = true
			for _, s := range a.sessions() {
				s.closed = true
				a.live.remove(s)
				s.term.Close()
			}
		})
		// Opens ready to type: a terminal in the last project, or the one
		// given after "--" (for tests).
		a.win.Update(func() {
			sh := a.defaultShell()
			if cmd := argsCommand(); cmd != nil {
				sh = shell{ID: "args", Name: L("命令", "Command"), Command: cmd, Quote: quotePowerShell}
			}
			a.openTab(a.st.LastProject, sh)
		})
		a.proxyInput = a.st.Proxy
		a.applyLanguage()
		a.applyTheme()
		a.startTray()
		mygo.App.OnNotificationClick(func(id string) { a.win.Update(func() { a.onNotificationClick(id) }) })
		if single {
			mygo.App.OnSecondInstance(func(args []string, _ string) { a.win.Update(func() { a.onSecondInstance(args) }) })
		}
		go a.monitor()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// argsCommand returns the arguments after "--", nil when there are none.
func argsCommand() []string {
	for i, arg := range os.Args {
		if arg == "--" && i+1 < len(os.Args) {
			return os.Args[i+1:]
		}
	}
	return nil
}

// probe logs to a file named by NOOK_PROBE, for tests; nothing otherwise.
type probe struct {
	mu sync.Mutex
	f  *os.File
}

func newProbe(path string) *probe {
	p := &probe{}
	if path != "" {
		p.f, _ = os.Create(path)
	}
	return p
}

func (p *probe) log(format string, args ...any) {
	if p == nil || p.f == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	fmt.Fprintf(p.f, "%s "+format+"\n", append([]any{time.Now().Format("15:04:05.000")}, args...)...)
}

// rawLog records the bytes both ways, quoted, to a file named by NOOK_RAW,
// for diagnosing what passes through ConPTY; nothing otherwise.
type rawLog struct {
	mu sync.Mutex
	f  *os.File
}

func (r *rawLog) dump(dir string, b []byte) {
	if r == nil || r.f == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	fmt.Fprintf(r.f, "%s %s %q\n", time.Now().Format("15:04:05.000"), dir, b)
}

// update runs fn on the main thread with the next frame: for what a
// goroutine read. Without a window, as in tests, it is dropped.
func (a *app) update(fn func()) {
	if a.win != nil {
		a.win.Update(fn)
	}
}
