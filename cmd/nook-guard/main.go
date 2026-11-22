// nook-guard stands in for an AI command-line tool in Nook's terminals.
// Nook copies it as claude.exe, codex.exe… into a folder first in their
// PATH. Typed "claude", it checks Nook's network rule and lock, says what
// it found, then runs the real claude with the same arguments, or stops
// it. It reads Nook's settings and this computer's tables; it never
// connects anywhere, and changes nothing of the tool's.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"

	"nook/core/config"
	"nook/core/netguard"

	"nook/core/lang"
)

const (
	green = "\x1b[32m"
	red   = "\x1b[31m"
	gray  = "\x1b[90m"
	reset = "\x1b[0m"
)

func main() {
	lang.Set(config.Load().Language, lang.System())
	self, err := os.Executable()
	if err != nil {
		fail("nook-guard: " + err.Error())
	}
	tool := strings.TrimSuffix(strings.ToLower(filepath.Base(self)), ".exe")
	real := findReal(tool, filepath.Dir(self))
	if real == "" {
		fail(fmt.Sprintf(lang.L("✗ 找不到 %s：它不在 PATH 里", "✗ %s not found: it is not in PATH"), tool))
	}
	// Only in Nook's terminals; elsewhere (the folder in another PATH by
	// mistake) the tool just runs.
	env := os.Environ()
	args := os.Args[1:]
	if os.Getenv("NOOK") == "1" {
		args = nookArgs(tool, args)
		var ok bool
		var lines []string
		ok, lines, env = check(tool, config.Load(), env)
		for _, l := range lines {
			fmt.Fprintln(os.Stderr, l)
		}
		if !ok {
			os.Exit(1)
		}
	}
	// Ctrl+C is the tool's: the guard waits for it to end.
	signal.Ignore(os.Interrupt)
	cmd := exec.Command(real, args...)
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err = cmd.Run()
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		os.Exit(exit.ExitCode())
	case err != nil:
		fail(fmt.Sprintf(lang.L("✗ 启动 %s 失败：%v", "✗ Could not start %s: %v"), tool, err))
	}
}

// nookArgs returns the arguments a tool starts with in Nook's terminals:
// Codex without its shared background server (--no-daemon), which would
// outlive the terminal, out of Nook's reach, and do the tool's networking
// where Nook does not see it.
func nookArgs(tool string, args []string) []string {
	if tool == "codex" && !slices.Contains(args, "--no-daemon") {
		return append([]string{"--no-daemon"}, args...)
	}
	return args
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, red+msg+reset)
	os.Exit(127)
}

// check decides whether tool may start, by the settings and what this
// computer shows, says why, a line per finding, and returns the
// environment to start it with. Messages go to stderr, so that a tool's
// output piped elsewhere stays its own.
//
// The tool goes through the proxy that runs now: the terminal's own, or,
// when the terminal opened before the proxy ran, the one found, given to
// the tool. With "no proxy, no AI" on (RequireProxy), no proxy stops the
// tool, unless the internet goes through a tunnel. Off, nothing stops it.
func check(tool string, s config.Settings, env []string) (bool, []string, []string) {
	if s.Locked {
		return false, []string{
			red + lang.L("✗ 联网锁锁着，", "✗ The network lock is on: ") + tool + lang.L(" 没有启动", " did not start") + reset,
			gray + lang.L("  点 Nook 顶部的锁就能解开", "  Click the lock at the top of Nook to unlock it") + reset,
		}, env
	}
	if own := netguard.EnvProxy(env); own.Works() {
		return true, []string{
			green + lang.L("✓ 代理 ", "✓ Proxy ") + own.Addr() + lang.L(" 已开", " is on") + reset,
			green + lang.L("✓ 放行，", "✓ Allowed: ") + tool + lang.L(" 启动", " starts") + reset,
		}, env
	}
	p := netguard.DetectProxy(s.Proxy)
	if p.Works() {
		return true, []string{
			green + lang.L("✓ 代理 ", "✓ Proxy ") + p.Addr() + lang.L(" 已开", " is on") + reset,
			green + lang.L("✓ 放行，已给 ", "✓ Allowed: started ") + tool + lang.L(" 带上代理", " with the proxy") + reset,
		}, withProxy(env, p.URL())
	}
	// A proxy in the terminal that is down would only make the tool fail.
	env = without(env, netguard.ProxyVars)
	if !s.Requires() {
		return true, []string{gray + lang.L("· 没有代理，直接联网", "· No proxy: connecting directly") + reset}, env
	}
	if tun := netguard.Carrying(netguard.Tunnels()); len(tun) > 0 {
		return true, []string{
			green + lang.L("✓ 虚拟网卡 ", "✓ Tunnel ") + tun[0].Name + lang.L(" 已开", " is on") + reset,
			green + lang.L("✓ 放行，", "✓ Allowed: ") + tool + lang.L(" 启动", " starts") + reset,
		}, env
	}
	why := red + lang.L("✗ 没找到代理，上网也不走虚拟网卡", "✗ No proxy found, and the internet does not go through a tunnel") + reset
	if p.Port != 0 {
		why = red + lang.L("✗ 代理 ", "✗ Proxy ") + p.Addr() + lang.L(" 没开", " is off") + reset
	}
	return false, []string{
		why,
		red + lang.L("✗ 已拦下：", "✗ Stopped: ") + tool + lang.L(" 没有启动，也没有联网", " did not start and connected nowhere") + reset,
		gray + lang.L("  开好代理再输入一次就行。在国外不用代理的话，到 Nook 的联网面板里关掉「没有代理时，不让 AI 启动」", "  Turn the proxy on and type it again. Outside China without a proxy, turn off “No proxy, no AI” in Nook's network panel") + reset,
	}, env
}

// withProxy returns env with the proxy at url in place of any other.
func withProxy(env []string, url string) []string {
	env = append(without(env, netguard.ProxyVars), "HTTP_PROXY="+url, "HTTPS_PROXY="+url)
	if len(without(env, []string{"NO_PROXY"})) == len(env) {
		env = append(env, "NO_PROXY=localhost,127.0.0.1,::1")
	}
	return env
}

// without returns env less the variables named keys, in any case, as
// Windows names them.
func without(env, keys []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		if !slices.ContainsFunc(keys, func(key string) bool { return strings.EqualFold(k, key) }) {
			out = append(out, kv)
		}
	}
	return out
}

// findReal finds the tool in PATH, as the shell would, outside the
// guard's own folder.
func findReal(tool, selfDir string) string {
	exts := strings.Split(strings.ToLower(os.Getenv("PATHEXT")), ";")
	if len(exts) == 0 || exts[0] == "" {
		exts = []string{".com", ".exe", ".bat", ".cmd"}
	}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		dir = strings.Trim(dir, `"`)
		if dir == "" || strings.EqualFold(filepath.Clean(dir), filepath.Clean(selfDir)) {
			continue
		}
		for _, ext := range exts {
			if ext == "" {
				continue
			}
			p := filepath.Join(dir, tool+ext)
			if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
				return p
			}
		}
	}
	return ""
}
