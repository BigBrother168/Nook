package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Going to a terminal from outside Nook: each terminal's programs see
// NOOK_TERMINAL, which names it, and `Nook.exe --show <that>` brings it to
// the front, the Nook started again handing it to the one running. A
// desktop pet whose circles stand for AI conversations goes to them so,
// as it goes to a tab of Windows Terminal by its WT_SESSION.

// terminalTag is what NOOK_TERMINAL says of terminal id: this Nook's
// process, then the terminal's number, which a later Nook reuses.
func terminalTag(id int) string { return fmt.Sprintf("%d-%d", os.Getpid(), id) }

// showArg returns the terminal "--show <tag>" names in args, when it is
// one of this Nook's.
func showArg(args []string) (id int, ok bool) {
	for i, arg := range args {
		tag, found := strings.CutPrefix(arg, "--show=")
		if !found && arg == "--show" && i+1 < len(args) {
			tag, found = args[i+1], true
		}
		if !found {
			continue
		}
		pid, n, cut := strings.Cut(tag, "-")
		if !cut || pid != strconv.Itoa(os.Getpid()) {
			return 0, false // of a Nook that has ended
		}
		id, err := strconv.Atoi(n)
		return id, err == nil
	}
	return 0, false
}

// onSecondInstance: Nook opened again shows its window, and the terminal
// asked for, if any.
func (a *app) onSecondInstance(args []string) {
	a.showWindow()
	if id, ok := showArg(args); ok {
		if s := a.sessionByID(id); s != nil {
			a.show(s)
		}
	}
}
