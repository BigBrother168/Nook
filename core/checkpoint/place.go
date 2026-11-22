package checkpoint

import (
	"os"
	"path/filepath"
	"strings"
)

// Where a project's copy goes. A project may be large, and the system
// drive small: the copy goes on the project's own drive, in a hidden
// folder at its root (X:\.nook\backups), unless the user chose a folder
// (Dir). A project on the drive of Root, as one in the user's folder,
// has its copy under Root; so does one whose drive takes no folder.

// Dir is the folder the user chose for the copies, "" for the project's
// own drive. Set by the app.
var Dir string

// driveFolder is the folder of the copies on the drive of volume vol.
func driveFolder(vol string) string {
	return filepath.Join(vol+`\`, ".nook", "backups")
}

// Place returns the folder holding the copies of project's files: a
// copy made before keeps its place until it goes.
func Place(project string) string {
	return filepath.Dir(gitDirOf(filepath.Clean(project), false))
}

// gitDirOf returns where project's repository is, or is to be. Where
// one is, a note under Root says, a few bytes, that it is found though
// the choice of folder changes since.
func gitDirOf(project string, make bool) string {
	name := repoName(project)
	if b, err := os.ReadFile(notePath(name)); err == nil {
		if dir := strings.TrimSpace(string(b)); exists(filepath.Join(dir, "HEAD")) {
			return dir
		}
	}
	if old := filepath.Join(Root(), name); exists(filepath.Join(old, "HEAD")) {
		return old // made before the notes
	}
	return filepath.Join(chooseFolder(project, make), name)
}

// chooseFolder is where a new copy of project goes; with make, the
// folder is made (and found to take one), else only named.
func chooseFolder(project string, make bool) string {
	if Dir != "" {
		if !make || os.MkdirAll(Dir, 0o755) == nil {
			return Dir
		}
	}
	vol := filepath.VolumeName(project)
	if vol == "" || strings.EqualFold(vol, filepath.VolumeName(Root())) {
		return Root()
	}
	dir := driveFolder(vol)
	if !make {
		return dir
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Root()
	}
	hide(filepath.Dir(dir))
	return dir
}

// notePath is the note saying where repository name is.
func notePath(name string) string {
	return filepath.Join(Root(), "where", name)
}

// note writes where repository name is; forget removes the note.
func note(name, gitDir string) {
	if os.MkdirAll(filepath.Dir(notePath(name)), 0o755) == nil {
		os.WriteFile(notePath(name), []byte(gitDir), 0o644)
	}
}

func forget(name string) { os.Remove(notePath(name)) }

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Find opens project's repository if there is one, making none: nil
// when no copy of its files is kept.
func Find(project string) (*Repo, error) {
	project = filepath.Clean(project)
	if !exists(filepath.Join(gitDirOf(project, false), "HEAD")) {
		return nil, nil
	}
	return Open(project)
}
