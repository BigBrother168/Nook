package history

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// fakeHome makes a home folder of its own for the tools' files.
func fakeHome(t *testing.T) string {
	h := t.TempDir()
	t.Setenv("USERPROFILE", h)
	t.Setenv("HOME", h)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CODEX_HOME", "")
	return h
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const claudeID = "11111111-2222-3333-4444-555555555555"

func TestClaude(t *testing.T) {
	h := fakeHome(t)
	write(t, filepath.Join(h, ".claude", "sessions", "4242.json"), `{"pid":4242,"sessionId":"`+claudeID+`","cwd":"E:\\Nook"}`)
	write(t, filepath.Join(h, ".claude", "projects", "E--Nook", claudeID+".jsonl"), `{"type":"user","message":{"role":"user","content":"<command-name>/model</command-name>"}}
{"type":"user","isMeta":true,"message":{"role":"user","content":"Caveat: added by Claude Code"}}
{"type":"user","message":{"role":"user","content":[{"type":"text","text":"看一下 main.go"},{"type":"image","source":{}}]}}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"thinking","thinking":"…"},{"type":"text","text":"好的。"},{"type":"tool_use","name":"Read","input":{"file_path":"E:\\Nook\\main.go"}}]}}
{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"package main"}]}}
{"type":"assistant","isSidechain":true,"message":{"role":"assistant","content":[{"type":"text","text":"a subagent's"}]}}
{"type":"ai-title","aiTitle":"读 main.go"}
`)
	if got := Find("claude", []uint32{7, 4242}, `E:\Nook`, time.Now(), nil); got != (Ref{"claude", claudeID}) {
		t.Fatalf("Find = %v", got)
	}
	if got := Find("claude", []uint32{7}, `E:\Nook`, time.Now(), nil); got.ID != "" {
		t.Fatalf("Find without its process = %v", got)
	}
	msgs, err := Messages(Ref{"claude", claudeID})
	if err != nil {
		t.Fatal(err)
	}
	want := []Message{
		{"user", "看一下 main.go\n[图片]"},
		{"ai", "好的。"},
		{"tool", `Read  E:\Nook\main.go`},
	}
	if !slices.Equal(msgs, want) {
		t.Fatalf("Messages = %q, want %q", msgs, want)
	}
	if got := Title(Ref{"claude", claudeID}); got != "读 main.go" {
		t.Fatalf("Title = %q", got)
	}
	if got := Resume(Ref{"claude", claudeID}); got != "claude --resume "+claudeID {
		t.Fatalf("Resume = %q", got)
	}
}

func TestCodex(t *testing.T) {
	h := fakeHome(t)
	start := time.Date(2026, 11, 21, 14, 0, 0, 0, time.Local)
	day := filepath.Join(h, ".codex", "sessions", "2026", "11", "21")
	rollout := func(id, cwd string, created time.Time, rest string) string {
		path := filepath.Join(day, "rollout-2026-11-21T14-00-00-"+id+".jsonl")
		write(t, path, `{"type":"session_meta","payload":{"id":"`+id+`","cwd":"`+cwd+`","timestamp":"`+created.UTC().Format(time.RFC3339Nano)+`"}}`+"\n"+rest)
		os.Chtimes(path, created.Add(time.Minute), created.Add(time.Minute))
		return path
	}
	const mine, other, old = "aaaaaaaa-0000-0000-0000-000000000001", "aaaaaaaa-0000-0000-0000-000000000002", "aaaaaaaa-0000-0000-0000-000000000003"
	rollout(mine, `E:\\Nook`, start.Add(3*time.Second), `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"# AGENTS.md instructions"},{"type":"input_text","text":"<environment_context></environment_context>"}]}}
{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"你好"}]}}
{"type":"response_item","payload":{"type":"function_call","name":"shell","arguments":"{\"command\":\"git status\"}"}}
{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"你好！"}]}}
`)
	rollout(other, `C:\\Users\\admin`, start.Add(5*time.Second), "")
	rollout(old, `E:\\Nook`, start.Add(-time.Hour), "")
	write(t, filepath.Join(h, ".codex", "session_index.jsonl"), `{"id":"`+mine+`","thread_name":"打招呼","updated_at":"`+start.UTC().Format(time.RFC3339)+`"}`+"\n")

	if got := Find("codex", nil, `e:\nook\`, start, nil); got.ID != mine {
		t.Fatalf("Find = %v, want the new one of the folder", got)
	}
	if got := Find("codex", nil, `E:\Nook`, start, map[string]bool{Ref{"codex", mine}.Key(): true}); got.ID != "" {
		t.Fatalf("Find with it taken = %v", got)
	}
	msgs, _ := Messages(Ref{"codex", mine})
	want := []Message{{"user", "你好"}, {"tool", "shell  git status"}, {"ai", "你好！"}}
	if !slices.Equal(msgs, want) {
		t.Fatalf("Messages = %q, want %q", msgs, want)
	}
	if got := Title(Ref{"codex", mine}); got != "打招呼" {
		t.Fatalf("Title = %q", got)
	}
}

func TestAgy(t *testing.T) {
	h := fakeHome(t)
	const id = "bbbbbbbb-0000-0000-0000-000000000001"
	dir := filepath.Join(h, ".gemini", "antigravity-cli")
	write(t, filepath.Join(dir, "cache", "last_conversations.json"), `{"E:\\Nook":"`+id+`"}`)
	write(t, filepath.Join(dir, "cache", "conversation_metadata.json"), `{"conversations":{"`+id+`":{"summary":{"Title":"回应问候","Preview":"你好！"}}}}`)
	write(t, filepath.Join(dir, "presence", id+".lock"), "")
	write(t, filepath.Join(dir, "conversations", id+".db"), "")
	if got := Find("agy", nil, `E:\Nook`, time.Now().Add(-time.Minute), nil); got.ID != id {
		t.Fatalf("Find = %v", got)
	}
	if got := Find("agy", nil, `E:\Nook`, time.Now().Add(time.Hour), nil); got.ID != "" {
		t.Fatalf("Find of one opened before it started = %v", got)
	}
	if got := Title(Ref{"agy", id}); got != "回应问候" {
		t.Fatalf("Title = %q", got)
	}
	if got := Resume(Ref{"agy", id}); got != "agy --conversation "+id {
		t.Fatalf("Resume = %q", got)
	}
}

func TestBadIDs(t *testing.T) {
	fakeHome(t)
	for _, id := range []string{"", "*", `..\x`, "a b", "11111111-2222;calc"} {
		if Resume(Ref{"claude", id}) != "" {
			t.Errorf("Resume(%q) gives a command", id)
		}
		if _, err := Messages(Ref{"claude", id}); err == nil {
			t.Errorf("Messages(%q) reads", id)
		}
	}
}

func TestScriptCall(t *testing.T) {
	cases := map[string]string{
		`text(await tools.exec_command({cmd:"Get-Item -LiteralPath 'E:\Nook\a.txt' -ErrorAction Stop"}))`:            `$ Get-Item -LiteralPath 'E:\Nook\a.txt' -ErrorAction Stop`,
		"const read = await tools.exec_command({cmd:'rg -n foo'})":                                                   "$ rg -n foo",
		`text(await tools.apply_patch("*** Begin Patch\n*** Update File: E:\Nook\pelican.html\n@@\n*** End Patch"))`: "pelican.html",
		"const commands = [": "",
	}
	for in, want := range cases {
		got, ok := scriptCall(in)
		if want == "" {
			if ok {
				t.Errorf("scriptCall(%q) = %q, want none", in, got)
			}
			continue
		}
		if !strings.HasSuffix(got, want) {
			t.Errorf("scriptCall(%q) = %q, want …%q", in, got, want)
		}
	}
}

func TestNamesKnowEveryTool(t *testing.T) {
	for _, tool := range []string{"claude", "codex", "gemini", "agy", "opencode", "qwen", "kimi", "grok", "crush", "aider", "copilot", "cursor-agent", "amp", "droid", "goose", "iflow"} {
		if Names(tool) == tool {
			t.Errorf("%s has no name", tool)
		}
	}
}
