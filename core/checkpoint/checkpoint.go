// Package checkpoint keeps a copy of a project's files as they were
// before an AI first changed them, in a git repository of Nook's own on
// the project's drive (place.go), apart from the project's .git, which it
// never touches: to show what the AI changed, and to keep or undo it, all
// or a file at a time. It runs the git of this computer, which never
// connects anywhere for these commands.
package checkpoint

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"nook/core/lang"
)

// ErrNoGit: git is not installed.
var ErrNoGit = lang.Error{Zh: "没有找到 git：保留 / 撤销改动需要安装 Git（git-scm.com）", En: "git not found: keeping or undoing changes needs Git (git-scm.com)"}

// ErrTooBig: the folder is not one to snapshot, as the home folder or a
// drive.
var ErrTooBig = lang.Error{Zh: "这个文件夹太大（用户目录或整个磁盘），不记录改动", En: "This folder is too large to track changes in (the home folder or a whole drive)"}

// Checkpoint is a snapshot.
type Checkpoint struct {
	ID    string
	Time  time.Time
	Label string
}

// Change is a file changed since a checkpoint.
type Change struct {
	Path        string // slash-separated, relative to the project
	Status      byte   // 'A' added, 'M' modified, 'D' deleted
	Plus, Minus int    // lines added and removed
	Binary      bool   // not text: no lines counted
	Unsaved     bool   // a file left out of the copy (skip.go): known, not to undo
}

// Repo is a project's shadow repository.
type Repo struct {
	Project string
	gitDir  string
	mu      sync.Mutex // one git command at a time: they share the index
}

// excludes are folders a snapshot leaves out, as a project's .gitignore
// would: what tools make again, and that is large.
var excludes = []string{
	"node_modules/", ".venv/", "venv/", "__pycache__/", ".next/", ".nuxt/",
	".turbo/", ".cache/", ".gradle/", ".idea/", ".vs/", "target/debug/", "target/release/",
}

// maxFile is the largest file a snapshot takes: larger ones, as
// installers and videos, are left out (skip.go).
var maxFile int64 = 20 << 20

// Root is where the shadow repositories are kept, set by the app.
var Root = func() string {
	d, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(d, "Nook", "shadow")
}

var (
	reposMu sync.Mutex
	repos   = map[string]*Repo{}
)

// Open returns the shadow repository of project, making it the first
// time.
func Open(project string) (*Repo, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return nil, ErrNoGit
	}
	project = filepath.Clean(project)
	if tooBig(project) {
		return nil, ErrTooBig
	}
	key := strings.ToLower(project)
	reposMu.Lock()
	defer reposMu.Unlock()
	if r := repos[key]; r != nil {
		return r, nil
	}
	r := &Repo{Project: project, gitDir: gitDirOf(project, true)}
	if _, err := os.Stat(filepath.Join(r.gitDir, "HEAD")); err != nil {
		if err := os.MkdirAll(r.gitDir, 0o755); err != nil {
			return nil, err
		}
		// No template: none of git's sample hooks, which Nook never runs.
		if _, err := r.git(nil, "init", "--quiet", "--template="); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Join(r.gitDir, "info"), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(r.gitDir, "info", "exclude"), []byte(strings.Join(excludes, "\n")+"\n"), 0o644); err != nil {
			return nil, err
		}
		os.WriteFile(filepath.Join(r.gitDir, "nook-project.txt"), []byte(project+"\n"), 0o644)
		note(repoName(project), r.gitDir)
	}
	repos[key] = r
	return r, nil
}

// repoName is the folder name of project's repository: its name, and a
// hash of its path, that two projects of a name stay apart.
func repoName(project string) string {
	sum := sha1.Sum([]byte(strings.ToLower(project)))
	return filepath.Base(project) + "-" + hex.EncodeToString(sum[:])[:12]
}

// tooBig reports whether dir is the user's home or a drive's root.
func tooBig(dir string) bool {
	if home, err := os.UserHomeDir(); err == nil && strings.EqualFold(filepath.Clean(home), dir) {
		return true
	}
	return filepath.Dir(dir) == dir
}

// git runs git on the shadow repository and the project's files, with
// none of the user's settings that would change what it does.
func (r *Repo) git(stdin []byte, args ...string) ([]byte, error) {
	return r.gitEnv(nil, stdin, args...)
}

// gitEnv is git, with env added to the environment.
func (r *Repo) gitEnv(env []string, stdin []byte, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	base := []string{
		"--git-dir=" + r.gitDir, "--work-tree=" + r.Project,
		"-c", "core.autocrlf=false", "-c", "core.safecrlf=false", "-c", "core.quotepath=false",
		"-c", "core.fsmonitor=false", "-c", "core.hooksPath=" + filepath.Join(r.gitDir, "no-hooks"),
		"-c", "commit.gpgsign=false", "-c", "user.name=Nook", "-c", "user.email=nook@localhost",
		"-c", "gc.autoDetach=false", "-c", "safe.directory=*",
	}
	cmd := exec.CommandContext(ctx, "git", append(base, args...)...)
	cmd.Dir = r.Project
	cmd.Env = append(gitEnv(), env...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	hideWindow(cmd)
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if ctx.Err() != nil {
			msg = lang.L("git 太久没有完成", "git took too long")
		}
		return out.Bytes(), fmt.Errorf("git %s: %v %s", args[0], err, msg)
	}
	return out.Bytes(), nil
}

// gitEnv is Nook's environment less what would point git elsewhere.
func gitEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(strings.ToUpper(k), "GIT_") {
			continue
		}
		env = append(env, kv)
	}
	return append(env, "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0")
}

// stage puts the project's files in the shadow index as they are now:
// those it holds, and the new ones but those copies leave out (skip.go),
// which it notes instead, the files of a first copy as there before the
// AI. A first copy over maxTotal stages nothing: TooMuch. The files to
// add go to git by name, not by pattern, which thousands left out would
// slow down.
func (r *Repo) stage() error {
	head, _ := r.git(nil, "rev-parse", "--verify", "--quiet", "HEAD")
	first := len(bytes.TrimSpace(head)) == 0
	out, err := r.git(nil, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return err
	}
	skip := r.readSkipped()
	var add []string
	var total int64
	noted := false
	for _, p := range strings.Split(string(out), "\x00") {
		if p == "" || strings.HasSuffix(p, "/") { // a repository of its own inside
			continue
		}
		if _, ok := skip[p]; ok {
			continue
		}
		full := filepath.Join(r.Project, filepath.FromSlash(p))
		fi, err := os.Lstat(full)
		if err != nil || fi.IsDir() {
			continue
		}
		if fi.Mode().IsRegular() && leaveOut(full, fi.Size()) {
			skip[p] = skipped{base: first, size: fi.Size(), mtime: fi.ModTime().UnixNano()}
			noted = true
			continue
		}
		add = append(add, p)
		total += fi.Size()
	}
	if first && total > maxTotal {
		return TooMuch{total}
	}
	if noted {
		if err := r.writeSkipped(skip); err != nil {
			return err
		}
	}
	if !first {
		if _, err := r.git(nil, "add", "--update", "--ignore-errors"); err != nil {
			return err
		}
	}
	if len(add) == 0 {
		return nil
	}
	err = r.addFiles(add)
	if err != nil {
		// A file another program holds: leave out those that cannot be
		// read, once.
		readable := add[:0:0]
		for _, p := range add {
			if f, err := os.Open(filepath.Join(r.Project, filepath.FromSlash(p))); err == nil {
				f.Close()
				readable = append(readable, p)
			}
		}
		if len(readable) < len(add) {
			err = r.addFiles(readable)
		}
	}
	return err
}

// addFiles adds files, by path, to the shadow index.
func (r *Repo) addFiles(paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	_, err := r.git([]byte(strings.Join(paths, "\x00")+"\x00"), "update-index", "--add", "--remove", "-z", "--stdin")
	return err
}

// Snapshot keeps the project's files as they are now. made is false
// when nothing changed since the last snapshot, which it returns.
func (r *Repo) Snapshot(label string) (cp Checkpoint, made bool, err error) {
	return r.snapshot(label, false)
}

// snapshot is Snapshot; always makes one, nothing changed or not, so that
// a restore always has its own to go back to, under its own name.
func (r *Repo) snapshot(label string, always bool) (cp Checkpoint, made bool, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.stage(); err != nil {
		return Checkpoint{}, false, err
	}
	head, _ := r.git(nil, "rev-parse", "--verify", "--quiet", "HEAD")
	if len(bytes.TrimSpace(head)) > 0 && !always {
		if _, err := r.git(nil, "diff", "--cached", "--quiet", "HEAD"); err == nil {
			list, err := r.list(1)
			if err != nil || len(list) == 0 {
				return Checkpoint{}, false, err
			}
			return list[0], false, nil
		}
	}
	if _, err := r.git(nil, "commit", "--quiet", "--no-verify", "--allow-empty", "-m", label); err != nil {
		return Checkpoint{}, false, err
	}
	list, err := r.list(1)
	if err != nil || len(list) == 0 {
		return Checkpoint{}, false, err
	}
	return list[0], true, nil
}

// List returns the latest n snapshots, the latest first.
func (r *Repo) List(n int) ([]Checkpoint, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.list(n)
}

func (r *Repo) list(n int) ([]Checkpoint, error) {
	head, _ := r.git(nil, "rev-parse", "--verify", "--quiet", "HEAD")
	if len(bytes.TrimSpace(head)) == 0 {
		return nil, nil
	}
	out, err := r.git(nil, "log", "-n", strconv.Itoa(n), "--format=%H%x00%ct%x00%s")
	if err != nil {
		return nil, err
	}
	var cps []Checkpoint
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.SplitN(line, "\x00", 3)
		if len(f) != 3 {
			continue
		}
		sec, _ := strconv.ParseInt(f[1], 10, 64)
		cps = append(cps, Checkpoint{ID: f[0], Time: time.Unix(sec, 0), Label: f[2]})
	}
	return cps, nil
}

// Changes returns the files changed since checkpoint id.
func (r *Repo) Changes(id string) ([]Change, error) {
	if !validID(id) {
		return nil, errors.New(lang.L("找不到改动前的备份", "The copy from before the changes is gone"))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.stage(); err != nil {
		return nil, err
	}
	out, err := r.git(nil, "diff", "--cached", "--name-status", "--no-renames", "-z", id)
	if err != nil {
		return nil, err
	}
	f := strings.Split(string(out), "\x00")
	var cs []Change
	for i := 0; i+1 < len(f); i += 2 {
		if f[i] == "" {
			continue
		}
		cs = append(cs, Change{Status: f[i][0], Path: f[i+1]})
	}
	// Lines added and removed: "plus\tminus\tpath", "-\t-\tpath" for binary files.
	if out, err := r.git(nil, "diff", "--cached", "--numstat", "--no-renames", "-z", id); err == nil {
		at := map[string]int{}
		for i, c := range cs {
			at[c.Path] = i
		}
		for _, rec := range strings.Split(string(out), "\x00") {
			f := strings.SplitN(rec, "\t", 3)
			if len(f) != 3 {
				continue
			}
			i, ok := at[f[2]]
			if !ok {
				continue
			}
			if f[0] == "-" {
				cs[i].Binary = true
				continue
			}
			cs[i].Plus, _ = strconv.Atoi(f[0])
			cs[i].Minus, _ = strconv.Atoi(f[1])
		}
	}
	return append(cs, r.unsaved(r.readSkipped())...), nil
}

// Content returns file path as checkpoint id kept it, "" when it had none.
func (r *Repo) Content(id, path string) (string, error) {
	if !validID(id) {
		return "", errors.New(lang.L("找不到改动前的备份", "The copy from before the changes is gone"))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out, err := r.git(nil, "cat-file", "blob", id+":"+path)
	if err != nil {
		return "", nil
	}
	return string(out), nil
}

// Restore puts the project's files back as checkpoint id kept them:
// those it changed go back, those it added go. With a label, it first
// snapshots them as they are under it, so that the restore can be
// undone, and returns that snapshot; with none, as when undoing a
// restore, it does not.
func (r *Repo) Restore(id, label string) (Checkpoint, error) {
	if !validID(id) {
		return Checkpoint{}, errors.New(lang.L("找不到改动前的备份", "The copy from before the changes is gone"))
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
	// What was added since: what the snapshot lacks of the files now.
	if err := r.stage(); err != nil {
		return before, err
	}
	out, err := r.git(nil, "diff", "--cached", "--name-only", "--no-renames", "--diff-filter=A", "-z", id)
	if err != nil {
		return before, err
	}
	for _, p := range strings.Split(string(out), "\x00") {
		if p != "" {
			os.Remove(filepath.Join(r.Project, filepath.FromSlash(p)))
		}
	}
	if _, err := r.git(nil, "checkout", "--force", id, "--", "."); err != nil {
		return before, err
	}
	// The index follows what the files are now, for the next comparison.
	r.git(nil, "reset", "--quiet", "--mixed", "HEAD")
	return before, nil
}

func validID(id string) bool {
	if len(id) < 7 || len(id) > 64 {
		return false
	}
	for _, c := range id {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return true
}
