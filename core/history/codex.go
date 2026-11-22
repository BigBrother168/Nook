package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"nook/core/lang"
)

// codexDir is Codex's folder: ~/.codex, or CODEX_HOME.
func codexDir() string {
	if d := os.Getenv("CODEX_HOME"); d != "" {
		return d
	}
	return filepath.Join(home(), ".codex")
}

// codexMeta is the first line of a Codex conversation's file.
type codexMeta struct {
	ID      string
	Cwd     string
	Created time.Time
}

// metas keeps the first lines read, which never change, by file.
var metas sync.Map

func readCodexMeta(path string) (codexMeta, bool) {
	if m, ok := metas.Load(path); ok {
		return m.(codexMeta), true
	}
	var l struct {
		Type    string `json:"type"`
		Payload struct {
			ID        string `json:"id"`
			Cwd       string `json:"cwd"`
			Timestamp string `json:"timestamp"`
		} `json:"payload"`
	}
	if json.Unmarshal(firstLine(path), &l) != nil || l.Type != "session_meta" || l.Payload.ID == "" {
		return codexMeta{}, false
	}
	created, _ := time.Parse(time.RFC3339Nano, l.Payload.Timestamp)
	m := codexMeta{ID: l.Payload.ID, Cwd: l.Payload.Cwd, Created: created}
	metas.Store(path, m)
	return m, true
}

// slack is how much earlier than the tool's start a file may say it was
// written, as clocks and file times round.
const slack = 2 * time.Second

// findCodex returns the conversation of a codex that started at since in
// dir. Codex names none by process, so it is the conversation of dir
// written since: a new one first, else one continued.
func findCodex(dir string, since time.Time, taken map[string]bool) string {
	root := codexDir()
	type cand struct {
		id      string
		created time.Time
		mod     time.Time
	}
	var cands []cand
	seen := map[string]bool{}
	consider := func(path string, mod time.Time) {
		if mod.Before(since.Add(-slack)) {
			return
		}
		m, ok := readCodexMeta(path)
		if !ok || seen[m.ID] || taken[Ref{"codex", m.ID}.Key()] || !sameDir(m.Cwd, dir) {
			return
		}
		seen[m.ID] = true
		cands = append(cands, cand{m.ID, m.Created, mod})
	}
	// New conversations: sessions/YYYY/MM/DD, by local date, since it
	// started (the last three days at most).
	now := time.Now()
	from := since.Add(-time.Hour)
	if from.Before(now.Add(-72 * time.Hour)) {
		from = now.Add(-72 * time.Hour)
	}
	for d := from; !d.After(now.Add(24 * time.Hour)); d = d.Add(24 * time.Hour) {
		day := filepath.Join(root, "sessions", d.Format("2006"), d.Format("01"), d.Format("02"))
		entries, _ := os.ReadDir(day)
		for _, e := range entries {
			if !strings.HasPrefix(e.Name(), "rollout-") || !strings.HasSuffix(e.Name(), ".jsonl") {
				continue
			}
			if info, err := e.Info(); err == nil {
				consider(filepath.Join(day, e.Name()), info.ModTime())
			}
		}
	}
	// Continued ones, kept where they began: the index says which changed.
	for id, updated := range codexIndexSince(since.Add(-slack)) {
		if path := codexFile(id); path != "" {
			if info, err := os.Stat(path); err == nil && info.ModTime().After(updated) {
				updated = info.ModTime()
			}
			consider(path, updated)
		}
	}
	var best cand
	for _, c := range cands {
		isNew, bestNew := !c.created.Before(since.Add(-slack)), !best.created.Before(since.Add(-slack))
		switch {
		case best.id == "":
			best = c
		case isNew && !bestNew, isNew == bestNew && c.mod.After(best.mod):
			best = c
		}
	}
	return best.id
}

// codexIndexSince returns the conversations session_index.jsonl says
// changed since t, with when.
func codexIndexSince(t time.Time) map[string]time.Time {
	out := map[string]time.Time{}
	eachLine(filepath.Join(codexDir(), "session_index.jsonl"), func(line []byte) {
		var l struct {
			ID        string `json:"id"`
			UpdatedAt string `json:"updated_at"`
		}
		if json.Unmarshal(line, &l) != nil || !validID.MatchString(l.ID) {
			return
		}
		if u, err := time.Parse(time.RFC3339Nano, l.UpdatedAt); err == nil && !u.Before(t) {
			out[l.ID] = u
		}
	})
	return out
}

// codexFile returns where conversation id is kept.
func codexFile(id string) string {
	root := codexDir()
	for _, pattern := range []string{
		filepath.Join(root, "sessions", "*", "*", "*", "rollout-*-"+id+".jsonl"),
		filepath.Join(root, "archived_sessions", "rollout-*-"+id+".jsonl"),
	} {
		if m, _ := filepath.Glob(pattern); len(m) > 0 {
			return m[0]
		}
	}
	return ""
}

// codexRead reads conversation id: what was said, nil when it is not
// there, and its title, the thread's name else the first question.
func codexRead(id string) ([]Message, string) {
	path := codexFile(id)
	if path == "" {
		return nil, ""
	}
	msgs := []Message{}
	eachLine(path, func(line []byte) {
		var l struct {
			Type    string `json:"type"`
			Payload struct {
				Type      string `json:"type"`
				Role      string `json:"role"`
				Name      string `json:"name"`
				Input     string `json:"input"`
				Arguments string `json:"arguments"`
				Content   []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"payload"`
		}
		if json.Unmarshal(line, &l) != nil || l.Type != "response_item" {
			return
		}
		p := l.Payload
		switch p.Type {
		case "message":
			var parts []string
			for _, c := range p.Content {
				switch {
				case c.Type == "input_image":
					parts = append(parts, lang.L("[图片]", "[image]"))
				case (c.Type == "input_text" || c.Type == "output_text") && !injected(c.Text):
					parts = append(parts, strings.TrimSpace(c.Text))
				}
			}
			text := strings.TrimSpace(strings.Join(parts, "\n"))
			switch {
			case text == "":
			case p.Role == "user":
				msgs = append(msgs, Message{Role: "user", Text: text})
			case p.Role == "assistant":
				msgs = append(msgs, Message{Role: "ai", Text: text})
			}
		case "function_call", "custom_tool_call", "local_shell_call":
			var input map[string]any
			json.Unmarshal([]byte(p.Arguments), &input)
			if input == nil && p.Input != "" {
				input = map[string]any{"command": p.Input}
			}
			msgs = append(msgs, Message{Role: "tool", Text: toolLine(p.Name, input)})
		}
	})
	title := ""
	eachLine(filepath.Join(codexDir(), "session_index.jsonl"), func(line []byte) {
		var l struct {
			ID         string `json:"id"`
			ThreadName string `json:"thread_name"`
		}
		if json.Unmarshal(line, &l) == nil && l.ID == id && l.ThreadName != "" {
			title = l.ThreadName
		}
	})
	if title == "" {
		title = firstUserLine(msgs)
	}
	return msgs, title
}
