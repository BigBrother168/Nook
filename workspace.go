package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"nook/core/config"
)

// pane is a node of a tab's layout: a terminal, or two panes split.
type pane struct {
	id       int
	s        *session // a leaf's terminal
	vertical bool     // a split's direction: the first above the second
	a, b     *pane
	ratio    float32 // the first's share, in percent
}

// tab is a row of the tab bar: one terminal or several, split.
type tab struct {
	id     int
	root   *pane
	focus  *session // the terminal last focused
	hidden bool     // put away: off the tab bar, still running, dim in the sidebar
}

// leaves returns the tab's terminals, in order.
func (p *pane) leaves() []*session {
	if p == nil {
		return nil
	}
	if p.s != nil {
		return []*session{p.s}
	}
	return append(p.a.leaves(), p.b.leaves()...)
}

// find returns the leaf showing s and its parent (nil for the root).
func (p *pane) find(s *session, parent *pane) (leaf, par *pane) {
	if p == nil {
		return nil, nil
	}
	if p.s == s {
		return p, parent
	}
	if l, pp := p.a.find(s, p); l != nil {
		return l, pp
	}
	return p.b.find(s, p)
}

func (a *app) newPane(s *session) *pane {
	a.nextID++
	return &pane{id: a.nextID, s: s}
}

// sessions returns every open terminal, tab by tab.
func (a *app) sessions() []*session {
	var out []*session
	for _, t := range a.tabs {
		out = append(out, t.root.leaves()...)
	}
	return out
}

// current returns the chosen tab, nil when there is none.
func (a *app) current() *tab {
	if a.cur < 0 || a.cur >= len(a.tabs) {
		return nil
	}
	return a.tabs[a.cur]
}

// The project in use: the tab bar shows its terminals, and what shows
// beside them, the right panel and the title are its. Each project keeps
// the tab chosen last and what showed beside its terminals, which come
// back as it is chosen again.

// tabGroup is the sidebar group of tab tb: its project, or "" for
// terminals of no project listed.
func (a *app) tabGroup(tb *tab) string { return a.sideGroup(tabProject(tb)) }

// shownTabs returns the tabs the tab bar shows: those of the project in
// use, not put away.
func (a *app) shownTabs() []*tab {
	var out []*tab
	for _, tb := range a.tabs {
		if !tb.hidden && a.tabGroup(tb) == a.group {
			out = append(out, tb)
		}
	}
	return out
}

// choose makes tb the tab in use, its project the one in use, and gives
// its terminal the focus; a tab put away comes back.
func (a *app) choose(tb *tab) {
	if !slices.Contains(a.tabs, tb) {
		return
	}
	tb.hidden = false
	a.setGroup(a.tabGroup(tb))
	a.cur = slices.Index(a.tabs, tb)
	a.remember(tb)
	if tb.focus != nil {
		tb.focus.unread = false
		a.picked = termKey(tb.focus)
	}
	a.pendingFocus = tb.focus
}

// remember keeps tb as the tab its project shows when chosen again.
func (a *app) remember(tb *tab) {
	if a.lastTab == nil {
		a.lastTab = map[string]*tab{}
	}
	a.lastTab[a.tabGroup(tb)] = tb
}

// setGroup makes g the project in use: what showed beside the terminals
// of the one before is kept, and g's comes back, with the tab it showed.
func (a *app) setGroup(g string) {
	if g == a.group {
		return
	}
	a.stashView()
	a.group = g
	if g != "" && g != a.st.LastProject {
		a.st.LastProject = g
		a.save()
	}
	a.restoreView()
	a.cur = -1
	if tb := a.lastTab[g]; tb != nil && !tb.hidden && slices.Contains(a.tabs, tb) && a.tabGroup(tb) == g {
		a.cur = slices.Index(a.tabs, tb)
	} else if shown := a.shownTabs(); len(shown) > 0 {
		a.cur = slices.Index(a.tabs, shown[0])
	}
	if c := a.current(); c != nil {
		a.pendingFocus = c.focus
	}
}

// fixCurrent keeps the tab in use one the tab bar shows, as projects are
// added or removed under it.
func (a *app) fixCurrent() {
	if c := a.current(); c != nil && !c.hidden && a.tabGroup(c) == a.group {
		return
	}
	a.cur = -1
	if shown := a.shownTabs(); len(shown) > 0 {
		a.cur = slices.Index(a.tabs, shown[0])
	}
}

// viewState is what a project showed beside its terminals.
type viewState struct {
	items   []viewItem
	viewing string
	file    *fileView
	diff    *diffView
	max     bool
	hidden  string
}

// stashView keeps what shows beside the terminals, as the project in use
// changes.
func (a *app) stashView() {
	if a.views == nil {
		a.views = map[string]viewState{}
	}
	a.leavePages()
	a.views[a.group] = viewState{a.items, a.viewing, a.file, a.diff, a.viewerMax, a.hidden}
}

// restoreView shows again what the project in use showed beside its
// terminals.
func (a *app) restoreView() {
	v := a.views[a.group]
	a.items, a.viewing, a.file, a.diff, a.viewerMax, a.hidden = v.items, v.viewing, v.file, v.diff, v.max, v.hidden
}

// neighbor chooses, in the place of tab gone, which the tab bar showed at
// index i, the tab now there or the one before; none when no tab of the
// project shows.
func (a *app) neighbor(i int) {
	a.cur = -1
	shown := a.shownTabs()
	if len(shown) == 0 {
		return
	}
	tb := shown[min(max(i, 0), len(shown)-1)]
	a.cur = slices.Index(a.tabs, tb)
	a.remember(tb)
	a.pendingFocus = tb.focus
	if tb.focus != nil && a.picked != "" {
		a.picked = termKey(tb.focus)
	}
}

// hideTab puts tb away: off the tab bar, its terminals running on, dim in
// the sidebar, where a click brings it back. The first time, Nook says so.
func (a *app) hideTab(tb *tab) {
	if tb == nil || tb.hidden {
		return
	}
	i := slices.Index(a.shownTabs(), tb)
	wasCur := a.current() == tb
	tb.hidden = true
	if wasCur {
		a.neighbor(i)
	}
	if !a.st.ToldHide {
		a.st.ToldHide = true
		a.save()
		a.toast(L("收起了，终端还在左栏运行", "Put away: it still runs, in the sidebar"), nil)
	}
}

// termKey names terminal s among what the sidebar picks.
func termKey(s *session) string { return "t:" + itoa(s.id) }

// focused returns the terminal with the focus in the chosen tab.
func (a *app) focused() *session {
	if t := a.current(); t != nil {
		return t.focus
	}
	return nil
}

// dirFor returns where a terminal of project proj starts: the project's
// folder, else the user's home.
func dirFor(proj string) string {
	if proj != "" {
		if fi, err := os.Stat(proj); err == nil && fi.IsDir() {
			return proj
		}
	}
	home, _ := os.UserHomeDir()
	return home
}

// openTab opens a terminal running sh in a new tab, for project proj.
func (a *app) openTab(proj string, sh shell) {
	a.openTabIn(proj, dirFor(proj), sh)
}

// openTabIn opens a terminal running sh in dir in a new tab, for project
// proj; nil when it could not.
func (a *app) openTabIn(proj, dir string, sh shell) *session {
	s, err := a.start(proj, dir, sh)
	if err != nil {
		a.notice = L("无法打开终端：", "Cannot open a terminal: ") + err.Error()
		return nil
	}
	a.nextID++
	t := &tab{id: a.nextID, root: a.newPane(s), focus: s}
	a.tabs = append(a.tabs, t)
	a.leavePages()
	a.choose(t)
	return s
}

// split opens a terminal beside the focused one (below it when vertical),
// in the same project and shell.
func (a *app) split(vertical bool) {
	t := a.current()
	if t == nil || t.focus == nil {
		a.openTab(a.currentProject(), a.defaultShell())
		return
	}
	old := t.focus
	s, err := a.start(old.proj, dirFor(old.proj), old.shell)
	if err != nil {
		a.notice = L("无法打开终端：", "Cannot open a terminal: ") + err.Error()
		return
	}
	leaf, _ := t.root.find(old, nil)
	if leaf == nil {
		return
	}
	// The leaf becomes the split; its terminal moves into the first half.
	first := a.newPane(old)
	*leaf = pane{id: leaf.id, vertical: vertical, a: first, b: a.newPane(s), ratio: 50}
	t.focus = s
	a.pendingFocus = s
}

// closeSession closes a terminal, ending all it runs, and takes its pane
// away: its sibling takes the split's place; a tab left empty goes.
func (a *app) closeSession(s *session) {
	if !s.closed {
		s.closed = true
		a.live.remove(s)
		if s.term != nil {
			// AI tools are asked to end first, as Nook does when it quits.
			if ending := a.aiTerminals([]*session{s}); len(ending) > 0 && !a.quitting {
				t := s.term
				go func() {
					endGently(ending)
					t.Close()
				}()
			} else {
				s.term.Close()
			}
		}
	}
	if !a.quitting {
		a.forget(s)
	}
	a.dropEnding(s)
	a.detach(s)
}

// detached is where a terminal taken out of its tab was: to put it back.
type detached struct {
	tb       *tab     // its tab
	at       int      // the tab's place in a.tabs
	shown    int      // the tab's place in the tab bar, when it was in use
	sibling  *session // in a split: a terminal of the other half
	vertical bool     // the split's direction
	first    bool     // s was the first half
	ratio    float32
	cur      bool // its tab was in use
}

// detach takes terminal s out of its tab, which goes when left empty, and
// tells where it was; the terminal runs on.
func (a *app) detach(s *session) (d detached, ok bool) {
	for i, t := range a.tabs {
		leaf, parent := t.root.find(s, nil)
		if leaf == nil {
			continue
		}
		cur := a.current()
		d = detached{tb: t, at: i, shown: slices.Index(a.shownTabs(), t), cur: cur == t}
		if parent == nil {
			a.tabs = slices.Delete(a.tabs, i, i+1)
			if cur == t {
				a.neighbor(d.shown)
			} else if cur != nil {
				a.cur = slices.Index(a.tabs, cur)
			}
			return d, true
		}
		sibling := parent.a
		d.first = sibling == leaf
		if d.first {
			sibling = parent.b
		}
		d.vertical, d.ratio = parent.vertical, parent.ratio
		*parent = *sibling
		d.sibling = parent.leaves()[0]
		if t.focus == s {
			t.focus = d.sibling
			if cur == t {
				a.pendingFocus = t.focus
			}
		}
		return d, true
	}
	return d, false
}

// attach puts terminal s back where detach took it from, as far as that
// is still there: beside the terminal it was split with, else in a tab of
// its own at its tab's place; and shows it.
func (a *app) attach(s *session, d detached) {
	if d.sibling != nil && !d.sibling.closed {
		if t := a.tabOf(d.sibling); t != nil {
			leaf, _ := t.root.find(d.sibling, nil)
			mine, other := a.newPane(s), a.newPane(d.sibling)
			first, second := other, mine
			if d.first {
				first, second = mine, other
			}
			*leaf = pane{id: leaf.id, vertical: d.vertical, a: first, b: second, ratio: d.ratio}
			t.focus = s
			a.choose(t)
			return
		}
	}
	t := d.tb
	if t == nil || slices.Contains(a.tabs, t) || !slices.Equal(t.root.leaves(), []*session{s}) {
		a.nextID++
		t = &tab{id: a.nextID, root: a.newPane(s), focus: s}
	}
	a.tabs = slices.Insert(a.tabs, min(d.at, len(a.tabs)), t)
	if c := a.current(); c != nil {
		a.cur = slices.Index(a.tabs, c)
	}
	a.choose(t)
}

// show makes s's tab the chosen one, its project the one in use, and gives
// s the focus; a page over the window (the settings) gives way.
func (a *app) show(s *session) {
	tb := a.tabOf(s)
	if tb == nil {
		return
	}
	a.leavePages()
	a.viewerMax = false // the terminal shows, not only a file beside it
	tb.focus = s
	a.choose(tb)
}

// cycle chooses the next tab of the tab bar (prev for the one before).
func (a *app) cycle(step int) {
	shown := a.shownTabs()
	if len(shown) == 0 {
		return
	}
	i := slices.Index(shown, a.current())
	if i < 0 {
		i = 0
	} else {
		i = (i + step + len(shown)) % len(shown)
	}
	a.choose(shown[i])
}

// goProject makes the project at path the one in use: the tab it showed
// last, or, with no terminal open there, the project's card, which opens
// one. Nothing starts unasked.
func (a *app) goProject(path string) {
	a.leavePages()
	a.setGroup(a.sideGroup(path))
	if c := a.current(); c != nil {
		a.choose(c)
	} else {
		a.picked = ""
	}
}

// addProject adds folder dir to the projects, once, and opens a terminal there.
func (a *app) addProject(dir string) {
	dir = filepath.Clean(dir)
	for _, p := range a.st.Projects {
		if strings.EqualFold(p.Path, dir) {
			a.goProject(p.Path)
			return
		}
	}
	name := filepath.Base(dir)
	if name == "." || name == `\` || name == "" {
		name = dir
	}
	a.st.Projects = append(a.st.Projects, config.Project{Name: name, Path: dir})
	a.save()
	a.openTab(dir, a.defaultShell())
}

// removeProject takes a project off the list; its open terminals stay.
func (a *app) removeProject(path string) {
	a.st.Projects = slices.DeleteFunc(a.st.Projects, func(p config.Project) bool { return p.Path == path })
	if a.st.LastProject == path {
		a.st.LastProject = ""
	}
	a.save()
	// Its terminals join those of no project.
	if a.group == path {
		a.setGroup("")
	}
}

func (a *app) defaultShell() shell { return shellByID(a.shells, a.st.DefaultShell) }

func (a *app) setDefaultShell(id string) {
	a.st.DefaultShell = id
	a.save()
}

// projectName returns the name of the project at path.
func (a *app) projectName(path string) string {
	for _, p := range a.st.Projects {
		if p.Path == path {
			return p.Name
		}
	}
	return filepath.Base(path)
}

// save writes the settings, unless they are a test's.
func (a *app) save() {
	if a.noSave {
		return
	}
	if err := a.st.Save(); err != nil {
		a.notice = L("无法保存设置：", "Cannot save the settings: ") + err.Error()
	}
}

// tabOf returns the tab s is in, nil for none.
func (a *app) tabOf(s *session) *tab {
	for _, tb := range a.tabs {
		if slices.Contains(tb.root.leaves(), s) {
			return tb
		}
	}
	return nil
}

// moveTab puts tab from where tab to is, as a terminal dragged in the
// sidebar or a tab in the tab bar; the tab in use stays in use.
func (a *app) moveTab(from, to *tab) {
	i, j := slices.Index(a.tabs, from), slices.Index(a.tabs, to)
	if i < 0 || j < 0 || i == j {
		return
	}
	cur := a.current()
	a.tabs = slices.Insert(slices.Delete(a.tabs, i, i+1), j, from)
	if cur != nil {
		a.cur = slices.Index(a.tabs, cur)
	}
}

// dragTarget makes row, which shows tab to, take a tab dragged onto it
// that accept takes (nil: any), moving it there, and shows where it
// would go while one is over it.
func (a *app) dragTarget(c *ui.Context, row ui.Element, to *tab, accept func(*tab) bool) {
	if to == nil {
		return
	}
	takes := func(from *tab) bool { return from != nil && from != to && (accept == nil || accept(from)) }
	if from, ok := ui.DragOver[*tab](row); ok && takes(from) {
		row.Background(dropColor(c))
	}
	if from, ok := ui.Drop[*tab](row); ok && takes(from) {
		a.afterFrame(func() { a.moveTab(from, to) })
	}
}

// dropColor is the background of a row a dragged one would go to.
func dropColor(c *ui.Context) ui.Color { return c.Theme().Accent.Alpha(0.18) }

// afterFrame runs fn as the next frame starts, not while the lists it
// changes are being built, and asks for that frame.
func (a *app) afterFrame(fn func()) {
	a.later = append(a.later, fn)
	a.update(func() {})
}

// runLater runs what afterFrame put off.
func (a *app) runLater() {
	fns := a.later
	a.later = nil
	for _, fn := range fns {
		fn()
	}
}

// tabProject is the project of tab tb's terminals.
func tabProject(tb *tab) string {
	if leaves := tb.root.leaves(); len(leaves) > 0 {
		return leaves[0].proj
	}
	return ""
}

// sideGroup is where the sidebar lists what is of project proj: under
// it, or, for no project listed, "" (Other terminals).
func (a *app) sideGroup(proj string) string {
	if proj != "" && a.hasProject(proj) {
		return proj
	}
	return ""
}
