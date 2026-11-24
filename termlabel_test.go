package main

import (
	"testing"
	"time"

	"nook/core/history"
)

func TestTermLabel(t *testing.T) {
	a := testApp()
	a.st.Projects = nil
	s := &session{id: 7, proj: `E:\Nook`, dir: `E:\Nook`, shell: shell{Name: "PowerShell"}, title: "Nook"}
	if got := a.termLabel(s); got != "Nook" {
		t.Errorf("no AI: %q", got)
	}
	a.net.ai = map[int]aiUse{7: {tools: []string{"agy"}}}
	if got := a.termLabel(s); got != "Antigravity" {
		t.Errorf("agy titled with the folder: %q", got)
	}
	a.net.ai[7] = aiUse{tools: []string{"claude"}}
	s.title = "✳ Claude Code"
	if got := a.termLabel(s); got != "Claude Code" {
		t.Errorf("claude's own title: %q", got)
	}
	s.title = "✳ 修登录的 bug"
	if got := a.termLabel(s); got != "Claude Code · ✳ 修登录的 bug" {
		t.Errorf("claude with a task: %q", got)
	}
}

func TestTermLabelApart(t *testing.T) {
	a := testApp()
	mk := func(id int) *session {
		s := &session{id: id, proj: `E:\Nook`, dir: `E:\Nook`, shell: shell{Name: "PowerShell"}, title: "Nook"}
		a.tabs = append(a.tabs, &tab{id: id, root: &pane{id: id, s: s}})
		return s
	}
	s1, s2, s3 := mk(1), mk(2), mk(3)
	a.net.ai = map[int]aiUse{1: {tools: []string{"agy"}}, 2: {tools: []string{"codex"}}, 3: {tools: []string{"agy"}}}
	if got := a.termLabel(s1) + "|" + a.termLabel(s2) + "|" + a.termLabel(s3); got != "Antigravity|Codex|Antigravity (2)" {
		t.Errorf("no titles yet: %q", got)
	}
	// Once the conversations have titles, they tell the terminals apart.
	now := time.Now()
	for id, s := range map[string]*session{"a1": s1, "a3": s3} {
		s.conv = history.Ref{Tool: "agy", ID: id}
		a.convs.live = map[string]time.Time{"agy:a1": now, "agy:a3": now}
	}
	a.convs.titles = map[string]string{"agy:a1": "你好，你是什么模型", "agy:a3": "把 README 改成英文版本再加一节安装说明"}
	if got := a.termLabel(s1); got != "Antigravity · 你好，你是什么模型" {
		t.Errorf("first: %q", got)
	}
	if got := a.termLabel(s3); got != "Antigravity · 把 README 改成英文版本再加一…" {
		t.Errorf("third: %q", got)
	}
}

// A number stays with its terminal when it is dragged elsewhere.
func TestTermNumberStays(t *testing.T) {
	a := testApp()
	var tabs []*tab
	for id := 1; id <= 3; id++ {
		s := &session{id: id, proj: `E:\Nook`, dir: `E:\Nook`, shell: shell{Name: "PowerShell"}, title: "Nook"}
		tabs = append(tabs, &tab{id: id, root: &pane{id: id, s: s}, focus: s})
	}
	a.tabs = append([]*tab{}, tabs...)
	a.net.ai = map[int]aiUse{1: {tools: []string{"codex"}}, 2: {tools: []string{"codex"}}, 3: {tools: []string{"codex"}}}
	a.moveTab(tabs[2], tabs[0]) // "Codex (3)" dragged to the top
	got := ""
	for _, tb := range a.tabs {
		got += a.termLabel(tb.focus) + "|"
	}
	if got != "Codex (3)|Codex|Codex (2)|" {
		t.Errorf("after the drag: %s", got)
	}
}
