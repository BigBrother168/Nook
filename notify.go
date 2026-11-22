package main

import (
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
)

// Windows notifications: an AI tool that finished, or waits for the user,
// says so while Nook is in the tray or behind other windows. Clicking one
// shows its terminal.

// tellGap is how long a terminal stays quiet after a notification: a
// tool's own notice and its state change say the same thing.
const tellGap = 5 * time.Second

// tell notifies about s, unless the user is looking at Nook.
func (a *app) tell(s *session, body string) {
	if a.win == nil || s.closed || (a.win.IsVisible() && a.win.IsFocused() && !a.win.IsMinimized()) {
		return
	}
	if time.Since(s.told) < tellGap {
		return
	}
	s.told = time.Now()
	mygo.NewNotification(mygo.NotificationOptions{
		ID:    "s:" + strconv.Itoa(s.id),
		Title: a.projectLabel(s) + " · " + a.termLabel(s),
		Body:  body,
		Group: "nook",
	}).Show()
}

// statusChanged tells of a tool that stopped working: done, failed, or
// waiting for the user.
func (a *app) statusChanged(s *session, from, to string) {
	if to == "working" && from != "working" {
		a.aiStarts(s.proj)
	}
	switch {
	case to == "blocked" && from != "blocked":
		a.tell(s, L("AI 在等你确认", "The AI is waiting for you to confirm"))
	case from == "working" && to == "done":
		a.tell(s, L("AI 做完了，等你回复", "The AI is done, waiting for your reply"))
	case from == "working" && to == "error":
		a.tell(s, L("AI 出错了", "The AI ran into an error"))
	}
}

// onNotificationClick shows the terminal a notification is about.
func (a *app) onNotificationClick(id string) {
	n, ok := strings.CutPrefix(id, "s:")
	if !ok {
		return
	}
	a.showWindow()
	if i, err := strconv.Atoi(n); err == nil {
		if s := a.sessionByID(i); s != nil {
			a.show(s)
		}
	}
}
