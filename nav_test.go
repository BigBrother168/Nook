package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"nook/core/config"
)

// twoProjects is an app with projects A and B, two terminals in A and one
// in B, B's in use.
func twoProjects(t *testing.T) (a *app, pa, pb string) {
	a = testApp()
	pa, pb = t.TempDir(), t.TempDir()
	a.st.Projects = []config.Project{{Name: "A", Path: pa}, {Name: "B", Path: pb}}
	sh := a.defaultShell()
	a.openTab(pa, sh)
	a.openTab(pa, sh)
	a.openTab(pb, sh)
	return a, pa, pb
}

// The tab bar shows the terminals of the project in use; choosing a
// project shows its own, at the tab it showed last.
func TestTabsFollowProject(t *testing.T) {
	a, pa, pb := twoProjects(t)
	if a.group != pb || len(a.shownTabs()) != 1 {
		t.Fatalf("in B: group %q, %d tabs shown", a.group, len(a.shownTabs()))
	}
	a.goProject(pa)
	if a.group != pa || len(a.shownTabs()) != 2 || a.focused() == nil || a.focused().proj != pa {
		t.Fatalf("in A: group %q, %d tabs shown, focused %v", a.group, len(a.shownTabs()), a.focused())
	}
	// The last chosen of A comes back after B.
	first := a.shownTabs()[0]
	a.choose(first)
	a.goProject(pb)
	a.goProject(pa)
	if a.current() != first {
		t.Fatal("A did not come back to its tab")
	}
	// Ctrl+Tab goes round A's tabs only.
	a.cycle(1)
	a.cycle(1)
	if a.current() != first || a.group != pa {
		t.Fatal("cycling left the project")
	}
	// What shows beside the terminals is the project's.
	a.viewing = "file:x"
	a.goProject(pb)
	if a.viewing != "" {
		t.Fatalf("B shows %q", a.viewing)
	}
	a.goProject(pa)
	if a.viewing != "file:x" {
		t.Fatalf("A shows %q", a.viewing)
	}
}

// A project without terminals shows its card; choosing it opens nothing.
func TestProjectOpensNothing(t *testing.T) {
	a := testApp()
	// Terminals the view can draw.
	start := a.start
	a.start = func(proj, dir string, sh shell) (*session, error) {
		s, _ := start(proj, dir, sh)
		term, err := terminal.New(terminal.Options{Conn: newFake("")})
		s.term = term
		return s, err
	}
	p := t.TempDir()
	a.st.Projects = []config.Project{{Name: "Empty", Path: p}}
	a.goProject(p)
	if len(a.tabs) != 0 || a.group != p {
		t.Fatalf("%d tabs, group %q", len(a.tabs), a.group)
	}
	tt := ui.NewTester(a.view, 1100, 700)
	tt.Frame()
	if !tt.HasText("Empty") || !tt.HasText("新建终端") {
		t.Fatalf("no card: %q", tt.Texts())
	}
	// The card's button, not the tab bar's +, also named so.
	clicked := false
	for _, e := range tt.AccessAll() {
		if e.Name == "新建终端" && e.Bounds.Y > 100 {
			tt.ClickAt(e.Bounds.X+e.Bounds.W/2, e.Bounds.Y+e.Bounds.H/2)
			clicked = true
			break
		}
	}
	if !clicked {
		t.Fatal("no button on the card")
	}
	if len(a.tabs) != 1 || a.focused().proj != p {
		t.Fatal("the card did not open a terminal")
	}
}

// The tab's × puts it away: it runs on, listed in the sidebar, and comes
// back from there.
func TestHideTab(t *testing.T) {
	a, pa, _ := twoProjects(t)
	a.goProject(pa)
	tb := a.current()
	s := tb.focus
	a.hideTab(tb)
	if !tb.hidden || slices.Contains(a.shownTabs(), tb) || s.closed {
		t.Fatal("not put away, or ended")
	}
	if a.current() == nil || a.current() == tb {
		t.Fatal("no other tab chosen")
	}
	if !a.st.ToldHide || len(a.toasts) != 1 {
		t.Fatal("putting away was not explained once")
	}
	a.show(s)
	if tb.hidden || a.current() != tb {
		t.Fatal("the sidebar did not bring it back")
	}
}

// Ending a terminal takes it out at once; Undo puts it back where it
// was, in its split too; the time over, it ends.
func TestEndAndUndo(t *testing.T) {
	a, pa, _ := twoProjects(t)
	a.goProject(pa)
	s := a.focused()
	a.endSession(s)
	if a.tabOf(s) != nil || s.closed || len(a.endings) != 1 || len(a.toasts) != 1 {
		t.Fatal("not taken out, or ended at once")
	}
	a.toasts[0].undo()
	if tb := a.tabOf(s); tb == nil || a.focused() != s {
		t.Fatal("undo did not bring it back")
	}
	// In a split: back beside the same terminal.
	a.split(false)
	right := a.focused()
	left := a.current().root.a.s
	a.endSession(left)
	if got := a.current().root.leaves(); len(got) != 1 || got[0] != right {
		t.Fatalf("split left %v", got)
	}
	a.undoEnd(a.endings[0])
	if got := a.current().root.leaves(); len(got) != 2 || got[0] != left || got[1] != right {
		t.Fatalf("split back as %v", got)
	}
	// Over: it ends.
	a.endSession(right)
	a.finishEnd(a.endings[0])
	if !right.closed || a.tabOf(right) != nil || len(a.endings) != 0 {
		t.Fatal("not ended")
	}
}

// The settings show in a dialog over the window, which keeps what it
// showed; the drawing board shows in it too; closing it, or going to a
// terminal, leaves the window as it was. A click outside closes it.
func TestSettingsGoBack(t *testing.T) {
	a, _, _ := twoProjects(t)
	a.st.Designs = []config.Design{{ID: "d", Name: "Mine", Dark: true, Background: "#101418", Surface: "#182028", Text: "#e0e6ee", Muted: "#8090a0", Border: "#283440", Accent: "#4a90d9"}}
	a.st.Palette = "my:d"
	a.viewing = "file:readme"
	a.openSettings()
	if a.settings != "settings" || a.viewing != "file:readme" {
		t.Fatalf("settings %q over %q", a.settings, a.viewing)
	}
	a.openDesigner(&ui.Context{})
	if a.settings != "designer" || !a.designFrom || a.viewing != "file:readme" {
		t.Fatalf("board: %q from settings %v, over %q", a.settings, a.designFrom, a.viewing)
	}
	a.leavePages()
	if a.settings != "" || a.viewing != "file:readme" {
		t.Fatalf("closed: %q, %q", a.settings, a.viewing)
	}
	// Choosing a terminal closes them too.
	a.openSettings()
	a.show(a.focused())
	if a.settings != "" || a.viewing != "file:readme" {
		t.Fatalf("after a terminal: %q, %q", a.settings, a.viewing)
	}
}

// A click outside the settings' dialog closes it; one inside does not.
func TestSettingsClickOutside(t *testing.T) {
	a := testApp()
	a.openSettings()
	tt := ui.NewTester(a.view, 1400, 900)
	tt.Frame()
	if err := tt.Click("外观"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.settings != "settings" || a.settingsTab != 2 {
		t.Fatalf("a click inside: %q, section %d", a.settings, a.settingsTab)
	}
	tt.ClickAt(20, 450)
	tt.Frame()
	if a.settings != "" {
		t.Fatalf("still open after a click outside: %q", a.settings)
	}
}

// Several things stay open beside the terminals, each a tab; closing the
// one showing shows the one before.
func TestViewerItems(t *testing.T) {
	a, _, _ := twoProjects(t)
	a.file, a.viewing = &fileView{path: "a.md", kind: kindMD}, "file:a.md"
	a.trackViewer("file")
	a.viewing = "codex:x"
	a.trackViewer("conv")
	a.file, a.viewing = &fileView{path: "b.md", kind: kindMD}, "file:b.md"
	a.trackViewer("file")
	if len(a.items) != 3 {
		t.Fatalf("%d open", len(a.items))
	}
	a.closeMainView()
	if a.viewing != "codex:x" || len(a.items) != 2 {
		t.Fatalf("after closing: %q, %d open", a.viewing, len(a.items))
	}
	a.showItem(a.items[0])
	if a.viewing != "file:a.md" || a.file.path != "a.md" {
		t.Fatalf("back to %q", a.viewing)
	}
}

// Putting away what shows beside the terminals keeps it open: the tab
// bar's button brings back the one put away, as it was; another project
// keeps its own.
func TestPutAway(t *testing.T) {
	a, pa, _ := twoProjects(t)
	a.file, a.viewing = &fileView{path: "a.md", kind: kindMD}, "file:a.md"
	a.trackViewer("file")
	a.viewing = "codex:x"
	a.trackViewer("conv")
	a.file, a.viewing = &fileView{path: "b.md", kind: kindMD}, "file:b.md"
	a.trackViewer("file")
	a.viewing = "file:a.md" // a.md shows
	a.file = a.items[0].file
	if a.putAway() != nil {
		t.Fatal("a button back while a.md shows")
	}
	a.hideViewer()
	if a.viewerKind() != "" || len(a.items) != 3 {
		t.Fatalf("put away: %q showing, %d open", a.viewerKind(), len(a.items))
	}
	it := a.putAway()
	if it == nil || it.key != "file:a.md" {
		t.Fatalf("the button brings back %v", it)
	}
	// Another project and back: still put away, the same waiting.
	a.setGroup(pa)
	a.setGroup(a.st.Projects[1].Path)
	if it := a.putAway(); it == nil || it.key != "file:a.md" {
		t.Fatalf("after a project and back: %v", it)
	}
	a.showItem(*a.putAway())
	if a.viewing != "file:a.md" || a.file.path != "a.md" || a.putAway() != nil {
		t.Fatalf("back: %q", a.viewing)
	}
}

// Closing the only thing open beside the terminals, from its small tab,
// closes it, and nothing else: the frame building it went on with it
// gone, and Nook ended.
func TestCloseOnlyItem(t *testing.T) {
	a := testApp()
	proj := t.TempDir()
	md := filepath.Join(proj, "README.md")
	os.WriteFile(md, []byte("# Demo\n"), 0o644)
	a.st.Projects = []config.Project{{Name: "p", Path: proj}}
	a.file, a.viewing = &fileView{path: md, kind: kindMD, md: "# Demo\n", done: true, size: 7}, "file:"+md
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if len(a.items) != 1 {
		t.Fatalf("%d open", len(a.items))
	}
	r, ok := tt.Find("关闭")
	if !ok {
		t.Fatal("no × on the small tab")
	}
	tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	tt.Frame()
	tt.Frame()
	if len(a.items) != 0 || a.viewerKind() != "" {
		t.Fatalf("after ×: %d open, %q showing", len(a.items), a.viewerKind())
	}
}
