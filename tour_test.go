package main

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"nook/core/config"
	"nook/core/history"
	"nook/core/netguard"
)

// auroraLook is a dark look of teal and blue, as made on the drawing
// board, for the shots.
var auroraLook = config.Design{ID: "aurora", Name: "极光", Dark: true, Background: "#132e37", Surface: "#1b2d36", Text: "#cedde3", Muted: "#96adb5", Border: "#29404b", Accent: "#77acb8", TermBg: "#111820", TermFg: "#b9cad2", Gradient: "#222e49", GradientAngle: 145, PanelOpacity: 0.59, TitleBar: "#14222d", Sidebar: "#111f29", TabBar: "#14232d", Panel: "#142c38", Selection: "#243e4b", TermTheme: "Nord", TermOwn: true}

// TestTourShots walks through the window as a user would, a shot at each
// step, into $NOOK_SHOTS: choosing projects, putting a tab away, ending a
// terminal, the settings, a narrow window. Made-up projects and
// conversations.
func TestTourShots(t *testing.T) {
	dir := os.Getenv("NOOK_SHOTS")
	if dir == "" {
		t.Skip("NOOK_SHOTS not set")
	}
	a := testApp()
	a.st.Designs = []config.Design{auroraLook}
	a.st.Palette = "my:aurora"
	a.st.SetRequireProxy(true)
	a.requireSel = true
	pa, pb, pc := `D:\code\my-app`, `D:\code\website`, `D:\notes`
	a.st.Projects = []config.Project{{Name: "my-app", Path: pa}, {Name: "website", Path: pb}, {Name: "notes", Path: pc}}
	a.net.ai = map[int]aiUse{}
	light, dark := a.termThemes()
	out := map[string]string{
		"codex":  "\x1b[1m>_ OpenAI Codex\x1b[0m (v0.171.0)\r\n\r\n> 你好\r\n\r\n你好！有什么可以帮你的？\r\n",
		"claude": "\x1b[38;5;209m Claude Code\x1b[0m v2.1.330\r\n\r\n\x1b[7m> 给首页加个小标题 \x1b[0m\r\n\r\n● 好的，我先看看 index.html 的结构。\r\n",
		"":       "PS D:\\code> ",
	}
	a.start = func(proj, dir string, sh shell) (*session, error) {
		a.nextID++
		s := &session{id: a.nextID, proj: proj, dir: dir, shell: sh}
		term, err := terminal.New(terminal.Options{Conn: newFake(out[sh.ID]), Theme: light, DarkTheme: dark})
		s.term = term
		return s, err
	}
	open := func(proj, tool, status, conv, title string) *session {
		s := a.openTabIn(proj, proj, shell{ID: tool, Name: "PowerShell 7"})
		s.status = status
		if tool != "" {
			s.conv = history.Ref{Tool: tool, ID: conv}
			a.convs.titles = mapSet(a.convs.titles, s.conv.Key(), title)
			a.convs.live = timeSet(a.convs.live, s.conv.Key())
			u := aiUse{tools: []string{tool}}
			u.routes[netguard.ViaProxy] = 2
			a.net.ai[s.id] = u
		}
		return s
	}
	open(pb, "claude", "blocked", "b1", "装依赖")
	open(pb, "", "", "", "")
	codex := open(pa, "codex", "done", "a1", "你好")
	codex.unread = true
	open(pa, "claude", "working", "a2", "首页小标题")
	for _, h := range []struct{ id, title, proj string }{{"h1", "新建 a.txt", pa}, {"h2", "整理 README", pc}, {"h3", "读书笔记格式", pc}} {
		conv := config.Conversation{Ref: history.Ref{Tool: "codex", ID: h.id}, Project: h.proj, Dir: h.proj, Seen: time.Now().Add(-time.Hour)}
		a.st.Conversations = append(a.st.Conversations, conv)
		a.convs.titles = mapSet(a.convs.titles, conv.Key(), h.title)
	}
	a.net.proxy = netguard.Proxy{Host: "127.0.0.1", Port: 10808, Source: "系统代理", Listening: true}

	tt := ui.NewTester(a.view, 1400, 820)
	// The window's buttons, at the top right as on Windows.
	tt.SetTitleBar(ui.TitleBar{Height: topBarHeight, Right: 138, Bottom: topBarHeight})
	shot := func(name string) {
		for i := 0; i < 5; i++ {
			tt.Advance(100 * time.Millisecond)
			tt.Frame()
		}
		f, err := os.Create(filepath.Join(dir, "tour-"+name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		png.Encode(f, tt.Image())
		f.Close()
	}
	shot("1-main")
	// Another project, and one with no terminal: its card.
	a.goProject(pb)
	shot("2-other-project")
	a.goProject(pc)
	shot("3-project-card")
	// A tab put away, then a terminal ended, with its note.
	a.goProject(pa)
	a.hideTab(a.current())
	shot("4-put-away")
	a.show(a.tabs[len(a.tabs)-1].focus)
	a.endSession(a.focused())
	shot("5-ended")
	a.undoEnd(a.endings[0])
	a.toasts = nil
	// The settings, over the whole window.
	a.openSettings()
	shot("6-settings")
	a.leavePages()
	// The network, in the right panel.
	a.side, a.sideTab = true, sideNet
	shot("7-network")
	a.side = false
	// Many tabs in a narrow window.
	for i := 0; i < 5; i++ {
		open(pa, "", "", "", "")
	}
	tt.SetSize(980, 700)
	shot("8-narrow-many")
}

// TestPageShots draws the pages and panels in the aurora look, into
// $NOOK_SHOTS: the changes, files and previews tabs, things open beside
// the terminals, the settings' sections, the drawing board, the close
// question, the welcome; then a light palette, and English.
func TestPageShots(t *testing.T) {
	dir := os.Getenv("NOOK_SHOTS")
	if dir == "" {
		t.Skip("NOOK_SHOTS not set")
	}
	proj := t.TempDir()
	for _, f := range []string{"README.md", "index.html", "src/app.js"} {
		p := filepath.Join(proj, filepath.FromSlash(f))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("# Demo\n\nA page.\n"), 0o644)
	}
	a := testApp()
	a.st.Designs = []config.Design{auroraLook}
	a.st.Palette = "my:aurora"
	a.st.Projects = []config.Project{{Name: "my-app", Path: proj}}
	a.net.proxy = netguard.Proxy{Host: "127.0.0.1", Port: 10808, Listening: true}
	out := "PS > "
	a.start = func(p, d string, sh shell) (*session, error) {
		a.nextID++
		s := &session{id: a.nextID, proj: p, dir: d, shell: sh}
		light, dark := a.termThemes()
		term, err := terminal.New(terminal.Options{Conn: newFake(out), Theme: light, DarkTheme: dark})
		s.term = term
		return s, err
	}
	a.openTab(proj, a.defaultShell())
	conv := config.Conversation{Ref: history.Ref{Tool: "codex", ID: "h1"}, Project: proj, Dir: proj, Seen: time.Now()}
	a.st.Conversations = []config.Conversation{conv}
	a.convs.titles = mapSet(a.convs.titles, conv.Key(), "新建 a.txt")
	tt := ui.NewTester(a.view, 1400, 820)
	tt.SetTitleBar(ui.TitleBar{Height: topBarHeight, Right: 138, Bottom: topBarHeight})
	shot := func(name string) {
		for i := 0; i < 5; i++ {
			tt.Advance(100 * time.Millisecond)
			tt.Frame()
		}
		f, err := os.Create(filepath.Join(dir, "page-"+name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		png.Encode(f, tt.Image())
		f.Close()
	}
	// Two things open beside the terminal: a file, and a conversation.
	a.side, a.sideTab = true, sideFiles
	a.fileRoot(proj).readDir()
	md := filepath.Join(proj, "README.md")
	a.file, a.viewing = &fileView{path: md, kind: kindMD, md: "# Demo\n\nA page.\n", done: true, size: 20}, "file:"+md
	shot("1-files")
	a.openConv(conv.Key(), proj)
	shot("2-two-open")
	a.hideViewer()
	shot("2-put-away")
	a.showItem(*a.putAway())
	a.closeItem(conv.Key())
	a.sideTab = sidePreviews
	shot("3-previews")
	a.sideTab = sideChanges
	shot("4-changes")
	for i, name := range []string{"general", "network", "look", "keys", "about"} {
		a.openSettings()
		a.settingsTab = i
		shot("5-settings-" + name)
	}
	a.leavePages()
	a.openDesigner(nil)
	shot("6-designer")
	a.designFine = true
	shot("6-designer-fine")
	quickPresets[1].apply(a.currentDesign())
	a.designChanged()
	a.leavePages()
	shot("6-quick-look")
	a.st.Palette = "my:aurora"
	a.st.Designs = []config.Design{auroraLook}
	a.designChanged()
	a.closeAsking = true
	shot("7-close")
	a.closeAsking = false
	a.st.Onboarded = false
	shot("8-welcome")
	a.st.Onboarded = true
	a.st.Palette = "paper"
	a.themeCache = nil
	shot("9-paper")
	// Each ready-made look, on the window, a terminal printing in color.
	out = "\x1b[34m~/my-app>\x1b[0m npm run dev\r\n  \x1b[32mready\x1b[0m in 312 ms  \x1b[36mhttp://localhost:5173/\x1b[0m\r\n\x1b[32m✓ 3 passed\x1b[0m  \x1b[33m⚠ 1 warning\x1b[0m  \x1b[31m✗ 0 failed\x1b[0m\r\n\x1b[35mgit\x1b[0m \x1b[1mmain\x1b[0m \x1b[90m3 ahead\x1b[0m\r\n\x1b[1;36m●\x1b[0m 好的，我先看看 index.html\r\n> "
	a.openTab(proj, a.defaultShell())
	for _, p := range quickPresets {
		a.st.Palette = "look:" + p.id
		a.applyColors()
		shot("10-look-" + p.id)
	}
}
