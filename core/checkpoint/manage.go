package checkpoint

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"nook/core/lang"
)

var (
	errNoSuch  = lang.Error{Zh: "找不到改动前的备份", En: "The copy from before the changes is gone"}
	errUnsaved = lang.Error{Zh: "这个大文件没有备份，撤销不了", En: "This large file was not kept: it cannot be undone"}
)

// RestoreFile puts one file back as checkpoint id kept it, removing it
// when it had none. With a label, it first snapshots the project under
// it, as Restore does, and returns that snapshot.
func (r *Repo) RestoreFile(id, path, label string) (Checkpoint, error) {
	if !validID(id) || path == "" || strings.HasPrefix(path, "/") || strings.Contains("/"+path+"/", "/../") {
		return Checkpoint{}, errNoSuch
	}
	var before Checkpoint
	if label != "" {
		var err error
		if before, _, err = r.snapshot(label, true); err != nil {
			return Checkpoint{}, err
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.readSkipped()[path]; ok {
		return before, errUnsaved // not in the copy: there is nothing to put back
	}
	if _, err := r.git(nil, "cat-file", "-e", id+":"+path); err != nil {
		err := os.Remove(filepath.Join(r.Project, filepath.FromSlash(path)))
		if errors.Is(err, os.ErrNotExist) {
			err = nil
		}
		return before, err
	}
	if _, err := r.git(nil, "checkout", "--force", id, "--", path); err != nil {
		return before, err
	}
	r.git(nil, "reset", "--quiet", "--mixed", "HEAD")
	return before, nil
}

// Clear forgets every checkpoint of the project, removing its shadow
// repository. The project's files are not touched.
func (r *Repo) Clear() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	reposMu.Lock()
	for k, v := range repos {
		if v == r {
			delete(repos, k)
		}
	}
	reposMu.Unlock()
	forget(repoName(r.Project))
	return os.RemoveAll(r.gitDir)
}

// KeepFile makes file path, as it is now, part of the latest checkpoint:
// it no longer counts as changed since. The project's files are not
// touched.
func (r *Repo) KeepFile(path string) error {
	if path == "" || strings.HasPrefix(path, "/") || strings.Contains("/"+path+"/", "/../") {
		return errNoSuch
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	// A file left out of the copy: noted as it is now, to show again only
	// when it changes again.
	skip := r.readSkipped()
	if _, ok := skip[path]; ok {
		if now, ok := r.noted(path, true); ok {
			skip[path] = now
		} else {
			delete(skip, path)
		}
		return r.writeSkipped(skip)
	}
	head, err := r.git(nil, "rev-parse", "--verify", "--quiet", "HEAD")
	if err != nil || len(strings.TrimSpace(string(head))) == 0 {
		return errNoSuch
	}
	if _, err := r.git(nil, "reset", "--quiet", "--mixed", "HEAD"); err != nil {
		return err
	}
	if _, err := r.git(nil, "add", "--all", "--", path); err != nil {
		return err
	}
	if _, err := r.git(nil, "commit", "--quiet", "--no-verify", "--allow-empty", "--amend", "--no-edit"); err != nil {
		return err
	}
	r.git(nil, "reflog", "expire", "--expire=now", "--all")
	r.git(nil, "prune", "--expire=now")
	return nil
}
