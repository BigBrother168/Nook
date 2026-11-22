package pty

import (
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// CleanOrphans ends the console hosts a Nook that ended without closing its
// terminals left: OpenConsole.exe processes of this Nook's own copy (next
// to Nook.exe) whose parent is no longer a running Nook. Only Nook runs
// that copy, so no other program's console is touched. It returns how
// many it ended.
func CleanOrphans() int {
	exe, err := os.Executable()
	if err != nil {
		return 0
	}
	ours := filepath.Join(filepath.Dir(exe), "OpenConsole.exe")
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0
	}
	defer windows.CloseHandle(snap)
	type proc struct {
		pid, parent uint32
		name        string
	}
	var procs []proc
	byPID := map[uint32]proc{}
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		p := proc{e.ProcessID, e.ParentProcessID, windows.UTF16ToString(e.ExeFile[:])}
		procs = append(procs, p)
		byPID[p.pid] = p
	}
	ended := 0
	for _, p := range procs {
		if !strings.EqualFold(p.name, "OpenConsole.exe") || !strings.EqualFold(imagePath(p.pid), ours) {
			continue
		}
		if parent, ok := byPID[p.parent]; ok && strings.EqualFold(parent.name, "Nook.exe") &&
			started(p.parent).Before(started(p.pid)) {
			continue // a running Nook's
		}
		if h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, p.pid); err == nil {
			if windows.TerminateProcess(h, 1) == nil {
				ended++
			}
			windows.CloseHandle(h)
		}
	}
	return ended
}

// imagePath returns the full path of process pid's program.
func imagePath(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}

// started returns when process pid started, the far future if unknown.
func started(pid uint32) time.Time {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return time.Unix(1<<40, 0)
	}
	defer windows.CloseHandle(h)
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		return time.Unix(1<<40, 0)
	}
	return time.Unix(0, created.Nanoseconds())
}
