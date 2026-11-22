package checkpoint

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func setup(t *testing.T) (*Repo, string) {
	t.Helper()
	shadow := t.TempDir()
	Root = func() string { return shadow }
	proj := t.TempDir()
	write(t, proj, "main.go", "package main\n")
	write(t, proj, "node_modules/x/index.js", "big\n")
	r, err := Open(proj)
	if err != nil {
		t.Fatal(err)
	}
	return r, proj
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(name))
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, dir, name string) string {
	b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
	if err != nil {
		return "<none>"
	}
	return string(b)
}

func TestSnapshotChangesRestore(t *testing.T) {
	r, proj := setup(t)
	if _, err := os.Stat(filepath.Join(proj, ".git")); err == nil {
		t.Fatal("the project got a .git")
	}
	cp, made, err := r.Snapshot("first")
	if err != nil || !made {
		t.Fatalf("first snapshot: %v %v", made, err)
	}
	if _, made, _ := r.Snapshot("again"); made {
		t.Fatal("a snapshot of nothing changed")
	}

	// An AI's turn: a file changed, one added, one deleted.
	write(t, proj, "main.go", "package main\n\nfunc main() {}\n")
	write(t, proj, "new.txt", "hi\n")
	write(t, proj, "big.bin", "0123456789")
	maxFile = 5
	defer func() { maxFile = 20 << 20 }()
	changes, err := r.Changes(cp.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []Change{{Path: "main.go", Status: 'M', Plus: 2}, {Path: "new.txt", Status: 'A', Plus: 1}, {Path: "big.bin", Status: 'A', Binary: true, Unsaved: true}}
	if !slices.Equal(changes, want) {
		t.Fatalf("changes = %+v, want %+v (no node_modules; the big file shown, not kept)", changes, want)
	}
	if old, _ := r.Content(cp.ID, "main.go"); old != "package main\n" {
		t.Fatalf("content = %q", old)
	}

	before, err := r.Restore(cp.ID, "撤回前的样子")
	if err != nil {
		t.Fatal(err)
	}
	if got := read(t, proj, "main.go"); got != "package main\n" {
		t.Fatalf("main.go after restore = %q", got)
	}
	if got := read(t, proj, "new.txt"); got != "<none>" {
		t.Fatalf("new.txt after restore = %q", got)
	}
	if got := read(t, proj, "node_modules/x/index.js"); got != "big\n" {
		t.Fatalf("node_modules touched: %q", got)
	}
	// The restore can be undone.
	if _, err := r.Restore(before.ID, "撤回前的样子"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, proj, "new.txt"); got != "hi\n" {
		t.Fatalf("new.txt after undoing = %q", got)
	}
	list, _ := r.List(10)
	if len(list) < 3 {
		t.Fatalf("list = %+v", list)
	}
}

func TestTooBig(t *testing.T) {
	home, _ := os.UserHomeDir()
	if _, err := Open(home); err != ErrTooBig {
		t.Fatalf("home: %v", err)
	}
	if _, err := Open(`C:\`); err != ErrTooBig {
		t.Fatalf("drive: %v", err)
	}
}

// A restore always leaves a snapshot of its own to undo it, even when the
// files did not change since the last one.
func TestRestoreAlwaysKeepsBefore(t *testing.T) {
	r, proj := setup(t)
	first, _, _ := r.Snapshot("first")
	write(t, proj, "new.txt", "hi")
	second, _, _ := r.Snapshot("second") // nothing changes after it
	if _, err := r.Restore(first.ID, "撤回前的样子"); err != nil {
		t.Fatal(err)
	}
	list, _ := r.List(5)
	if len(list) != 3 || list[0].Label != "撤回前的样子" || list[1].ID != second.ID {
		t.Fatalf("list = %+v", list)
	}
	if _, err := r.Restore(list[0].ID, "撤回前的样子"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, proj, "new.txt"); got != "hi" {
		t.Fatalf("new.txt after undoing = %q", got)
	}
}
