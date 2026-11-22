package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"nook/core/history"
	"nook/core/pty"
	"nook/core/stream"
)

// session is one terminal: a shell in a ConPTY Nook owns.
type session struct {
	id          int
	proj        string // the project's path, "" for none
	dir         string // where it started
	shell       shell
	term        *terminal.Terminal
	conn        *tapped
	transparent bool // made see-through, for a look with a background
	find        findState
	handle      ui.Handle

	title  string      // the program's (OSC 0/2)
	status string      // the program's last report (OSC 7501): working, blocked, done, …
	conv   history.Ref // the AI conversation in it, the latest
	told   time.Time   // the last notification about it
	closed bool
}

// label is what tabs and the sidebar show for the session.
func (s *session) label() string {
	t := strings.TrimSpace(s.title)
	// Shells set the title to their program's path: show its name.
	if strings.HasSuffix(strings.ToLower(t), ".exe") && strings.ContainsAny(t, `\/`) {
		t = strings.TrimSuffix(filepath.Base(t), filepath.Ext(t))
	}
	if t == "" {
		t = s.shell.Name
	}
	return t
}

// tapped is a session's ConPTY, whose output the scanner reads on its way
// to the terminal view.
type tapped struct {
	*pty.Session
	sc    *stream.Scanner
	urls  *stream.URLFinder
	raw   *rawLog
	first sync.Once
	onOut func()
	ready chan struct{} // closed at the program's first output
}

func (t *tapped) Read(b []byte) (int, error) {
	n, err := t.Session.Read(b)
	if n > 0 {
		t.first.Do(func() {
			close(t.ready)
			if t.onOut != nil {
				t.onOut()
			}
		})
		t.sc.Feed(b[:n])
		if t.urls != nil {
			t.urls.Feed(b[:n])
		}
		t.raw.dump("out", b[:n])
	}
	return n, err
}

func (t *tapped) Write(b []byte) (int, error) {
	t.raw.dump("in ", b)
	return t.Session.Write(b)
}

// newSession starts sh in dir, for project proj ("" for none).
func (a *app) newSession(proj, dir string, sh shell) (*session, error) {
	a.nextID++
	s := &session{id: a.nextID, proj: proj, dir: dir, shell: sh, transparent: seeThrough(a.currentDesign())}
	conn := &tapped{
		Session: pty.Start(pty.Options{Command: sh.Command, Dir: dir, Env: environment(slices.Concat(sh.Env, a.proxyEnv(), []string{"NOOK_TERMINAL=" + terminalTag(s.id)}), a.shimDir)}),
		sc:      &stream.Scanner{},
		raw:     a.raw,
		ready:   make(chan struct{}),
	}
	if a.nextID == 1 {
		conn.onOut = func() { a.probe.log("first-output %d ms", sinceProcessStart().Milliseconds()) }
	}
	conn.sc.On = func(code, data string) { a.onOSC(s, code, data) }
	conn.urls = &stream.URLFinder{On: func(u string) { a.win.Update(func() { a.addPreview(s.proj, u) }) }}
	s.conn = conn
	light, dark := a.termThemes()
	t, err := terminal.New(terminal.Options{
		Conn:      conn,
		Font:      a.termFont(),
		Theme:     light,
		DarkTheme: dark,
		// The user's look shows its background through the terminals.
		Transparent: s.transparent,
		OnTitle:     func(title string) { a.win.Update(func() { s.title = title }) },
		// The shell ended (exit), or Nook closed it: its pane goes.
		OnExit: func(int) { a.win.Update(func() { a.closeSession(s) }) },
		// Enter, with an AI running, most likely sends it a request: a
		// snapshot first.
		OnKey: func(mods ui.Modifiers, key ui.Key) bool {
			if key == ui.KeyEnter && mods == 0 {
				a.onEnter(s)
			}
			return false
		},
		OnDrop: func(paths []string) bool {
			s.term.Paste(quotePaths(sh.Quote, paths))
			return true
		},
		OnPaste: func(d transfer.Data) bool {
			keys, paste, taken := pasteNoText(d, sh.Quote)
			switch {
			case paste:
				s.term.Paste(keys)
			case taken:
				s.term.Send([]byte(keys))
			}
			return taken
		},
		// The program's own notices (OSC 9, OSC 777).
		OnNotify: func(title, body string) {
			a.probe.log("notify %q %q", title, body)
			if body == "" {
				body = title
			}
			a.win.Update(func() { a.tell(s, body) })
		},
	})
	if err != nil {
		conn.Close()
		return nil, err
	}
	s.term = t
	a.live.add(s)
	return s, nil
}

// onOSC handles what a session's program tells the terminal, on the
// session's reading goroutine.
func (a *app) onOSC(s *session, code, data string) {
	a.probe.log("osc %d %s %q", s.id, code, data)
	if code != "7501" {
		return
	}
	if strings.HasPrefix(data, "?") {
		// The program asks whether the terminal reads its status reports.
		// The terminal (libghostty-vt in MyGo 0.10) does not answer; Nook
		// reads them, so Nook says yes, before the terminal answers the DA1
		// that follows the query.
		s.conn.Write([]byte("\x1b]7501;?\x1b\\"))
		return
	}
	if st, ok := stream.ParseStatus(data); ok {
		state := st.State
		if state == "clear" || state == "idle" {
			state = ""
		}
		a.win.Update(func() {
			from := s.status
			s.status = state
			a.syncTray()
			a.statusChanged(s, from, state)
		})
	}
}

// statusColor is the dot shown for a session's status, ok false for none.
func statusColor(t *ui.Theme, status string) (ui.Color, bool) {
	switch status {
	case "working":
		return t.Accent, true
	case "blocked":
		return t.Warning, true
	case "done":
		return t.Success, true
	case "error":
		return t.Danger, true
	}
	return ui.Color{}, false
}

// statusText is the status in words, for tooltips.
func statusText(status string) string {
	switch status {
	case "working":
		return L("工作中", "Working")
	case "blocked":
		return L("等你确认", "Waiting for you to confirm")
	case "done":
		return L("完成了，等你回复", "Done, waiting for your reply")
	case "error":
		return L("出错了", "Error")
	}
	return ""
}

// inherited are variables that describe the terminal or session Nook was
// started from, not Nook's: programs would take Nook for that terminal
// (WT_SESSION makes Claude Code think it runs in Windows Terminal), or
// for a child of that Claude Code session. Users' own settings, as
// CLAUDE_CODE_USE_BEDROCK, stay.
var inherited = []string{
	"WT_SESSION", "WT_PROFILE_ID", "TERM_PROGRAM", "TERM_PROGRAM_VERSION", "TERMINAL_EMULATOR",
	"TMUX", "TMUX_PANE", "STY", "ZELLIJ", "ZELLIJ_SESSION_NAME", "KITTY_WINDOW_ID", "KITTY_PID",
	"WEZTERM_PANE", "WEZTERM_EXECUTABLE", "ALACRITTY_WINDOW_ID", "ConEmuPID", "VSCODE_INJECTION",
	"CLAUDECODE", "CLAUDE_PID", "CLAUDE_EFFORT", "CLAUDE_CODE_ENTRYPOINT", "CLAUDE_CODE_SESSION_ID",
	"CLAUDE_CODE_CHILD_SESSION", "CLAUDE_CODE_CHILD_NOTES_DIR", "CLAUDE_CODE_SESSION_ATTENDED",
	"CLAUDE_CODE_MESSAGING_SOCKET", "CLAUDE_CODE_MESSAGING_TOKEN", "CLAUDE_CODE_EXECPATH",
	"WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", // the preview's browser's, not the terminals'
}

// environment is Nook's environment, without what belongs to the terminal
// Nook was started from, with the terminal's variables and extra, and
// shims (the guards' folder) first in PATH.
func environment(extra []string, shims string) []string {
	var env []string
	for _, e := range os.Environ() {
		k, _, _ := strings.Cut(e, "=")
		drop := false
		for _, name := range inherited {
			if strings.EqualFold(k, name) {
				drop = true
				break
			}
		}
		if !drop {
			env = append(env, e)
		}
	}
	set := func(kv string) {
		k, _, _ := strings.Cut(kv, "=")
		for i, e := range env {
			if ek, _, _ := strings.Cut(e, "="); strings.EqualFold(ek, k) {
				env[i] = kv
				return
			}
		}
		env = append(env, kv)
	}
	set("TERM=xterm-256color")
	set("COLORTERM=truecolor")
	set("TERM_PROGRAM=Nook")
	set("NOOK=1")
	for _, kv := range extra {
		set(kv)
	}
	if shims != "" {
		for i, e := range env {
			if k, v, _ := strings.Cut(e, "="); strings.EqualFold(k, "PATH") {
				env[i] = k + "=" + shims + string(os.PathListSeparator) + v
			}
		}
	}
	return env
}
