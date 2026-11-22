package main

import (
	"os"
	"path/filepath"
	"testing"

	"nook/core/checkpoint"
)

// Changes add up over the AI's turns until kept or undone, file by file
// or all at once; keeping or undoing leaves no copy behind. This is what
// aiStarts, keepFile, askFileBack, keepAll and askUndoAll do.
func TestKeepOrUndo(t *testing.T) {
	shadow := t.TempDir()
	checkpoint.Root = func() string { return shadow }
	proj := t.TempDir()
	write := func(name, s string) { os.WriteFile(filepath.Join(proj, name), []byte(s), 0o644) }
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(proj, name))
		if err != nil {
			return "<none>"
		}
		return string(b)
	}
	open := func() *checkpoint.Repo {
		r, err := checkpoint.Open(proj)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	// pending is what the card shows: the base, and the files changed.
	pending := func() (checkpoint.Checkpoint, []checkpoint.Change) {
		r := open()
		cps, _ := r.List(1)
		if len(cps) == 0 {
			return checkpoint.Checkpoint{}, nil
		}
		ch, err := r.Changes(cps[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		return cps[0], ch
	}
	// starts is aiStarts: a copy, unless one is kept already.
	starts := func() {
		r := open()
		if cps, _ := r.List(1); len(cps) == 0 {
			r.Snapshot("AI 改之前")
		}
	}

	write("a.txt", "a")
	starts()
	write("a.txt", "a2") // the first turn
	starts()             // the second turn: no new copy
	write("b.txt", "b")
	base, ch := pending()
	if len(ch) != 2 {
		t.Fatalf("two turns: %+v", ch)
	}

	// Keep one file, undo the other: nothing left, nothing kept behind.
	if err := open().KeepFile("a.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := open().RestoreFile(base.ID, "b.txt", ""); err != nil {
		t.Fatal(err)
	}
	if _, ch := pending(); len(ch) != 0 || read("a.txt") != "a2" || read("b.txt") != "<none>" {
		t.Fatalf("after keeping a and undoing b: %+v", ch)
	}
	open().Clear()
	if b, _ := pending(); b.ID != "" {
		t.Fatal("a copy left behind")
	}

	// Undo all: the files as they were, and no copy.
	starts()
	write("a.txt", "a3")
	write("c.txt", "c")
	base, _ = pending()
	if _, err := open().Restore(base.ID, ""); err != nil {
		t.Fatal(err)
	}
	open().Clear()
	if read("a.txt") != "a2" || read("c.txt") != "<none>" {
		t.Fatal("not undone")
	}
	if b, _ := pending(); b.ID != "" {
		t.Fatal("a copy left behind")
	}
}
