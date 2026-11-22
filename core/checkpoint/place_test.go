package checkpoint

import (
	"strings"
	"testing"
)

// A project's copy goes on its own drive, or where the user chose; one
// made before stays where it is until it goes.
func TestPlace(t *testing.T) {
	root := t.TempDir()
	Root = func() string { return root }
	defer func() { Dir = "" }()

	// The drive of Root: under Root.
	proj := t.TempDir()
	if got := Place(proj); got != root {
		t.Fatalf("same drive: %s, want %s", got, root)
	}
	// Another drive: its own hidden folder (only the path: no drive is touched).
	if got := driveFolder("E:"); !strings.EqualFold(got, `E:\.nook\backups`) {
		t.Fatalf("drive E: %s", got)
	}
	// A folder chosen.
	Dir = t.TempDir()
	if got := Place(proj); got != Dir {
		t.Fatalf("chosen: %s, want %s", got, Dir)
	}
	// A copy made there stays found though the choice changes.
	r, err := Open(proj)
	if err != nil {
		t.Fatal(err)
	}
	write(t, proj, "a.txt", "a")
	if _, _, err := r.Snapshot("base"); err != nil {
		t.Fatal(err)
	}
	chosen := Dir
	Dir = ""
	if got := Place(proj); got != chosen {
		t.Fatalf("after changing the choice: %s, want %s", got, chosen)
	}
	r.Clear()
	if got := Place(proj); got != root {
		t.Fatalf("after it went: %s", got)
	}
}

// Finding makes nothing; a project with no copy has no repository.
func TestFind(t *testing.T) {
	root := t.TempDir()
	Root = func() string { return root }
	proj := t.TempDir()
	if r, err := Find(proj); r != nil || err != nil {
		t.Fatalf("found %v %v", r, err)
	}
	if got := Place(proj); got != root {
		t.Fatalf("place %s", got)
	}
	r, _ := Open(proj)
	if f, err := Find(proj); f != r || err != nil {
		t.Fatalf("not found after opening: %v %v", f, err)
	}
	r.Clear()
}
