package main

import (
	"fmt"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"
)

// Ending a terminal leaves a few seconds to take it back: it leaves the
// window at once, and its program ends once the note at the bottom of the
// middle goes, unless “Undo” brought it back first, as it was.

// ending is a terminal ended, its program still running until timer.
type ending struct {
	s     *session
	where detached
	timer *time.Timer
}

// How long an ending can be taken back: longer while the AI in it works.
const (
	endGrace     = 5 * time.Second
	endGraceBusy = 8 * time.Second
)

// endSession ends terminal s: out of sight now, its program ended in a few
// seconds, unless the note's “Undo” brings it back.
func (a *app) endSession(s *session) {
	if s == nil || s.closed || slices.ContainsFunc(a.endings, func(e *ending) bool { return e.s == s }) {
		return
	}
	name := a.termLabel(s)
	busy := s.status == "working" || s.status == "blocked"
	where, ok := a.detach(s)
	if !ok {
		a.closeSession(s)
		return
	}
	e := &ending{s: s, where: where}
	a.endings = append(a.endings, e)
	grace, text := endGrace, fmt.Sprintf(L("已结束「%s」", "Ended “%s”"), name)
	if busy {
		grace, text = endGraceBusy, fmt.Sprintf(L("已结束「%s」，AI 还在干活", "Ended “%s”, its AI still at work"), name)
	}
	a.toastFor(text, grace, func() { a.undoEnd(e) })
	e.timer = time.AfterFunc(grace, func() { a.update(func() { a.finishEnd(e) }) })
}

// undoEnd brings back a terminal ended, where it was.
func (a *app) undoEnd(e *ending) {
	if !slices.Contains(a.endings, e) {
		return
	}
	a.endings = slices.DeleteFunc(a.endings, func(x *ending) bool { return x == e })
	if e.timer != nil {
		e.timer.Stop()
	}
	if !e.s.closed {
		a.attach(e.s, e.where)
	}
}

// finishEnd ends the program of a terminal ended, its time to take it
// back over.
func (a *app) finishEnd(e *ending) {
	if !slices.Contains(a.endings, e) {
		return
	}
	a.endings = slices.DeleteFunc(a.endings, func(x *ending) bool { return x == e })
	a.closeSession(e.s)
}

// dropEnding forgets that s waits to end, as it closes another way.
func (a *app) dropEnding(s *session) {
	a.endings = slices.DeleteFunc(a.endings, func(e *ending) bool {
		if e.s != s {
			return false
		}
		if e.timer != nil {
			e.timer.Stop()
		}
		return true
	})
}

// everySession returns the terminals open, and those ended still running,
// as Nook quits.
func (a *app) everySession() []*session {
	out := a.sessions()
	for _, e := range a.endings {
		out = append(out, e.s)
	}
	return out
}

// toastNote is a short note at the bottom of the middle: what just
// happened, with a way to undo it.
type toastNote struct {
	id   int
	text string
	undo func()
}

// toast shows a note for a few seconds.
func (a *app) toast(text string, undo func()) { a.toastFor(text, 4*time.Second, undo) }

// toastFor shows a note for d; the newest shows.
func (a *app) toastFor(text string, d time.Duration, undo func()) {
	a.nextToast++
	n := &toastNote{id: a.nextToast, text: text, undo: undo}
	a.toasts = append(a.toasts, n)
	time.AfterFunc(d, func() { a.update(func() { a.dropToast(n) }) })
}

func (a *app) dropToast(n *toastNote) {
	a.toasts = slices.DeleteFunc(a.toasts, func(x *toastNote) bool { return x == n })
}

// toastView shows the newest note, over the bottom of the middle.
func (a *app) toastView(c *ui.Context) {
	if len(a.toasts) == 0 {
		return
	}
	n := a.toasts[len(a.toasts)-1]
	t := c.Theme()
	ui.Row(c).Absolute().Left(0).Right(0).Bottom(22).Justify(ui.Center).Children(func() {
		c.Key(n.id)
		note := ui.Row(c).PaddingStart(16).PaddingEnd(6).Height(38).Gap(10).Radius(10).AlignItems(ui.Center).
			Background(t.Surface).Border(1, t.Border).Shadow(0, 6, 20, 0, ui.Hex("#000000").Alpha(0.28)).
			Transition(ui.ElementTransition{Duration: 160 * time.Millisecond, Enter: &ui.Motion{Y: 10}, ExitDuration: -1})
		note.Children(func() {
			ui.Text(c, n.text).FontSize(13).SingleLine().MaxWidth(420)
			if n.undo != nil {
				b := ui.ButtonBase(c).Label(L("撤销", "Undo")).Padding(5, 10).Radius(7)
				if b.Hovered() {
					b.Background(t.SurfaceHover)
				}
				b.Children(func() { ui.Text(c, L("撤销", "Undo")).FontSize(13).Bold().TextColor(t.Accent) })
				if b.Clicked() {
					a.dropToast(n)
					n.undo()
				}
			} else {
				ui.Box(c).Width(10)
			}
		})
	})
}
