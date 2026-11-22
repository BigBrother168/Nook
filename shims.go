package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// installShims puts the guard, as <tool>.exe, in a folder that goes first
// in PATH in Nook's terminals, for each AI tool installed: typed there,
// "claude" runs the guard, which checks the network rule and lock, then
// the real claude. The folder is Nook's own (%LOCALAPPDATA%\Nook\shims);
// nothing of the tools changes. It returns the folder, "" when Nook has
// no guard next to it.
func installShims(tools []string) string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	guard, err := os.ReadFile(filepath.Join(filepath.Dir(exe), "nook-guard.exe"))
	if err != nil {
		return ""
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(base, "Nook", "shims")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	want := map[string]bool{}
	for _, t := range tools {
		if !validTool(t) {
			continue
		}
		if _, err := exec.LookPath(t); err != nil {
			continue // not installed
		}
		name := strings.ToLower(t) + ".exe"
		want[name] = true
		dst := filepath.Join(dir, name)
		if cur, err := os.ReadFile(dst); err == nil && bytes.Equal(cur, guard) {
			continue
		}
		tmp := dst + ".tmp"
		if os.WriteFile(tmp, guard, 0o755) == nil {
			if os.Rename(tmp, dst) != nil {
				os.Remove(tmp) // in use by a running tool: the old one stays
			}
		}
	}
	// Tools no longer installed, or no longer on the list, lose theirs.
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if !want[strings.ToLower(e.Name())] {
				os.Remove(filepath.Join(dir, e.Name()))
			}
		}
	}
	return dir
}

// validTool reports whether t is a plain command name.
func validTool(t string) bool {
	if t == "" || len(t) > 64 {
		return false
	}
	for _, r := range t {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}
