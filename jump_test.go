package main

import (
	"os"
	"strconv"
	"testing"
)

// "--show <tag>" names a terminal of this Nook; one of an ended Nook,
// nothing.
func TestShowArg(t *testing.T) {
	me := strconv.Itoa(os.Getpid())
	cases := []struct {
		args []string
		id   int
		ok   bool
	}{
		{[]string{"--show", terminalTag(7)}, 7, true},
		{[]string{"--show=" + me + "-12"}, 12, true},
		{[]string{"--show", "1-7"}, 0, false},
		{[]string{"--show"}, 0, false},
		{[]string{"--show", me + "-x"}, 0, false},
		{nil, 0, false},
	}
	for _, c := range cases {
		if id, ok := showArg(c.args); id != c.id || ok != c.ok {
			t.Errorf("showArg(%q) = %d, %v; want %d, %v", c.args, id, ok, c.id, c.ok)
		}
	}
}

// Going to a terminal chooses its tab and leaves a page over the
// terminals.
func TestShowTerminal(t *testing.T) {
	a := testApp()
	sh := a.defaultShell()
	a.openTab("", sh)
	first := a.focused()
	a.openTab("", sh)
	a.viewing, a.viewerMax = "designer", true
	a.show(first)
	if a.cur != 0 || a.current().focus != first || a.viewing != "" || a.viewerMax {
		t.Fatalf("cur %d viewing %q max %v", a.cur, a.viewing, a.viewerMax)
	}
}
