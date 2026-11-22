package main

import (
	"strings"
	"testing"

	"nook/core/netguard"
)

// ChatGPT desktop's two Codex components show as one program, each
// saying what it does; the same one twice shows once.
func TestGroupOthers(t *testing.T) {
	app := `C:\Program Files\WindowsApps\OpenAI.Codex_26.1117.4957.0_x64__2p2nqsd0c76g0\app\ChatGPT.exe`
	var two, five netguard.Routes
	two[netguard.ViaProxy], five[netguard.ViaProxy] = 2, 5
	others := []otherAI{
		{pid: 1, name: "codex.exe", parent: "ChatGPT.exe", parentPath: app, cmd: `codex.exe -c features.code_mode_host=true app-server --analytics-default-enabled`, routes: two},
		{pid: 2, name: "codex.exe", parent: "ChatGPT.exe", parentPath: app, cmd: `codex.exe exec-server --remote https://codex-cloud-environments.chatgpt.com/api`, routes: five},
		{pid: 3, name: "codex.exe", parent: "ChatGPT.exe", parentPath: app, cmd: `codex.exe app-server`, routes: two},
		{pid: 4, name: "claude.exe", parent: "pwsh.exe", routes: two},
	}
	g := groupOthers(others)
	if len(g) != 2 || !strings.Contains(g[0].name, "ChatGPT") || g[0].note == "" || g[1].name != "PowerShell" {
		t.Fatalf("groups %+v", g)
	}
	if len(g[0].tools) != 2 || g[0].tools[0].routes[netguard.ViaProxy] != 4 {
		t.Fatalf("ChatGPT's tools %+v", g[0].tools)
	}
	if r, _ := roleOf(g[0].tools[1]); !strings.Contains(r, "云端") {
		t.Fatalf("exec-server's role %q", r)
	}
}
