package main

import (
	"image/png"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"nook/core/checkpoint"
	"nook/core/config"
	"nook/core/history"
	"nook/core/lang"
	"nook/core/netguard"
)

// fakeConn is a terminal's program that printed out and then waits.
type fakeConn struct {
	r *io.PipeReader
	w *io.PipeWriter
}

func newFake(out string) *fakeConn {
	r, w := io.Pipe()
	go w.Write([]byte(out))
	return &fakeConn{r, w}
}
func (f *fakeConn) Read(b []byte) (int, error)  { return f.r.Read(b) }
func (f *fakeConn) Write(b []byte) (int, error) { return len(b), nil }
func (f *fakeConn) Close() error                { f.w.Close(); return nil }

// TestMainShot draws the main window with terminals, made up, into
// $NOOK_SHOTS: shots to check the layout by, in Chinese, and the README's,
// in Chinese and, named -en, in English. Nothing in them is anyone's: a
// made-up project on D:, made-up conversations and connections.
func TestMainShot(t *testing.T) {
	dir := os.Getenv("NOOK_SHOTS")
	if dir == "" {
		t.Skip()
	}
	defer func() { lang.English = false }()
	for _, en := range []bool{false, true} {
		lang.English = en
		mainShots(t, dir, en)
	}
}

func mainShots(t *testing.T, dir string, en bool) {
	suffix := ""
	if en {
		suffix = "-en"
	}
	a := testApp()
	a.st.Designs = []config.Design{{ID: "blue1", Name: "蓝调", Dark: true, Background: "#0f1729", Surface: "#141e34", Text: "#dce6f8", Muted: "#8494b4", Border: "#24324f", Accent: "#3b82f6", TermBg: "#0f1729", TermFg: "#dce6f8", TitleBar: "#0b1222", Sidebar: "#121b30", TabBar: "#0f1729", Panel: "#121b30", Selection: "#1d3156", TermTheme: "TokyoNight Storm", PanelOpacity: 0.82}}
	a.st.Palette = "my:blue1"
	a.st.SetRequireProxy(true)
	a.requireSel = true // the switch, as Nook sets it from the setting as it starts
	proj := `D:\code\my-app`
	a.st.Projects = []config.Project{{Name: "my-app", Path: proj}, {Name: "website", Path: `D:\code\website`}}
	a.st.LastProject = proj
	a.net.ai = map[int]aiUse{}
	light, dark := a.termThemes()
	add := func(title, status, out, tool, conv, convTitle string) {
		id := len(a.tabs) + 1
		term, err := terminal.New(terminal.Options{Conn: newFake(out), Theme: light, DarkTheme: dark})
		if err != nil {
			t.Fatal(err)
		}
		s := &session{id: id, proj: proj, dir: proj, shell: shell{Name: "PowerShell 7"}, title: title, status: status, term: term}
		if conv != "" {
			s.conv = history.Ref{Tool: tool, ID: conv}
			a.convs.titles = mapSet(a.convs.titles, s.conv.Key(), convTitle)
			a.convs.live = timeSet(a.convs.live, s.conv.Key())
		}
		a.tabs = append(a.tabs, &tab{id: id, root: &pane{id: id, s: s}, focus: s})
		if tool != "" {
			a.net.ai[id] = aiUse{tools: []string{tool}}
		}
	}
	add("my-app", "done", "\x1b[1m>_ OpenAI Codex\x1b[0m (v0.171.0)\r\n   D:\\code\\my-app\r\n\r\n> "+L("你好", "hi")+"\r\n\r\n"+L("你好！有什么可以帮你的？", "Hi! What can I help you with?")+"\r\n", "codex", "c1", L("你好", "Hello"))
	add("my-app", "done", "Antigravity CLI 1.5.14\r\n", "agy", "", "")
	add("✳ Claude Code", "working", "\x1b[38;5;209m Claude Code\x1b[0m v2.1.330\r\n Opus 5.5 · D:\\code\\my-app\r\n\r\n\x1b[7m> "+L("给首页加个小标题", "add a heading to the home page")+" \x1b[0m\r\n\r\n● "+L("好的，我先看看 index.html 的结构。", "Sure, let me look at index.html first.")+"\r\n\r\n\x1b[38;5;209m* Gallivanting… (5s)\x1b[0m\r\n", "claude", "c3", L("首页小标题", "Home page heading"))
	a.cur = 2
	for _, h := range []struct{ id, zh, en string }{{"h1", "新建 a.txt", "Create a.txt"}, {"h2", "整理 README", "Tidy the README"}} {
		conv := config.Conversation{Ref: history.Ref{Tool: "codex", ID: h.id}, Project: proj, Dir: proj, Seen: time.Now().Add(-time.Hour)}
		a.st.Conversations = append(a.st.Conversations, conv)
		a.convs.titles = mapSet(a.convs.titles, conv.Key(), L(h.zh, h.en))
	}
	// Online through the proxy: the AI tools in Nook, and Claude Code in
	// VS Code.
	a.net.proxy = netguard.Proxy{Host: "127.0.0.1", Port: 7890, Source: "系统代理", Listening: true}
	for id, n := range map[int]int{1: 2, 2: 1, 3: 3} {
		u := a.net.ai[id]
		u.routes[netguard.ViaProxy] = n
		a.net.ai[id] = u
	}
	vscode := otherAI{pid: 7316, name: "claude.exe", parent: "Code.exe"}
	vscode.routes[netguard.ViaProxy] = 2
	a.net.others = []otherAI{vscode}

	shotAt := func(name string, w, h int) {
		tt := ui.NewTester(a.view, w, h)
		for i := 0; i < 5; i++ {
			tt.Frame()
			time.Sleep(30 * time.Millisecond)
		}
		f, err := os.Create(filepath.Join(dir, name+suffix+".png"))
		if err != nil {
			t.Fatal(err)
		}
		png.Encode(f, tt.Image())
		f.Close()
	}
	a.side = false
	if !en {
		shotAt("main", 1400, 800)
		a.openPalette()
		shotAt("palette", 1400, 800)
		a.paletteOpen = false
		a.notice = "⚠ my-app 里的 codex 有 2 条连接没走代理，会露出真实 IP"
		shotAt("notice", 1400, 500)
		a.notice = ""
		a.askUndoAll(proj)
		shotAt("confirm", 1400, 800)
		a.confirm = nil
		shotAt("narrow", 900, 700)
		a.side, a.netOpen = true, true
		shotAt("crowded", 1000, 700)
		a.side, a.netOpen = false, false
	}

	// For the README: what the AI changed, beside the terminals.
	a.side, a.sideTab = true, 0
	pc := a.projChanges(proj)
	pc.read = time.Now()
	pc.base = checkpoint.Checkpoint{ID: "a1a1a1a1", Time: time.Now().Add(-2 * time.Minute)}
	pc.changes = []checkpoint.Change{{Path: "index.html", Status: 'M', Plus: 40, Minus: 6}, {Path: "README.md", Status: 'M', Plus: 3}, {Path: "a.txt", Status: 'A', Plus: 1}}
	shotAt("readme-changes", 1400, 800)
	if !en {
		// Large files left out of the copy, changed by the AI; then only
		// those; then a project too large to copy.
		changes, base := pc.changes, pc.base
		pc.changes = append(slices.Clone(changes), checkpoint.Change{Path: "data/IMG_0042.jpg", Status: 'M', Binary: true, Unsaved: true}, checkpoint.Change{Path: "data/raw/IMG_0043.jpg", Status: 'D', Binary: true, Unsaved: true})
		shotAt("changes-unsaved", 1400, 800)
		pc.changes = []checkpoint.Change{{Path: "data/IMG_0042.jpg", Status: 'M', Binary: true, Unsaved: true}}
		shotAt("changes-only-unsaved", 1400, 800)
		pc.changes, pc.base, pc.skip = nil, checkpoint.Checkpoint{}, checkpoint.TooMuch{Size: 3 << 30}
		shotAt("changes-skip", 1400, 800)
		pc.changes, pc.base, pc.skip = changes, base, nil
	}

	// Every AI tool online, all through the proxy.
	a.side, a.netOpen = false, true
	shotAt("readme-net", 1400, 800)

	// Codex in a PowerShell window outside Nook, started without the
	// proxy.
	outside := otherAI{pid: 9120, name: "codex.exe", parent: "pwsh.exe", cmd: "codex"}
	outside.routes[netguard.Direct] = 2
	a.net.others = []otherAI{vscode, outside}
	a.notice = L("⚠ Nook 外面的 AI 有 2 条连接没走代理，会露出真实 IP（点顶栏看是哪个）", "⚠ AI tools outside Nook have 2 connections around the proxy, showing your real IP (see the top bar)")
	shotAt("readme-leak", 1400, 800)
	a.notice = ""
	a.net.others = []otherAI{vscode}

	// The proxy off: claude, typed in a terminal, does not start.
	a.net.proxy.Listening = false
	ai := a.net.ai
	a.net.ai, a.net.others = map[int]aiUse{}, nil
	for id, u := range ai {
		u.routes = netguard.Routes{}
		a.net.ai[id] = u
	}
	const red, gray, reset = "\x1b[31m", "\x1b[90m", "\x1b[0m"
	add("PowerShell", "", "PowerShell 7.5.4\r\nPS D:\\code\\my-app> claude\r\n"+
		red+L("✗ 代理 127.0.0.1:7890 没开", "✗ Proxy 127.0.0.1:7890 is off")+reset+"\r\n"+
		red+L("✗ 已拦下：claude 没有启动，也没有联网", "✗ Stopped: claude did not start and connected nowhere")+reset+"\r\n"+
		gray+L("  开好代理再输入一次就行。在国外不用代理的话，到 Nook 的联网面板里关掉「没有代理时，不让 AI 启动」", "  Turn the proxy on and type it again. Outside China without a proxy, turn off “No proxy, no AI” in Nook's network panel")+reset+"\r\n"+
		"PS D:\\code\\my-app> ", "", "", "")
	a.cur = len(a.tabs) - 1
	shotAt("readme-guard", 1400, 800)
	a.tabs = a.tabs[:len(a.tabs)-1]
	a.cur = 2
	a.netOpen = false
	a.net.ai, a.net.others, a.net.proxy.Listening = ai, []otherAI{vscode}, true

	// Files and previews, of the demo project when it is there (made up,
	// in D:\code\my-app, for these shots only).
	if _, err := os.Stat(filepath.Join(proj, "src", "main.js")); err == nil {
		root := a.fileRoot(proj)
		root.readDir()
		for _, k := range root.kids {
			if k.name == "src" {
				k.open = true
				k.readDir()
			}
		}
		js := filepath.Join(proj, "src", "main.js")
		data, _ := os.ReadFile(js)
		a.file, a.viewing = &fileView{path: js, kind: fileKind(js), code: codeState(js, data), done: true, size: int64(len(data))}, "file:"+js
		a.side, a.sideTab = true, 1
		shotAt("readme-files", 1400, 800)
		a.file, a.viewing = nil, ""

		const green, cyan, bold = "\x1b[32m", "\x1b[36m", "\x1b[1m"
		add("npm run dev", "", "PS D:\\code\\my-app> npm run dev\r\n\r\n> my-app@0.1.0 dev\r\n> vite\r\n\r\n  "+
			green+bold+"VITE"+reset+green+" v6.2.0"+reset+"  ready in 312 ms\r\n\r\n  "+
			green+"➜"+reset+"  "+bold+"Local"+reset+":   "+cyan+"http://localhost:5173/"+reset+"\r\n  "+
			green+"➜"+reset+"  "+bold+"Network"+reset+": use --host to expose\r\n", "", "", "")
		a.cur = len(a.tabs) - 1
		a.addPreview(proj, "http://localhost:4173/")
		a.addPreview(proj, "http://localhost:5173/")
		a.sideTab = 2
		shotAt("readme-preview", 1400, 800)
		a.tabs = a.tabs[:len(a.tabs)-1]
		a.cur = 2
		a.side = false
	}

	// A past conversation, read from the AI tool's own files.
	key := history.Ref{Tool: "codex", ID: "h1"}.Key()
	a.convs.texts = map[string]*transcript{key: {done: true, shown: shownStep, msgs: []history.Message{
		{Role: "user", Text: L("新建一个 a.txt，里面写 hello", "Create a.txt with hello in it")},
		{Role: "tool", Text: "$ Set-Content a.txt hello"},
		{Role: "tool", Text: L("改了 a.txt", "Changed a.txt")},
		{Role: "ai", Text: L("已新建 [a.txt](D:/code/my-app/a.txt)，内容是 `hello`。", "Created [a.txt](D:/code/my-app/a.txt), containing `hello`.")},
		{Role: "user", Text: L("首页再加一个小标题", "Add a heading to the home page too")},
		{Role: "tool", Text: L("改了 index.html", "Changed index.html")},
		{Role: "ai", Text: L("已在页脚上方加了小标题「联系我们」，沿用现有的二级标题样式。", "Added the heading “Contact us” above the footer, in the style of the other second-level headings.")},
	}}}
	a.viewing = key
	shotAt("readme-conv", 1400, 800)
}

func mapSet(m map[string]string, k, v string) map[string]string {
	if m == nil {
		m = map[string]string{}
	}
	m[k] = v
	return m
}

func timeSet(m map[string]time.Time, k string) map[string]time.Time {
	if m == nil {
		m = map[string]time.Time{}
	}
	m[k] = time.Now()
	return m
}
