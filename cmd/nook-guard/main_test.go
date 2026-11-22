package main

import (
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"nook/core/config"
)

func settings(require bool, proxy string) config.Settings {
	s := config.Settings{Proxy: proxy}
	s.SetRequireProxy(require)
	return s
}

func TestCheck(t *testing.T) {
	locked := settings(false, "")
	locked.Locked = true
	if ok, lines, _ := check("claude", locked, nil); ok || !strings.Contains(strings.Join(lines, ""), "联网锁") {
		t.Fatalf("locked: %v %q", ok, lines)
	}
	// A proxy set by hand on a port nothing listens on: stopped when a
	// proxy is required (and the internet goes through no tunnel), let
	// through when not.
	ok, lines, _ := check("claude", settings(true, "127.0.0.1:1"), nil)
	if tunnelsUp() {
		if !ok {
			t.Fatalf("a tunnel carries the internet, yet stopped: %q", lines)
		}
	} else if ok || !strings.Contains(strings.Join(lines, ""), "没开") {
		t.Fatalf("dead proxy, required: %v %q", ok, lines)
	}
	if ok, lines, _ := check("claude", settings(false, "127.0.0.1:1"), nil); !ok {
		t.Fatalf("dead proxy, not required: stopped %q", lines)
	}
}

// TestCheckProxy: the tool goes through the proxy that runs, whenever its
// terminal opened.
func TestCheckProxy(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0") // a proxy, as far as the guard can tell
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	addr := l.Addr().String()
	url := "http://" + addr

	// The terminal has it: the tool starts with the terminal's own.
	env := []string{"PATH=x", "https_proxy=" + url}
	if ok, lines, out := check("claude", settings(true, ""), env); !ok || !slices.Equal(out, env) {
		t.Fatalf("terminal's proxy: %v %q %q", ok, lines, out)
	}
	// The terminal opened before the proxy ran: the tool is given it.
	ok, lines, out := check("claude", settings(true, addr), []string{"PATH=x"})
	if !ok || value(out, "HTTPS_PROXY") != url || value(out, "HTTP_PROXY") != url || value(out, "NO_PROXY") == "" {
		t.Fatalf("proxy found, terminal without: %v %q %q", ok, lines, out)
	}
	// The terminal's proxy is down and another runs: the one that runs.
	_, _, out = check("claude", settings(true, addr), []string{"HTTPS_PROXY=http://127.0.0.1:1", "NO_PROXY=corp"})
	if value(out, "HTTPS_PROXY") != url || value(out, "NO_PROXY") != "corp" || len(out) != 3 {
		t.Fatalf("dead terminal proxy: %q", out)
	}
	// None runs, none required: the dead one is dropped, the tool goes
	// direct.
	_, _, out = check("claude", settings(false, "127.0.0.1:1"), []string{"HTTP_PROXY=http://127.0.0.1:1", "PATH=x"})
	if !slices.Equal(out, []string{"PATH=x"}) {
		t.Fatalf("dead proxy dropped: %q", out)
	}
}

func value(env []string, key string) string {
	for _, kv := range env {
		if k, v, _ := strings.Cut(kv, "="); strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}

func TestFindRealSkipsGuardFolder(t *testing.T) {
	guard, tools := t.TempDir(), t.TempDir()
	for _, p := range []string{filepath.Join(guard, "claude.exe"), filepath.Join(tools, "claude.cmd")} {
		os.WriteFile(p, []byte("x"), 0o644)
	}
	t.Setenv("PATH", guard+string(os.PathListSeparator)+tools)
	t.Setenv("PATHEXT", ".COM;.EXE;.BAT;.CMD")
	if got, want := findReal("claude", guard), filepath.Join(tools, "claude.cmd"); !strings.EqualFold(got, want) {
		t.Fatalf("found %q, want %q", got, want)
	}
	if got := findReal("nothing", guard); got != "" {
		t.Fatalf("found %q for a missing tool", got)
	}
}

func TestNookArgs(t *testing.T) {
	cases := []struct {
		tool string
		in   []string
		want []string
	}{
		{"codex", nil, []string{"--no-daemon"}},
		{"codex", []string{"resume", "abc"}, []string{"--no-daemon", "resume", "abc"}},
		{"codex", []string{"--no-daemon", "resume"}, []string{"--no-daemon", "resume"}},
		{"claude", []string{"--resume", "abc"}, []string{"--resume", "abc"}},
	}
	for _, c := range cases {
		if got := nookArgs(c.tool, c.in); !slices.Equal(got, c.want) {
			t.Errorf("nookArgs(%q, %q) = %q, want %q", c.tool, c.in, got, c.want)
		}
	}
}
