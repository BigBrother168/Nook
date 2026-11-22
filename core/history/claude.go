package history

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"nook/core/lang"
)

// claudeDir is Claude Code's folder: ~/.claude, or CLAUDE_CONFIG_DIR.
func claudeDir() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d
	}
	return filepath.Join(home(), ".claude")
}

// findClaude returns the conversation of a claude among pids: a running
// claude names its own in sessions/<pid>.json.
func findClaude(pids []uint32) string {
	dir := filepath.Join(claudeDir(), "sessions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	have := map[string]bool{}
	for _, e := range entries {
		have[e.Name()] = true
	}
	for _, pid := range pids {
		name := strconv.FormatUint(uint64(pid), 10) + ".json"
		if !have[name] {
			continue
		}
		var f struct {
			SessionID string `json:"sessionId"`
		}
		if readJSON(filepath.Join(dir, name), &f) && f.SessionID != "" {
			return f.SessionID
		}
	}
	return ""
}

// claudeFile returns where conversation id is kept:
// projects/<folder>/<id>.jsonl.
func claudeFile(id string) string {
	m, _ := filepath.Glob(filepath.Join(claudeDir(), "projects", "*", id+".jsonl"))
	if len(m) == 0 {
		return ""
	}
	return m[0]
}

// claudeRead reads conversation id: what was said, nil when it is not
// there, and its title, the user's (/rename) over Claude Code's own.
func claudeRead(id string) ([]Message, string) {
	path := claudeFile(id)
	if path == "" {
		return nil, ""
	}
	msgs := []Message{}
	var custom, auto string
	eachLine(path, func(line []byte) {
		if !bytes.Contains(line, []byte(`"type":"`)) {
			return
		}
		var l struct {
			Type        string `json:"type"`
			IsMeta      bool   `json:"isMeta"`
			IsSidechain bool   `json:"isSidechain"`
			AITitle     string `json:"aiTitle"`
			CustomTitle string `json:"customTitle"`
			Message     struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &l) != nil {
			return
		}
		switch l.Type {
		case "ai-title":
			auto = l.AITitle
		case "custom-title":
			custom = l.CustomTitle
		case "user":
			if l.IsMeta || l.IsSidechain {
				return
			}
			if text := userText(l.Message.Content); text != "" {
				msgs = append(msgs, Message{Role: "user", Text: text})
			}
		case "assistant":
			if l.IsSidechain {
				return
			}
			msgs = append(msgs, assistantMessages(l.Message.Content)...)
		}
	})
	title := custom
	if title == "" {
		title = auto
	}
	if title == "" {
		title = firstUserLine(msgs)
	}
	return msgs, title
}

// contentBlock is a part of a message, as the Messages API has them.
type contentBlock struct {
	Type  string         `json:"type"`
	Text  string         `json:"text"`
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

// userText is what the user said in a message's content, "" for what a
// tool returned or Claude Code added.
func userText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if injected(s) {
			return ""
		}
		return strings.TrimSpace(s)
	}
	var blocks []contentBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if !injected(b.Text) {
				parts = append(parts, strings.TrimSpace(b.Text))
			}
		case "image":
			parts = append(parts, lang.L("[图片]", "[image]"))
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

// assistantMessages is what the AI said in a message's content, and the
// tools it used.
func assistantMessages(raw json.RawMessage) []Message {
	var blocks []contentBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return nil
	}
	var out []Message
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if t := strings.TrimSpace(b.Text); t != "" {
				out = append(out, Message{Role: "ai", Text: t})
			}
		case "tool_use":
			out = append(out, Message{Role: "tool", Text: toolLine(b.Name, b.Input)})
		}
	}
	return out
}
