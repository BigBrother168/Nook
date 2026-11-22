package checkpoint

import (
	"os/exec"
	"syscall"
)

// hideWindow keeps git from flashing a console window.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}

// hide marks a folder hidden, as Explorer shows none such by default.
func hide(path string) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return
	}
	if attrs, err := syscall.GetFileAttributes(p); err == nil && attrs&syscall.FILE_ATTRIBUTE_HIDDEN == 0 {
		syscall.SetFileAttributes(p, attrs|syscall.FILE_ATTRIBUTE_HIDDEN)
	}
}
