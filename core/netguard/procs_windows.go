package netguard

import (
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Proc is a running process.
type Proc struct {
	PID, Parent uint32
	Name        string // "claude.exe"
}

// Tool returns the process's program name without ".exe", lower case.
func (p Proc) Tool() string { return strings.TrimSuffix(strings.ToLower(p.Name), ".exe") }

// Snapshot returns the processes running now, by ID.
func Snapshot() map[uint32]Proc {
	out := map[uint32]Proc{}
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return out
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		out[e.ProcessID] = Proc{e.ProcessID, e.ParentProcessID, windows.UTF16ToString(e.ExeFile[:])}
	}
	return out
}

// Children indexes procs by parent.
func Children(procs map[uint32]Proc) map[uint32][]uint32 {
	out := map[uint32][]uint32{}
	for _, p := range procs {
		out[p.Parent] = append(out[p.Parent], p.PID)
	}
	return out
}

// Tree returns root and its descendants that started after since (none
// before root started), children before their children. A process whose
// parent ended still names it, so the chain holds; the start time keeps a
// reused process ID out.
func Tree(kids map[uint32][]uint32, root uint32, since time.Time) []uint32 {
	out := []uint32{root}
	seen := map[uint32]bool{root: true}
	for i := 0; i < len(out); i++ {
		for _, c := range kids[out[i]] {
			if !seen[c] && !Started(c).Before(since) {
				seen[c] = true
				out = append(out, c)
			}
		}
	}
	return out
}

// Started returns when process pid started; the zero time if unknown.
func Started(pid uint32) time.Time {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return time.Time{}
	}
	defer windows.CloseHandle(h)
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		return time.Time{}
	}
	return time.Unix(0, created.Nanoseconds())
}

// CommandLine returns how process pid was started, "" when Windows does
// not say (a process of another user, or ended).
func CommandLine(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	const processCommandLineInformation = 60
	var size uint32
	windows.NtQueryInformationProcess(h, processCommandLineInformation, nil, 0, &size)
	if size == 0 || size > 1<<16 {
		return ""
	}
	buf := make([]byte, size)
	if err := windows.NtQueryInformationProcess(h, processCommandLineInformation, unsafe.Pointer(&buf[0]), size, &size); err != nil {
		return ""
	}
	us := (*windows.NTUnicodeString)(unsafe.Pointer(&buf[0]))
	if us.Buffer == nil || us.Length == 0 {
		return ""
	}
	return windows.UTF16ToString(unsafe.Slice(us.Buffer, us.Length/2))
}

// ImagePath returns the file process pid runs, "" when Windows does not
// say.
func ImagePath(pid uint32) string {
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
