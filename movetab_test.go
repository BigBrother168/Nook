package main

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"nook/core/config"
	"nook/core/history"
)

func TestMoveTab(t *testing.T) {
	a := testApp()
	t1, t2, t3 := &tab{id: 1}, &tab{id: 2}, &tab{id: 3}
	a.tabs, a.cur = []*tab{t1, t2, t3}, 1 // t2 in use
	order := func() string {
		s := ""
		for _, tb := range a.tabs {
			s += itoa(tb.id)
		}
		return s
	}
	a.moveTab(t3, t1) // the last dragged to the top
	if got := order(); got != "312" || a.tabs[a.cur] != t2 {
		t.Errorf("up: %s, in use %d", got, a.tabs[a.cur].id)
	}
	a.moveTab(t3, t2) // dropped on the last: it goes last
	if got := order(); got != "123" || a.tabs[a.cur] != t2 {
		t.Errorf("down: %s, in use %d", got, a.tabs[a.cur].id)
	}
	a.moveTab(t1, t1)
	if got := order(); got != "123" {
		t.Errorf("onto itself: %s", got)
	}
}

// sidebarTester shows a's sidebar as Nook does, what was dropped moved
// as each frame starts.
func sidebarTester(a *app) *ui.Tester {
	tt := ui.NewTester(func(c *ui.Context) {
		a.runLater()
		a.sidebar(c)
	}, 260, 600)
	tt.Frame()
	return tt
}

// drag presses the row showing from, moves in steps, as a hand does, and
// lets go on the row showing to.
func drag(t *testing.T, tt *ui.Tester, from, to string) {
	t.Helper()
	f, ok1 := tt.Find(from)
	g, ok2 := tt.Find(to)
	if !ok1 || !ok2 {
		t.Fatalf("%q or %q not shown: %v", from, to, tt.Texts())
	}
	x, y := f.X+f.W/2, f.Y+f.H/2
	ty := g.Y + g.H/2
	tt.Press(x, y)
	tt.Frame()
	for k := 1; k <= 8; k++ {
		tt.Move(x, y+(ty-y)*float32(k)/8)
		tt.Frame()
	}
	tt.Release(x, ty)
	tt.Frame()
	tt.Frame()
}

// addTerminal opens a terminal titled title in project proj.
func addTerminal(a *app, proj, title string) {
	id := len(a.tabs) + 1
	s := &session{id: id, proj: proj, dir: proj, shell: shell{Name: "PowerShell"}, title: title}
	a.tabs = append(a.tabs, &tab{id: id, root: &pane{id: id, s: s}, focus: s})
}

func tabOrder(a *app) string {
	var out []string
	for _, tb := range a.tabs {
		out = append(out, tb.focus.title)
	}
	return strings.Join(out, " ")
}

// A terminal dragged in the sidebar onto another takes its place, again
// and again.
func TestDragInSidebar(t *testing.T) {
	a := testApp()
	proj := `E:\Nook`
	a.st.Projects = []config.Project{{Name: "Nook", Path: proj}}
	for _, title := range []string{"aaa", "bbb", "ccc"} {
		addTerminal(a, proj, title)
	}
	a.cur = 0
	tt := sidebarTester(a)
	drag(t, tt, "ccc", "aaa")
	if got := tabOrder(a); got != "ccc aaa bbb" {
		t.Errorf("after dragging ccc onto aaa: %s", got)
	}
	drag(t, tt, "bbb", "ccc")
	if got := tabOrder(a); got != "bbb ccc aaa" {
		t.Errorf("after the second drag: %s", got)
	}
	drag(t, tt, "bbb", "aaa") // down, to the bottom
	if got := tabOrder(a); got != "ccc aaa bbb" {
		t.Errorf("after dragging down: %s", got)
	}
}

// A terminal dropped on another project's stays where it was: there it
// would move only in the tab bar.
func TestDragInSidebarStaysInProject(t *testing.T) {
	a := testApp()
	a.st.Projects = []config.Project{{Name: "One", Path: `E:\One`}, {Name: "Two", Path: `E:\Two`}}
	addTerminal(a, `E:\One`, "aaa")
	addTerminal(a, `E:\Two`, "bbb")
	a.cur = 0
	tt := sidebarTester(a)
	drag(t, tt, "bbb", "aaa")
	if got := tabOrder(a); got != "aaa bbb" {
		t.Errorf("dropped on another project's terminal: %s", got)
	}
}

// convsOf lists the conversations' ids, in the order kept.
func convsOf(a *app) string {
	var out []string
	for _, c := range a.st.Conversations {
		out = append(out, c.ID)
	}
	return strings.Join(out, " ")
}

// addConv lists a closed conversation of proj, titled title.
func addConv(a *app, proj, id, title string) {
	conv := config.Conversation{Ref: history.Ref{Tool: "codex", ID: id}, Project: proj, Dir: proj, Seen: time.Now()}
	a.st.Conversations = append(a.st.Conversations, conv)
	if a.convs.titles == nil {
		a.convs.titles = map[string]string{}
	}
	a.convs.titles[conv.Key()] = title
}

// Past conversations are dragged up and down in their history, and keep
// the order.
func TestDragHistory(t *testing.T) {
	a := testApp()
	a.st.Projects = []config.Project{{Name: "One", Path: `E:\One`}, {Name: "Two", Path: `E:\Two`}}
	addConv(a, `E:\One`, "c1", "first talk")
	addConv(a, `E:\Two`, "x1", "other talk")
	addConv(a, `E:\One`, "c2", "second talk")
	addConv(a, `E:\One`, "c3", "third talk")
	tt := sidebarTester(a)
	drag(t, tt, "third talk", "first talk")
	if got := convsOf(a); got != "c3 c1 x1 c2" {
		t.Errorf("third dragged onto the first: %s", got)
	}
	if got := shownOrder(t, tt, "first talk", "second talk", "third talk"); got != "third talk, first talk, second talk" {
		t.Errorf("shown after the first drag: %s", got)
	}
	drag(t, tt, "first talk", "second talk") // down, to the bottom
	if got := convsOf(a); got != "c3 x1 c2 c1" {
		t.Errorf("first dragged onto the last: %s", got)
	}
	if got := shownOrder(t, tt, "first talk", "second talk", "third talk"); got != "third talk, second talk, first talk" {
		t.Errorf("shown after the second drag: %s", got)
	}
	// Another project's conversation stays in its own history.
	drag(t, tt, "other talk", "third talk")
	if got := convsOf(a); got != "c3 x1 c2 c1" {
		t.Errorf("dropped on another project's history: %s", got)
	}
}

// shownOrder lists texts as the sidebar shows them, top to bottom.
func shownOrder(t *testing.T, tt *ui.Tester, texts ...string) string {
	t.Helper()
	ys := map[string]float32{}
	for _, s := range texts {
		r, ok := tt.Find(s)
		if !ok {
			t.Fatalf("%q not shown", s)
		}
		ys[s] = r.Y
	}
	sorted := append([]string{}, texts...)
	for i := range sorted {
		for j := i + 1; j < len(sorted); j++ {
			if ys[sorted[j]] < ys[sorted[i]] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	return strings.Join(sorted, ", ")
}

// A new conversation goes first, so that, closed, it shows at the top.
func TestTrackPutsNewFirst(t *testing.T) {
	a := testApp()
	addConv(a, `E:\One`, "old1", "")
	addConv(a, `E:\One`, "old2", "")
	s := &session{id: 1, proj: `E:\One`, dir: `E:\One`}
	a.track(s, history.Ref{Tool: "claude", ID: "new1"})
	if got := convsOf(a); got != "new1 old1 old2" {
		t.Errorf("tracked: %s", got)
	}
	// The AI starting another conversation in that terminal: it replaces it.
	a.track(s, history.Ref{Tool: "claude", ID: "new2"})
	if got := convsOf(a); got != "new2 old1 old2" {
		t.Errorf("replaced: %s", got)
	}
}

// Nook 1.0.0 kept conversations oldest first and showed them latest
// first: put once in the order shown, then left as the user drags them.
func TestOrderConversations(t *testing.T) {
	now := time.Now()
	st := config.Settings{Conversations: []config.Conversation{
		{Ref: history.Ref{ID: "a"}, Seen: now.Add(-3 * time.Hour)},
		{Ref: history.Ref{ID: "b"}, Seen: now.Add(-2 * time.Hour)},
		{Ref: history.Ref{ID: "c"}, Seen: now.Add(-1 * time.Hour)},
	}}
	ids := func() string {
		var out []string
		for _, c := range st.Conversations {
			out = append(out, c.ID)
		}
		return strings.Join(out, "")
	}
	orderConversations(&st)
	if got := ids(); got != "cba" || !st.ConvOrdered {
		t.Errorf("first run: %s, ordered %v", got, st.ConvOrdered)
	}
	// Dragged: "a" to the top. The next run keeps it there.
	st.Conversations[0], st.Conversations[2] = st.Conversations[2], st.Conversations[0]
	orderConversations(&st)
	if got := ids(); got != "abc" {
		t.Errorf("kept as dragged: %s", got)
	}
}

// Rows that drag still open with a click.
func TestClickStillOpens(t *testing.T) {
	a := testApp()
	proj := `E:\One`
	a.st.Projects = []config.Project{{Name: "One", Path: proj}}
	addTerminal(a, proj, "aaa")
	addTerminal(a, proj, "bbb")
	addConv(a, proj, "c1", "first talk")
	a.cur = 0
	tt := sidebarTester(a)
	click := func(text string) {
		r, ok := tt.Find(text)
		if !ok {
			t.Fatalf("%q not shown", text)
		}
		tt.Press(r.X+r.W/2, r.Y+r.H/2)
		tt.Frame()
		tt.Release(r.X+r.W/2, r.Y+r.H/2)
		tt.Frame()
		tt.Frame()
	}
	click("bbb")
	if a.cur != 1 {
		t.Errorf("clicking bbb: tab %d in use", a.cur)
	}
	click("first talk")
	if a.viewing != "codex:c1" {
		t.Errorf("clicking the conversation: viewing %q", a.viewing)
	}
	if got := tabOrder(a); got != "aaa bbb" {
		t.Errorf("a click moved something: %s", got)
	}
}
