package history

import (
	"os"
	"path/filepath"
	"time"

	"nook/core/lang"
)

// agyDir is the Antigravity CLI's folder.
func agyDir() string { return filepath.Join(home(), ".gemini", "antigravity-cli") }

// findAgy returns the conversation of an agy that started at since in
// dir: agy keeps the last one of each folder, and marks a conversation it
// opens in presence/<id>.lock.
func findAgy(dir string, since time.Time, taken map[string]bool) string {
	var last map[string]string
	if !readJSON(filepath.Join(agyDir(), "cache", "last_conversations.json"), &last) {
		return ""
	}
	for d, id := range last {
		if !sameDir(d, dir) || !validID.MatchString(id) || taken[Ref{"agy", id}.Key()] {
			continue
		}
		if info, err := os.Stat(filepath.Join(agyDir(), "presence", id+".lock")); err == nil && !info.ModTime().Before(since.Add(-slack)) {
			return id
		}
	}
	return ""
}

// agySummary returns the title and the summary agy keeps of conversation id.
func agySummary(id string) (title, preview string) {
	var meta struct {
		Conversations map[string]struct {
			Summary struct {
				Title   string `json:"Title"`
				Preview string `json:"Preview"`
			} `json:"summary"`
		} `json:"conversations"`
	}
	if !readJSON(filepath.Join(agyDir(), "cache", "conversation_metadata.json"), &meta) {
		return "", ""
	}
	s := meta.Conversations[id].Summary
	if s.Title == "" {
		s.Title = clip(s.Preview, 80)
	}
	return s.Title, s.Preview
}

// agyMessages is what Nook shows of an agy conversation: agy keeps what
// was said in a database of its own, which Nook does not read, so only
// its summary.
func agyMessages(id string) ([]Message, error) {
	if _, err := os.Stat(filepath.Join(agyDir(), "conversations", id+".db")); err != nil {
		return nil, os.ErrNotExist
	}
	msgs := []Message{{Role: "note", Text: lang.L("agy 把对话内容存在它自己的数据库里，Nook 暂时只显示它的摘要。点“继续”可以在 agy 里看完整对话。", "agy keeps what was said in a database of its own; Nook shows its summary for now. Click “Continue” to see it all in agy.")}}
	if _, preview := agySummary(id); preview != "" {
		msgs = append(msgs, Message{Role: "ai", Text: preview})
	}
	return msgs, nil
}
