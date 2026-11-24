package main

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"nook/core/checkpoint"
	"nook/core/config"
	"nook/core/history"
	"nook/core/netguard"
)

// TestShots renders the window in a few states as PNG files, to look at
// the layout: NOOK_SHOTS=<folder> go test -run TestShots .
// The terminals are left out: the test's have no program.
func TestShots(t *testing.T) {
	dir := os.Getenv("NOOK_SHOTS")
	if dir == "" {
		t.Skip("NOOK_SHOTS not set")
	}
	proj := `E:\Demo`
	setup := func(palette string) *app {
		a := testApp()
		a.st.Palette = palette
		a.st.Projects = []config.Project{{Name: "Demo", Path: proj}, {Name: "Website", Path: `E:\Website`}}
		a.st.LastProject = proj
		now := time.Now()
		pc := a.projChanges(proj)
		pc.read = now
		pc.base = checkpoint.Checkpoint{ID: "a1a1a1a1", Time: now.Add(-2 * time.Minute), Label: "AI 改之前"}
		pc.changes = []checkpoint.Change{{Path: "index.html", Status: 'M', Plus: 12, Minus: 3}, {Path: "src/app.js", Status: 'A', Plus: 40}, {Path: "old.css", Status: 'D', Minus: 18}}
		return a
	}
	shot := func(name string, a *app, w, h int, prep func(tt *ui.Tester)) {
		tt := ui.NewTester(a.view, w, h)
		tt.Frame()
		if prep != nil {
			prep(tt)
			tt.Frame()
		}
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	for _, pal := range []string{"", "warm", "paper"} {
		a := setup(pal)
		a.side, a.sideTab = true, 0
		shot("changes-"+pal, a, 1300, 800, nil)
	}
	a := setup("paper")
	a.side, a.sideTab = true, 0
	a.projChanges(proj).changes = nil
	a.projChanges(proj).done, a.projChanges(proj).doneAt = "已保留 ✓", time.Now()
	a.st.SidebarHidden = true
	shot("changes-undo", a, 1300, 800, nil)
	// A real folder for the files tab.
	tmp := t.TempDir()
	for _, f := range []string{"README.md", "index.html", "logo.png", "demo.mp4", "report.docx", "src/app.js", "src/style.css", "data.json", ".env"} {
		p := filepath.Join(tmp, filepath.FromSlash(f))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("# Demo\n\nA **page**.\n"), 0o644)
	}
	a = setup("ocean")
	a.st.Projects[0].Path, a.st.LastProject = tmp, tmp
	a.changes = nil
	a.side, a.sideTab = true, 1
	a.fileRoot(tmp).readDir()
	a.fileRoot(tmp).kids[0].open = true
	md := filepath.Join(tmp, "README.md")
	a.file, a.viewing = &fileView{path: md, kind: kindMD, md: "# Demo\n\nA **page** of the project.\n\n- one\n- two\n", done: true, size: 1234}, "file:"+md
	shot("files", a, 1300, 800, nil)
	doc := filepath.Join(tmp, "report.docx")
	a.file, a.viewing = &fileView{path: doc, kind: kindOffice, done: true, size: 52345, mod: time.Now().Add(-time.Hour)}, "file:"+doc
	shot("files-other", a, 1300, 800, nil)
	// A closed conversation, with no terminal open: it takes the middle.
	for _, folded := range []bool{true, false} {
		a = setup("warm")
		conv := config.Conversation{Ref: history.Ref{Tool: "codex", ID: "c1"}, Project: proj, Dir: proj, Seen: time.Now()}
		a.st.Conversations = []config.Conversation{conv}
		a.convs.titles = map[string]string{conv.Key(): "回应问候"}
		a.convs.texts = map[string]*transcript{conv.Key(): {done: true, shown: shownStep, open: map[int]bool{3: !folded}, msgs: []history.Message{
			{Role: "user", Text: "你好"},
			{Role: "ai", Text: "你好！有什么我可以帮你的？"},
			{Role: "user", Text: "新建一个 a.txt，里面写 hello"},
			{Role: "tool", Text: `$ Get-Item -LiteralPath 'E:\Nook\a.txt' -ErrorAction SilentlyContinue`},
			{Role: "tool", Text: `$ Set-Content -LiteralPath 'E:\Nook\a.txt' -Value hello`},
			{Role: "tool", Text: "改了 a.txt"},
			{Role: "ai", Text: "已新建 [a.txt](E:/Nook/a.txt)，并核对内容为 `hello`。\n\n- 位置：`E:\\Nook`\n- 内容：一行 hello"},
			{Role: "user", Text: "在 pelican.html 里再加一个小标题"},
			{Role: "tool", Text: "改了 pelican.html"},
			{Role: "ai", Text: "已在 [pelican.html](E:/Nook/pelican.html:44) 的播放按钮上方添加小标题「骑行控制」。"},
		}}}
		a.viewing = conv.Key()
		name := "conv"
		if !folded {
			name = "conv-open"
		}
		shot(name, a, 1300, 800, nil)
	}
	a = setup("ocean")
	a.openSettings()
	a.settingsTab = 2
	shot("settings-look", a, 1300, 900, nil)
	a = setup("")
	a.side, a.sideTab = true, 2
	a.st.Projects[0].Previews = []string{"http://127.0.0.1:8765/", "http://localhost:5173/"}
	shot("preview", a, 1300, 800, nil)
	for i, name := range []string{"general", "network", "shortcuts", "about"} {
		a = setup("")
		a.openSettings()
		a.settingsTab = []int{0, 1, 3, 4}[i]
		shot("settings-"+name, a, 1300, 900, nil)
	}
	a = setup("")
	a.st.Onboarded = false
	shot("welcome", a, 1300, 900, nil)
	// The drawing board, with a gradient and see-through panels.
	a = setup("ocean")
	d := a.newDesign(ui.DarkTheme())
	randomColors(d)
	d.Gradient, d.GradientAngle, d.PanelOpacity = "#4a2a6a", 135, 0.7
	d.Sidebar, d.TitleBar = "#2a1f3d", "#3a2547"
	termTheme(d, "Dracula")
	a.openDesigner(nil)
	shot("designer", a, 1300, 900, nil)
	a.viewing = ""
	a.side, a.sideTab = true, 0
	shot("design-window", a, 1300, 800, nil)
	a = setup("ocean")
	a.side, a.sideTab = true, sideNet
	var r netguard.Routes
	r[netguard.ViaProxy] = 5
	app := `C:\Program Files\WindowsApps\OpenAI.Codex_26.1117.4957.0_x64__2p2nqsd0c76g0\app\ChatGPT.exe`
	a.net.others = []otherAI{
		{pid: 1, name: "codex.exe", parent: "ChatGPT.exe", parentPath: app, cmd: "codex.exe app-server --analytics-default-enabled", routes: r},
		{pid: 2, name: "codex.exe", parent: "ChatGPT.exe", parentPath: app, cmd: "codex.exe exec-server --remote https://x", routes: r},
		{pid: 3, name: "claude.exe", parent: "pwsh.exe", routes: r},
	}
	shot("net", a, 1300, 900, nil)
	a.net.others[2].routes[netguard.Direct] = 2
	shot("net-direct", a, 1300, 900, nil)
}
