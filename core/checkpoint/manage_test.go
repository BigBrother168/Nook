package checkpoint

import "testing"

// One file goes back; the others stay as they are.
func TestRestoreFile(t *testing.T) {
	r, proj := setup(t)
	cp, _, _ := r.Snapshot("first")
	write(t, proj, "main.go", "changed")
	write(t, proj, "new.txt", "new")
	if _, err := r.RestoreFile(cp.ID, "main.go", ""); err != nil {
		t.Fatal(err)
	}
	if read(t, proj, "main.go") != "package main\n" || read(t, proj, "new.txt") != "new" {
		t.Fatal("main.go not back, or new.txt touched")
	}
	before, err := r.RestoreFile(cp.ID, "new.txt", "before")
	if err != nil || before.ID == "" {
		t.Fatalf("restore with a snapshot: %v %v", before, err)
	}
	if read(t, proj, "new.txt") != "<none>" {
		t.Fatal("new.txt, added since, stays")
	}
	if _, err := r.RestoreFile(cp.ID, "../x", ""); err == nil {
		t.Fatal("a path out of the project")
	}
}

// Undoing a restore takes no snapshot of its own.
func TestUndoRestore(t *testing.T) {
	r, proj := setup(t)
	first, _, _ := r.Snapshot("first")
	write(t, proj, "new.txt", "hi")
	before, err := r.Restore(first.ID, "before")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Restore(before.ID, ""); err != nil {
		t.Fatal(err)
	}
	if read(t, proj, "new.txt") != "hi" {
		t.Fatal("not undone")
	}
	if list, _ := r.List(10); len(list) != 2 {
		t.Fatalf("undo took a snapshot: %+v", list)
	}
	if err := r.Clear(); err != nil {
		t.Fatal(err)
	}
}

// Keeping one file takes it off the changes; the other stays changed,
// and the project's files stay as they are.
func TestKeepFile(t *testing.T) {
	r, proj := setup(t)
	cp, _, _ := r.Snapshot("base")
	write(t, proj, "main.go", "changed")
	write(t, proj, "new.txt", "new")
	if err := r.KeepFile("main.go"); err != nil {
		t.Fatal(err)
	}
	list, _ := r.List(5)
	if len(list) != 1 || list[0].Label != "base" {
		t.Fatalf("list %+v", list)
	}
	ch, _ := r.Changes(list[0].ID)
	if len(ch) != 1 || ch[0].Path != "new.txt" {
		t.Fatalf("changes %+v", ch)
	}
	if read(t, proj, "main.go") != "changed" {
		t.Fatal("main.go touched")
	}
	// Undoing the other puts it back as the snapshot had it: gone.
	if _, err := r.RestoreFile(list[0].ID, "new.txt", ""); err != nil {
		t.Fatal(err)
	}
	if ch, _ := r.Changes(list[0].ID); len(ch) != 0 || read(t, proj, "new.txt") != "<none>" {
		t.Fatalf("after undoing new.txt: %+v", ch)
	}
	_ = cp
}
