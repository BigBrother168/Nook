package checkpoint

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// data is n bytes that are not text.
func data(n int, b byte) string { return strings.Repeat("\x00"+string(b), n/2) }

// A project with a dataset: the pictures and data stay out of the copy,
// the code and the large text in; what the AI does to the data shows, to
// be known and not undone.
func TestDataLeftOut(t *testing.T) {
	r, proj := setup(t)
	bigData = 100
	defer func() { bigData = 1 << 20 }()
	write(t, proj, "data/0001.jpg", "small picture")              // small: kept
	write(t, proj, "data/0002.jpg", strings.Repeat("x", 200))     // a picture by name: out
	write(t, proj, "data/points.xyz", data(200, 'a'))             // unknown kind, not text: out
	write(t, proj, "data/notes.csv", strings.Repeat("1,2\n", 60)) // large text: kept
	cp, _, err := r.Snapshot("first")
	if err != nil {
		t.Fatal(err)
	}
	if old, _ := r.Content(cp.ID, "data/notes.csv"); old == "" {
		t.Fatal("large text left out")
	}
	if old, _ := r.Content(cp.ID, "data/0001.jpg"); old != "small picture" {
		t.Fatal("small picture left out")
	}
	if old, _ := r.Content(cp.ID, "data/0002.jpg"); old != "" {
		t.Fatal("large picture kept")
	}

	// The AI's turn: code changed; a picture rewritten, data deleted, data made.
	write(t, proj, "main.go", "package main\n\nfunc main() {}\n")
	write(t, proj, "data/0002.jpg", strings.Repeat("y", 300))
	os.Remove(filepath.Join(proj, "data", "points.xyz"))
	write(t, proj, "data/out.bin", data(400, 'b'))
	changes, err := r.Changes(cp.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []Change{
		{Path: "main.go", Status: 'M', Plus: 2},
		{Path: "data/0002.jpg", Status: 'M', Binary: true, Unsaved: true},
		{Path: "data/out.bin", Status: 'A', Binary: true, Unsaved: true},
		{Path: "data/points.xyz", Status: 'D', Binary: true, Unsaved: true},
	}
	if !slices.Equal(changes, want) {
		t.Fatalf("changes = %+v\nwant %+v", changes, want)
	}

	// Not to undo: nothing to put back, and the file stays.
	if _, err := r.RestoreFile(cp.ID, "data/0002.jpg", ""); !errors.Is(err, errUnsaved) {
		t.Fatalf("undoing a file left out: %v", err)
	}
	if got := read(t, proj, "data/0002.jpg"); got != strings.Repeat("y", 300) {
		t.Fatal("a file left out was touched")
	}

	// Known: it goes, until it changes again.
	if err := r.KeepFile("data/0002.jpg"); err != nil {
		t.Fatal(err)
	}
	if err := r.KeepFile("data/points.xyz"); err != nil {
		t.Fatal(err)
	}
	changes, _ = r.Changes(cp.ID)
	if len(changes) != 2 || changes[0].Path != "main.go" || changes[1].Path != "data/out.bin" {
		t.Fatalf("after knowing: %+v", changes)
	}
	write(t, proj, "data/0002.jpg", strings.Repeat("z", 500))
	changes, _ = r.Changes(cp.ID)
	if !slices.ContainsFunc(changes, func(c Change) bool { return c.Path == "data/0002.jpg" && c.Unsaved }) {
		t.Fatalf("changed again, not shown: %+v", changes)
	}

	// Undoing all puts the code back and leaves the data as it is.
	if _, err := r.Restore(cp.ID, ""); err != nil {
		t.Fatal(err)
	}
	if got := read(t, proj, "main.go"); got != "package main\n" {
		t.Fatalf("main.go = %q", got)
	}
	if got := read(t, proj, "data/out.bin"); got != data(400, 'b') {
		t.Fatal("data the AI made was deleted, with no copy")
	}
}

// A first copy over the most is not made, and the files stay as they are.
func TestTooMuch(t *testing.T) {
	r, proj := setup(t)
	maxTotal = 10
	defer func() { maxTotal = 1 << 30 }()
	write(t, proj, "a.txt", "more than ten bytes")
	_, _, err := r.Snapshot("first")
	var tm TooMuch
	if !errors.As(err, &tm) || tm.Size < 20 {
		t.Fatalf("err = %v", err)
	}
	if cps, _ := r.List(1); len(cps) != 0 {
		t.Fatal("a copy was made")
	}
	if got := read(t, proj, "a.txt"); got != "more than ten bytes" {
		t.Fatal("files touched")
	}
}
