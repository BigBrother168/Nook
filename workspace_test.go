package main

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"

	"nook/core/lang"

	"nook/core/config"
	"nook/core/history"
)

// testApp is an app whose terminals are fakes, without programs.
func testApp() *app {
	a := &app{cur: -1, noSave: true, shells: []shell{{ID: "pwsh", Name: "PowerShell 7"}}}
	a.st.Onboarded = true // no welcome over what the tests click
	a.start = func(proj, dir string, sh shell) (*session, error) {
		a.nextID++
		return &session{id: a.nextID, proj: proj, dir: dir, shell: sh}, nil
	}
	return a
}

// layout writes a tab's layout as text: ids of terminals, splits as
// [a | b] (side by side) or [a / b] (one above the other).
func layout(p *pane) string {
	if p.s != nil {
		return fmt.Sprint(p.s.id)
	}
	sep := " | "
	if p.vertical {
		sep = " / "
	}
	return "[" + layout(p.a) + sep + layout(p.b) + "]"
}

func TestTabsAndSplits(t *testing.T) {
	a := testApp()
	sh := a.defaultShell()

	a.openTab("", sh)
	a.openTab("", sh)
	if len(a.tabs) != 2 || a.cur != 1 {
		t.Fatalf("two tabs: %d tabs, current %d", len(a.tabs), a.cur)
	}
	first := a.tabs[0].focus

	// Split the second tab right, then its new terminal down.
	a.split(false)
	right := a.focused()
	a.split(true)
	below := a.focused()
	tb := a.tabs[1]
	want := fmt.Sprintf("[%d | [%d / %d]]", tb.root.leaves()[0].id, right.id, below.id)
	if got := layout(tb.root); got != want {
		t.Fatalf("layout %s, want %s", got, want)
	}
	if a.pendingFocus != below {
		t.Fatal("the new terminal does not get the focus")
	}

	// Closing the right one leaves the one below in its place, focused.
	a.show(right)
	a.closeSession(right)
	if got, want := layout(tb.root), fmt.Sprintf("[%d | %d]", tb.root.leaves()[0].id, below.id); got != want {
		t.Fatalf("after closing: layout %s, want %s", got, want)
	}
	if tb.focus != below {
		t.Fatalf("focus went to %d, not to %d, which took the closed one's place", tb.focus.id, below.id)
	}
	if !right.closed {
		t.Fatal("the closed terminal is not closed")
	}

	// Closing the rest of the tab removes it; the first tab is chosen.
	for _, s := range tb.root.leaves() {
		a.closeSession(s)
	}
	if len(a.tabs) != 1 || a.cur != 0 || a.focused() != first {
		t.Fatalf("after closing a tab: %d tabs, current %d", len(a.tabs), a.cur)
	}

	// Closing the last one leaves nothing open.
	a.closeSession(first)
	if len(a.tabs) != 0 || a.current() != nil || a.focused() != nil {
		t.Fatalf("after closing all: %d tabs, current %d", len(a.tabs), a.cur)
	}
	// Closing a terminal twice (its program's end after Nook closed it)
	// changes nothing.
	a.closeSession(first)
}

func TestCycleAndShow(t *testing.T) {
	a := testApp()
	sh := a.defaultShell()
	for range 3 {
		a.openTab("", sh)
	}
	a.cycle(1)
	if a.cur != 0 {
		t.Fatalf("next after the last: %d", a.cur)
	}
	a.cycle(-1)
	if a.cur != 2 {
		t.Fatalf("previous before the first: %d", a.cur)
	}
	s := a.tabs[1].focus
	a.show(s)
	if a.cur != 1 || a.pendingFocus != s {
		t.Fatalf("show: current %d", a.cur)
	}
}

func TestProjects(t *testing.T) {
	a := testApp()
	dir := t.TempDir()
	a.addProject(dir)
	a.addProject(dir + `\`) // the same folder: not twice
	if len(a.st.Projects) != 1 || len(a.tabs) != 2 {
		t.Fatalf("%d projects, %d tabs", len(a.st.Projects), len(a.tabs))
	}
	if s := a.focused(); s.proj != a.st.Projects[0].Path || s.dir != dir {
		t.Fatalf("terminal in %q (project %q)", s.dir, s.proj)
	}
	if a.st.LastProject != a.st.Projects[0].Path {
		t.Fatalf("last project %q", a.st.LastProject)
	}
	// Splitting keeps the project.
	a.split(false)
	if a.focused().proj != a.st.Projects[0].Path {
		t.Fatal("split left the project")
	}
	a.removeProject(a.st.Projects[0].Path)
	if len(a.st.Projects) != 0 || a.st.LastProject != "" || len(a.tabs) != 2 {
		t.Fatalf("after removing: %d projects, last %q, %d tabs", len(a.st.Projects), a.st.LastProject, len(a.tabs))
	}
}

// The sidebar keeps the conversations of the terminals open when Nook
// quits; a terminal the user closes takes its conversation off.
func TestConversationsStayListed(t *testing.T) {
	proj := t.TempDir()
	a := testApp()
	a.st.Projects = []config.Project{{Name: "p", Path: proj}}
	sh := a.defaultShell()
	a.openTab(proj, sh)
	s1 := a.focused()
	a.openTab(proj, sh)
	s2 := a.focused()
	r1 := history.Ref{Tool: "claude", ID: "11111111-0000-0000-0000-000000000001"}
	r2 := history.Ref{Tool: "codex", ID: "11111111-0000-0000-0000-000000000002"}
	r3 := history.Ref{Tool: "claude", ID: "11111111-0000-0000-0000-000000000003"}
	a.track(s1, r1)
	a.track(s2, r2)
	if len(a.st.Conversations) != 2 || len(a.closedConvs(proj, false)) != 0 {
		t.Fatalf("two open: %+v, closed %+v", a.st.Conversations, a.closedConvs(proj, false))
	}

	a.closeSession(s2) // the user closes it
	a.track(s1, r3)    // a new conversation in s1, as after /clear
	if len(a.st.Conversations) != 1 || a.st.Conversations[0].Ref != r3 {
		t.Fatalf("after closing s2 and a new conversation in s1: %+v", a.st.Conversations)
	}

	a.quitting = true // Nook quits: s1 ends, its conversation stays
	a.closeSession(s1)
	closed := a.closedConvs(proj, false)
	if len(closed) != 1 || closed[0].Ref != r3 || closed[0].Dir != proj {
		t.Fatalf("after quitting: %+v", closed)
	}

	b := testApp() // the next run
	b.st = a.st
	b.resume(closed[0])
	s := b.focused()
	if s == nil || s.conv != r3 || s.dir != proj || len(b.closedConvs(proj, false)) != 0 {
		t.Fatalf("resumed: %+v, closed %+v", s, b.closedConvs(proj, false))
	}
	b.closeSession(s)
	if len(b.st.Conversations) != 0 {
		t.Fatalf("closed after resuming: %+v", b.st.Conversations)
	}
}

func TestPaletteFinds(t *testing.T) {
	a := testApp()
	a.st.Projects = []config.Project{{Name: "Nook", Path: `E:\Nook`}}
	a.openTab("", a.defaultShell())
	find := func(q string) []string {
		var out []string
		for _, it := range a.paletteItems() {
			if it.matches(q) {
				out = append(out, it.label)
			}
		}
		return out
	}
	if got := find("分屏"); len(got) != 2 {
		t.Errorf("分屏: %q", got)
	}
	if got := find("项目 NOOK"); len(got) != 1 || got[0] != "项目：Nook" {
		t.Errorf("项目 NOOK: %q", got)
	}
	if got := find("终端 PowerShell"); len(got) == 0 {
		t.Errorf("终端 PowerShell: %q", got)
	}
	if got := find("没有这个"); len(got) != 0 {
		t.Errorf("没有这个: %q", got)
	}
}

// The welcome shows on the first run, and goes for good with its button.
func TestWelcome(t *testing.T) {
	a := testApp()
	a.st.Onboarded = false
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if !tt.HasText("欢迎使用 Nook") {
		t.Fatalf("no welcome in %q", tt.Texts())
	}
	if err := tt.Click("开始使用"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.st.Onboarded || tt.HasText("欢迎使用 Nook") {
		t.Fatal("the welcome stays")
	}
}

// Nook speaks English when asked.
func TestEnglish(t *testing.T) {
	defer lang.Set("zh", nil)
	a := testApp()
	a.st.Language = "en"
	lang.Set(a.st.Language, nil)
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if !tt.HasText("Projects") || tt.HasText("项目") {
		t.Fatalf("not English: %q", tt.Texts())
	}
}
