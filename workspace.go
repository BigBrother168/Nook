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
	id    int
	root  *pane
	focus *session // the terminal last focused
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
	a.cur = len(a.tabs) - 1
	a.pendingFocus = s
	if proj != a.st.LastProject {
		a.st.LastProject = proj
		a.save()
	}
	return s
}

// split opens a terminal beside the focused one (below it when vertical),
// in the same project and shell.
func (a *app) split(vertical bool) {
	t := a.current()
	if t == nil || t.focus == nil {
		a.openTab(a.st.LastProject, a.defaultShell())
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
	for i, t := range a.tabs {
		leaf, parent := t.root.find(s, nil)
		if leaf == nil {
			continue
		}
		if parent == nil {
			a.tabs = slices.Delete(a.tabs, i, i+1)
			if a.cur >= len(a.tabs) {
				a.cur = len(a.tabs) - 1
			}
			if c := a.current(); c != nil {
				a.pendingFocus = c.focus
			}
			return
		}
		sibling := parent.a
		if sibling == leaf {
			sibling = parent.b
		}
		*parent = *sibling
		if t.focus == s {
			t.focus = parent.leaves()[0]
			if i == a.cur {
				a.pendingFocus = t.focus
			}
		}
		return
	}
}

// show makes s's tab the chosen one and gives s the focus.
func (a *app) show(s *session) {
	for i, t := range a.tabs {
		if leaf, _ := t.root.find(s, nil); leaf != nil {
			if a.viewing == "settings" || a.viewing == "designer" {
				a.viewing = ""
			}
			a.viewerMax = false // the terminal shows, not only a file beside it
			a.cur = i
			t.focus = s
			a.pendingFocus = s
			return
		}
	}
}

// cycle chooses the next tab (prev for the one before).
func (a *app) cycle(step int) {
	if len(a.tabs) == 0 {
		return
	}
	a.cur = (a.cur + step + len(a.tabs)) % len(a.tabs)
	a.pendingFocus = a.tabs[a.cur].focus
}

// addProject adds folder dir to the projects, once, and opens a terminal there.
func (a *app) addProject(dir string) {
	dir = filepath.Clean(dir)
	for _, p := range a.st.Projects {
		if strings.EqualFold(p.Path, dir) {
			a.openTab(p.Path, a.defaultShell())
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
