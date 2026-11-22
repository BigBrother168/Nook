package pty

import (
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// sweep ends the descendants of root that started after since, the
// session's start: those a job cannot hold, as programs started with
// CREATE_BREAKAWAY_FROM_JOB (PowerShell's Start-Process does) are
// outside it. A process whose parent ended still names it, so the chain
// holds; the start time keeps a reused process ID out.
func sweep(root uint32, since time.Time) {
	for _, pid := range descendants(root, since) {
		h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid)
		if err != nil {
			continue
		}
		windows.TerminateProcess(h, 1)
		windows.CloseHandle(h)
	}
}

// descendants returns the processes descended from root that started
// after since, children before their children.
func descendants(root uint32, since time.Time) []uint32 {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	children := map[uint32][]uint32{}
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		children[e.ParentProcessID] = append(children[e.ParentProcessID], e.ProcessID)
	}
	var out []uint32
	seen := map[uint32]bool{root: true}
	queue := []uint32{root}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		for _, c := range children[p] {
			if seen[c] || !startedAfter(c, since) {
				continue
			}
			seen[c] = true
			out = append(out, c)
			queue = append(queue, c)
		}
	}
	return out
}

// startedAfter reports whether process pid started after t.
func startedAfter(pid uint32, t time.Time) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		return false
	}
	return !time.Unix(0, created.Nanoseconds()).Before(t)
}
