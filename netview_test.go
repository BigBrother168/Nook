package main

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"nook/core/lang"
	"nook/core/netguard"
)

// The panel's switch keeps its setting until clicked, and a click sets it.
func TestRequireSwitch(t *testing.T) {
	a := testApp()
	a.st.SetRequireProxy(true)
	a.requireSel = true
	a.netOpen = true
	tt := ui.NewTester(a.view, 1200, 800)
	for range 5 {
		tt.Frame()
	}
	if !a.st.Requires() {
		t.Fatal("the switch went off without a click")
	}
	if err := tt.Click(requireLabel()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.st.Requires() {
		t.Fatal("still on after a click")
	}
}

// Without a tunnel, the panel suggests one; with one, it says so.
func TestTunTip(t *testing.T) {
	a := testApp()
	a.netOpen = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if !tt.HasText(tunTip(false)) {
		t.Fatalf("no tunnel: no suggestion in %q", tt.Texts())
	}
	a.net.tunnels = []netguard.Adapter{{Name: "singbox_tun", Internet: true}}
	tt.Frame()
	if !tt.HasText("已开 · singbox_tun") || tt.HasText(tunTip(false)) {
		t.Fatalf("tunnel on: %q", tt.Texts())
	}
}

func TestRoutesText(t *testing.T) {
	cases := map[netguard.Routes]string{
		{}:              "暂时没有连接",
		{3, 0, 0, 0, 0}: "3 条连接，都走代理",
		{0, 2, 0, 0, 0}: "2 条连接，都走虚拟网卡",
		{2, 1, 0, 0, 0}: "3 条连接，都走代理或虚拟网卡",
		{2, 0, 0, 0, 1}: "3 条连接：走代理 2、直连 1",
		{0, 0, 1, 1, 0}: "2 条连接：连本机 1、连局域网 1",
	}
	for r, want := range cases {
		if got := routesText(r); got != want {
			t.Errorf("%v: %q, want %q", r, got, want)
		}
	}
	lang.English = true
	defer func() { lang.English = false }()
	for r, want := range map[netguard.Routes]string{
		{1, 0, 0, 0, 0}: "1 connection, all through the proxy",
		{3, 0, 0, 0, 0}: "3 connections, all through the proxy",
		{2, 0, 0, 0, 1}: "3 connections: proxy 2, direct 1",
	} {
		if got := routesText(r); got != want {
			t.Errorf("%v: %q, want %q", r, got, want)
		}
	}
}

// The panel says first whether the real IP shows; the top bar too.
func TestIPVerdict(t *testing.T) {
	a := testApp()
	a.netOpen = true
	tt := ui.NewTester(a.view, 1300, 900)
	tt.Frame()
	if !tt.HasText("现在没有 AI 在联网") || !tt.HasText("没有 AI 在运行") {
		t.Fatalf("nothing running: %q", tt.Texts())
	}

	a.openTab("", a.defaultShell())
	s := a.focused()
	a.viewing = "designer" // the fake terminal has nothing to draw
	a.net.ai = map[int]aiUse{s.id: {tools: []string{"codex"}, routes: netguard.Routes{netguard.ViaProxy: 3}}}
	a.net.others = []otherAI{{pid: 9, name: "codex.exe", parent: "ChatGPT.exe", routes: netguard.Routes{netguard.ViaProxy: 1}}}
	tt.Frame()
	if !tt.HasText("✓ 真实 IP 没露出去") || !tt.HasText("AI 都走代理 · 1") {
		t.Fatalf("all through the proxy: %q", tt.Texts())
	}

	a.net.others[0].routes[netguard.Direct] = 1
	tt.Frame()
	if !tt.HasText("⚠ 真实 IP 可能露出去了") || !tt.HasText("其他 AI 有直连 · 1 条") || !tt.HasText("codex · ChatGPT 桌面版：1 条连接没走代理") {
		t.Fatalf("direct elsewhere: %q", tt.Texts())
	}

	a.net.ai[s.id] = aiUse{tools: []string{"codex"}, routes: netguard.Routes{netguard.ViaProxy: 3, netguard.Direct: 2}}
	tt.Frame()
	if !tt.HasText("AI 有直连 · 2 条") || !tt.HasText("codex · "+a.projectLabel(s)+"：2 条连接没走代理") || !tt.HasText("结束 Nook 里的 AI") {
		t.Fatalf("direct in Nook: %q", tt.Texts())
	}
}
