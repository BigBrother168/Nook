package pty

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"nook/core/lang"
)

var (
	kernel32                       = windows.NewLazySystemDLL("kernel32.dll")
	procUpdateProcThreadAttribute  = kernel32.NewProc("UpdateProcThreadAttribute")
	procInitializeProcThreadAttrib = kernel32.NewProc("InitializeProcThreadAttributeList")
	procDeleteProcThreadAttribList = kernel32.NewProc("DeleteProcThreadAttributeList")
)

// conpty is Microsoft's ConPTY as Windows Terminal and VS Code ship it,
// conpty.dll and OpenConsole.exe next to Nook.exe, loaded from there only.
// Windows' own (kernel32) is older: it forwards a program's synchronized
// output marks (mode 2026) ahead of the text between them, so full-screen
// programs flicker. Without the files Nook falls back to Windows' own.
var conpty = sync.OnceValue(func() *windows.LazyDLL {
	exe, err := os.Executable()
	if err != nil {
		return nil
	}
	dir := filepath.Dir(exe)
	if _, err := os.Stat(filepath.Join(dir, "OpenConsole.exe")); err != nil {
		return nil
	}
	dll := windows.NewLazyDLL(filepath.Join(dir, "conpty.dll"))
	if dll.Load() != nil || dll.NewProc("ConptyCreatePseudoConsole").Find() != nil {
		return nil
	}
	return dll
})

// pseudoconsoleGlyphWidthGraphemes measures characters by grapheme
// clusters, as the terminal (Ghostty's) does.
const pseudoconsoleGlyphWidthGraphemes = 0x08

// UsingBundledConPTY reports whether sessions use the ConPTY next to Nook.
func UsingBundledConPTY() bool { return conpty() != nil }

func createPseudoConsole(size windows.Coord, in, out windows.Handle, console *windows.Handle) error {
	dll := conpty()
	if dll == nil {
		return windows.CreatePseudoConsole(size, in, out, 0, console)
	}
	r, _, _ := dll.NewProc("ConptyCreatePseudoConsole").Call(
		uintptr(*(*uint32)(unsafe.Pointer(&size))), uintptr(in), uintptr(out),
		pseudoconsoleGlyphWidthGraphemes, uintptr(unsafe.Pointer(console)))
	if r != 0 {
		return windows.Errno(r)
	}
	return nil
}

func resizePseudoConsole(console windows.Handle, size windows.Coord) error {
	dll := conpty()
	if dll == nil {
		return windows.ResizePseudoConsole(console, size)
	}
	r, _, _ := dll.NewProc("ConptyResizePseudoConsole").Call(uintptr(console), uintptr(*(*uint32)(unsafe.Pointer(&size))))
	if r != 0 {
		return windows.Errno(r)
	}
	return nil
}

func closePseudoConsole(console windows.Handle) {
	dll := conpty()
	if dll == nil {
		windows.ClosePseudoConsole(console)
		return
	}
	dll.NewProc("ConptyClosePseudoConsole").Call(uintptr(console))
}

// startDelay is how long a Session waits for its first size before it
// starts its program at 80×24 anyway.
const startDelay = 500 * time.Millisecond

// Session is a program in a ConPTY, inside a job object of its own: when
// the session closes, or Nook exits in any way, the job's handle closes
// and Windows ends every process in it, the program's children too. Close
// also ends the descendants that left the job (see sweep), and the
// console tells programs still attached to it that it closed, as it does
// when Nook ends without closing it.
type Session struct {
	opts Options

	startOnce sync.Once
	started   chan struct{}
	startErr  error
	errShown  bool

	mu      sync.Mutex
	console windows.Handle // the HPCON, 0 once closed
	job     windows.Handle
	process windows.Handle
	pid     uint32
	since   time.Time // just before the program started
	in      *os.File
	out     *os.File
	closed  bool

	done     chan struct{}
	exitCode int
}

// Start makes a Session. Its program starts at the first Resize, so that it
// draws its first screen at the view's size, or after half a second.
func Start(o Options) *Session {
	s := &Session{opts: o, started: make(chan struct{}), done: make(chan struct{})}
	time.AfterFunc(startDelay, func() { s.start(80, 24) })
	return s
}

func (s *Session) start(cols, rows int) {
	s.startOnce.Do(func() {
		s.startErr = s.spawn(cols, rows)
		if s.startErr != nil {
			close(s.done)
		}
		close(s.started)
	})
}

func (s *Session) spawn(cols, rows int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	if len(s.opts.Command) == 0 {
		return errors.New("pty: no command")
	}
	path, err := exec.LookPath(s.opts.Command[0])
	if err != nil {
		return fmt.Errorf(lang.L("找不到 %s：%w", "%s not found: %w"), s.opts.Command[0], err)
	}

	// The job: every process the program starts belongs to it, and closing
	// its last handle ends them.
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return fmt.Errorf("pty: CreateJobObject: %w", err)
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		return fmt.Errorf("pty: SetInformationJobObject: %w", err)
	}

	// The console reads what is typed from inR and writes the output to
	// outW; Nook writes to inW and reads outR.
	var inR, inW, outR, outW windows.Handle
	if err := windows.CreatePipe(&inR, &inW, nil, 0); err != nil {
		windows.CloseHandle(job)
		return fmt.Errorf("pty: %w", err)
	}
	if err := windows.CreatePipe(&outR, &outW, nil, 0); err != nil {
		windows.CloseHandle(job)
		windows.CloseHandle(inR)
		windows.CloseHandle(inW)
		return fmt.Errorf("pty: %w", err)
	}
	var console windows.Handle
	err = createPseudoConsole(coord(cols, rows), inR, outW, &console)
	windows.CloseHandle(inR)
	windows.CloseHandle(outW)
	if err != nil {
		windows.CloseHandle(job)
		windows.CloseHandle(inW)
		windows.CloseHandle(outR)
		return fmt.Errorf(lang.L("pty: CreatePseudoConsole（需要 Windows 10 1809 以上）：%w", "pty: CreatePseudoConsole (needs Windows 10 1809 or later): %w"), err)
	}

	since := time.Now().Add(-time.Second) // process times are coarser than the clock
	process, thread, pid, err := createProcess(console, path, s.opts)
	if err == nil {
		// The program starts suspended, joins the job, then runs, so that
		// nothing it starts can be outside the job.
		if err = windows.AssignProcessToJobObject(job, process); err == nil {
			_, err = windows.ResumeThread(thread)
		}
		windows.CloseHandle(thread)
		if err != nil {
			windows.TerminateProcess(process, 1)
			windows.CloseHandle(process)
		}
	}
	if err != nil {
		closePseudoConsole(console)
		windows.CloseHandle(job)
		windows.CloseHandle(inW)
		windows.CloseHandle(outR)
		return err
	}

	s.console, s.job, s.process, s.pid, s.since = console, job, process, pid, since
	s.in = os.NewFile(uintptr(inW), "conpty-in")
	s.out = os.NewFile(uintptr(outR), "conpty-out")
	go s.wait()
	return nil
}

func createProcess(console windows.Handle, path string, o Options) (process, thread windows.Handle, pid uint32, err error) {
	var size uintptr
	procInitializeProcThreadAttrib.Call(0, 1, 0, uintptr(unsafe.Pointer(&size)))
	list := make([]byte, size)
	if r, _, e := procInitializeProcThreadAttrib.Call(uintptr(unsafe.Pointer(&list[0])), 1, 0, uintptr(unsafe.Pointer(&size))); r == 0 {
		return 0, 0, 0, fmt.Errorf("pty: InitializeProcThreadAttributeList: %w", e)
	}
	defer procDeleteProcThreadAttribList.Call(uintptr(unsafe.Pointer(&list[0])))
	if r, _, e := procUpdateProcThreadAttribute.Call(uintptr(unsafe.Pointer(&list[0])), 0,
		windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, uintptr(console), unsafe.Sizeof(console), 0, 0); r == 0 {
		return 0, 0, 0, fmt.Errorf("pty: UpdateProcThreadAttribute: %w", e)
	}

	var si windows.StartupInfoEx
	si.Cb = uint32(unsafe.Sizeof(si))
	// The console's own standard handles: without invalid ones, a child of
	// a process whose output is redirected would write there instead.
	si.Flags = windows.STARTF_USESTDHANDLES
	si.StdInput, si.StdOutput, si.StdErr = windows.InvalidHandle, windows.InvalidHandle, windows.InvalidHandle
	si.ProcThreadAttributeList = (*windows.ProcThreadAttributeList)(unsafe.Pointer(&list[0]))

	args := append([]string{path}, o.Command[1:]...)
	cmdline, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(args))
	if err != nil {
		return 0, 0, 0, err
	}
	var dir *uint16
	if o.Dir != "" {
		if dir, err = windows.UTF16PtrFromString(o.Dir); err != nil {
			return 0, 0, 0, err
		}
	}
	env := environmentBlock(o.Env)
	var pi windows.ProcessInformation
	flags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_UNICODE_ENVIRONMENT | windows.CREATE_SUSPENDED)
	if err := windows.CreateProcess(nil, cmdline, nil, nil, false, flags, &env[0], dir, &si.StartupInfo, &pi); err != nil {
		return 0, 0, 0, fmt.Errorf(lang.L("启动 %s 失败：%w", "could not start %s: %w"), path, err)
	}
	return pi.Process, pi.Thread, pi.ProcessId, nil
}

// wait waits for the program to end, then hangs up the console once its
// last output is read, as terminal apps do.
func (s *Session) wait() {
	windows.WaitForSingleObject(s.process, windows.INFINITE)
	var code uint32
	windows.GetExitCodeProcess(s.process, &code)
	s.mu.Lock()
	s.exitCode = int(code)
	s.mu.Unlock()
	time.Sleep(100 * time.Millisecond)
	s.Close()
}

// Read reads what the program writes. Before the program starts it waits;
// if it could not start, it returns why, in red, then io.EOF.
func (s *Session) Read(b []byte) (int, error) {
	<-s.started
	if s.startErr != nil {
		if s.errShown {
			return 0, io.EOF
		}
		s.errShown = true
		return copy(b, "\x1b[31m"+s.startErr.Error()+"\x1b[0m\r\n"), nil
	}
	return s.out.Read(b)
}

// Write types b into the terminal.
func (s *Session) Write(b []byte) (int, error) {
	<-s.started
	if s.startErr != nil {
		return 0, ErrClosed
	}
	n, err := s.in.Write(b)
	if errors.Is(err, os.ErrClosed) {
		err = ErrClosed
	}
	return n, err
}

// Resize sets the terminal's size; the first one starts the program.
func (s *Session) Resize(cols, rows int) error {
	s.start(cols, rows)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.console == 0 {
		return nil
	}
	return resizePseudoConsole(s.console, coord(cols, rows))
}

// Close ends every process of the session and closes the console. Read
// returns io.EOF once the console wrote what it had.
func (s *Session) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()
	// A session closed before it started never starts: spawn sees closed,
	// which also lets Read and Write return.
	s.start(80, 24)
	s.mu.Lock()
	console, job, pid, since := s.console, s.job, s.pid, s.since
	s.console, s.job = 0, 0
	s.mu.Unlock()
	if console == 0 {
		return nil
	}
	sweep(pid, since)        // those that left the job
	windows.CloseHandle(job) // the program and all else it started
	s.in.Close()
	// ClosePseudoConsole waits for the output to be read before Windows 11
	// 24H2: Read keeps reading meanwhile, until the end, then the pipe goes.
	go func() {
		closePseudoConsole(console)
		s.out.Close()
		windows.CloseHandle(s.process)
		close(s.done)
	}()
	return nil
}

// Pid returns the program's process ID, 0 before it started.
func (s *Session) Pid() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return int(s.pid)
}

// Since returns a moment just before the program started: its
// descendants started after it.
func (s *Session) Since() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.since
}

// Done is closed once the session ended.
func (s *Session) Done() <-chan struct{} { return s.done }

// coord packs a size as ConPTY takes it.
func coord(cols, rows int) windows.Coord {
	return windows.Coord{X: int16(max(1, min(cols, 0x7FFF))), Y: int16(max(1, min(rows, 0x7FFF)))}
}

// environmentBlock encodes env as CreateProcess takes it: NUL-terminated
// UTF-16 strings, then another NUL.
func environmentBlock(env []string) []uint16 {
	var b []uint16
	for _, kv := range env {
		u, err := windows.UTF16FromString(kv)
		if err != nil {
			continue // a NUL inside: leave it out
		}
		b = append(b, u...) // with its NUL
	}
	if len(b) == 0 {
		b = append(b, 0)
	}
	return append(b, 0)
}
