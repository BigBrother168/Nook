package main

import (
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

// shell is a program a terminal can run.
type shell struct {
	ID      string   // stable, saved in the settings
	Name    string   // shown
	Command []string // program and arguments
	Env     []string // added to the environment
	Quote   quoting  // how it takes a dropped file's path
}

// detectShells returns the shells installed, the default first among them
// as found: PowerShell 7, Windows PowerShell, Command Prompt, Git Bash, WSL.
func detectShells() []shell {
	var out []shell
	if p, err := exec.LookPath("pwsh.exe"); err == nil {
		out = append(out, shell{ID: "pwsh", Name: "PowerShell 7", Command: []string{p, "-NoLogo"}, Quote: quotePowerShell})
	}
	if p, err := exec.LookPath("powershell.exe"); err == nil {
		out = append(out, shell{ID: "powershell", Name: "Windows PowerShell", Command: []string{p, "-NoLogo"}, Quote: quotePowerShell})
	}
	cmd := os.Getenv("COMSPEC")
	if cmd == "" {
		cmd, _ = exec.LookPath("cmd.exe")
	}
	if cmd != "" {
		out = append(out, shell{ID: "cmd", Name: L("命令提示符", "Command Prompt"), Command: []string{cmd}, Quote: quoteCmd})
	}
	if bash := gitBash(); bash != "" {
		out = append(out, shell{ID: "gitbash", Name: "Git Bash", Command: []string{bash, "--login", "-i"},
			Env: []string{"CHERE_INVOKING=1"}, Quote: quoteMSYS})
	}
	if p, err := exec.LookPath("wsl.exe"); err == nil && wslInstalled() {
		out = append(out, shell{ID: "wsl", Name: "WSL", Command: []string{p}, Quote: quoteWSL})
	}
	return out
}

// gitBash returns Git for Windows' bash.exe, next to the git.exe in PATH or
// where its installer puts it.
func gitBash() string {
	var roots []string
	if g, err := exec.LookPath("git.exe"); err == nil {
		d := filepath.Dir(g) // …\Git\cmd, or …\Git\mingw64\bin
		roots = append(roots, filepath.Dir(d), filepath.Dir(filepath.Dir(d)))
	}
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\GitForWindows`, registry.QUERY_VALUE); err == nil {
		if p, _, err := k.GetStringValue("InstallPath"); err == nil {
			roots = append(roots, p)
		}
		k.Close()
	}
	roots = append(roots, filepath.Join(os.Getenv("ProgramFiles"), "Git"))
	for _, r := range roots {
		p := filepath.Join(r, "bin", "bash.exe")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// wslInstalled reports whether WSL has a distribution, which wsl.exe alone,
// present on every Windows, does not say.
func wslInstalled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Lxss`, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return false
	}
	defer k.Close()
	names, _ := k.ReadSubKeyNames(1)
	return len(names) > 0
}

// shellByID returns the shell with id, else the first.
func shellByID(shells []shell, id string) shell {
	for _, s := range shells {
		if s.ID == id {
			return s
		}
	}
	if len(shells) > 0 {
		return shells[0]
	}
	return shell{ID: "cmd", Name: L("命令提示符", "Command Prompt"), Command: []string{"cmd.exe"}, Quote: quoteCmd}
}
