// Package history reads what AI command-line tools keep of their
// conversations on this computer: which conversation a running tool is
// in, its title, and what was said. It only reads files; it never writes
// them, never starts a tool and never connects anywhere.
package history

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"nook/core/lang"
)

// Ref names a conversation of a tool.
type Ref struct {
	Tool string `json:"tool"` // claude, codex or agy
	ID   string `json:"id"`
}

// Key is r as one string, for maps.
func (r Ref) Key() string { return r.Tool + ":" + r.ID }

// Message is one thing said in a conversation.
type Message struct {
	Role string // "user", "ai", "tool" (a tool the AI used) or "note" (Nook's)
	Text string
}

// Names returns a tool's name as people know it.
func Names(tool string) string {
	switch tool {
	case "claude":
		return "Claude Code"
	case "codex":
		return "Codex"
	case "agy":
		return "Antigravity"
	}
	if n, ok := toolNames[tool]; ok {
		return n
	}
	return tool
}

// toolNames are the names of the other AI tools Nook knows
// (config.DefaultAITools), whose conversations it does not read.
var toolNames = map[string]string{
	"gemini":       "Gemini CLI",
	"opencode":     "OpenCode",
	"qwen":         "Qwen Code",
	"kimi":         "Kimi CLI",
	"grok":         "Grok CLI",
	"crush":        "Crush",
	"aider":        "Aider",
	"copilot":      "Copilot CLI",
	"cursor-agent": "Cursor Agent",
	"amp":          "Amp",
	"droid":        "Droid",
	"goose":        "Goose",
	"iflow":        "iFlow CLI",
}

// Resume returns the command that continues r in its tool, "" for a tool
// Nook does not know.
func Resume(r Ref) string {
	if !validID.MatchString(r.ID) {
		return ""
	}
	switch r.Tool {
	case "claude":
		return "claude --resume " + r.ID
	case "codex":
		return "codex resume " + r.ID
	case "agy":
		return "agy --conversation " + r.ID
	}
	return ""
}

// Find returns the conversation a tool running in a terminal is in: pids
// are the tool's processes, dir the terminal's folder, since when the tool
// started; taken are conversations other terminals hold. The zero Ref
// when it cannot tell yet, as before the tool opens one.
func Find(tool string, pids []uint32, dir string, since time.Time, taken map[string]bool) Ref {
	var id string
	switch tool {
	case "claude":
		id = findClaude(pids)
	case "codex":
		id = findCodex(dir, since, taken)
	case "agy":
		id = findAgy(dir, since, taken)
	}
	if !validID.MatchString(id) {
		return Ref{}
	}
	return Ref{Tool: tool, ID: id}
}

// Title returns r's title as its tool keeps it, else its first question;
// "" when there is neither.
func Title(r Ref) string {
	if !validID.MatchString(r.ID) {
		return ""
	}
	switch r.Tool {
	case "claude":
		_, title := claudeRead(r.ID)
		return title
	case "codex":
		_, title := codexRead(r.ID)
		return title
	case "agy":
		title, _ := agySummary(r.ID)
		return title
	}
	return ""
}

// Messages returns what was said in r, in order.
func Messages(r Ref) ([]Message, error) {
	if !validID.MatchString(r.ID) {
		return nil, os.ErrNotExist
	}
	switch r.Tool {
	case "claude":
		msgs, _ := claudeRead(r.ID)
		if msgs == nil {
			return nil, os.ErrNotExist
		}
		return msgs, nil
	case "codex":
		msgs, _ := codexRead(r.ID)
		if msgs == nil {
			return nil, os.ErrNotExist
		}
		return msgs, nil
	case "agy":
		return agyMessages(r.ID)
	}
	return nil, os.ErrNotExist
}

// validID is a conversation ID as the tools make them: a UUID, which also
// keeps it from meaning anything in a file pattern or a command line.
var validID = regexp.MustCompile(`^[A-Za-z0-9-]{8,64}$`)

func home() string {
	h, _ := os.UserHomeDir()
	return h
}

// sameDir reports whether two paths name one folder, as Windows compares
// them.
func sameDir(a, b string) bool {
	return a != "" && b != "" && strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// eachLine calls fn with each line of a JSON Lines file, however long.
func eachLine(path string, fn func(line []byte)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 64<<10)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			fn(line)
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// firstLine returns the first line of a file.
func firstLine(path string) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	line, _ := bufio.NewReaderSize(f, 64<<10).ReadBytes('\n')
	return line
}

// readJSON reads a JSON file into v.
func readJSON(path string, v any) bool {
	b, err := os.ReadFile(path)
	return err == nil && json.Unmarshal(b, v) == nil
}

// injected reports whether text is what a tool put in the user's turn
// itself, as its instructions and context, rather than what the user said.
func injected(text string) bool {
	t := strings.TrimSpace(text)
	return strings.HasPrefix(t, "<") || strings.HasPrefix(t, "# AGENTS.md")
}

// firstUserLine returns the first line of the first thing the user said.
func firstUserLine(msgs []Message) string {
	for _, m := range msgs {
		if m.Role == "user" {
			line, _, _ := strings.Cut(strings.TrimSpace(m.Text), "\n")
			return clip(line, 80)
		}
	}
	return ""
}

// clip shortens s to n characters.
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// toolLine is a tool the AI used, as one line: its name and what it worked
// on, as a file, a command or a pattern.
func toolLine(name string, input map[string]any) string {
	for _, k := range []string{"description", "file_path", "path", "command", "cmd", "pattern", "url", "query"} {
		if v, ok := input[k].(string); ok && v != "" {
			if s, ok := scriptCall(v); ok {
				return s
			}
			line, _, _ := strings.Cut(v, "\n")
			return name + "  " + clip(line, 90)
		}
	}
	return name
}

// Codex runs its tools from scripts, as
// text(await tools.exec_command({cmd:"Get-Item …"})): the command, or the
// files a patch changes, say more than the script's first line.
var (
	scriptCmd   = regexp.MustCompile(`exec_command\(\{\s*cmd\s*:\s*("(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'|` + "`[^`]*`)")
	scriptPatch = regexp.MustCompile(`\*\*\* (?:Update|Add|Delete) File: (.+?)(?:\\n|\n|$)`)
)

// scriptCall is the one line of a tool script of Codex's: "$ " and the
// command it runs, or the files it patches.
func scriptCall(v string) (string, bool) {
	if m := scriptCmd.FindStringSubmatch(v); m != nil {
		cmd := m[1]
		if s, err := strconv.Unquote(cmd); err == nil && cmd[0] == '"' {
			cmd = s
		} else {
			cmd = strings.ReplaceAll(cmd[1:len(cmd)-1], `\`, `\`)
		}
		line, _, _ := strings.Cut(strings.TrimSpace(cmd), "\n")
		return "$ " + clip(line, 90), true
	}
	if strings.Contains(v, "apply_patch") {
		var names []string
		for _, m := range scriptPatch.FindAllStringSubmatch(v, -1) {
			p := strings.ReplaceAll(strings.TrimSpace(m[1]), `\`, `\`)
			if n := filepath.Base(p); !slices.Contains(names, n) {
				names = append(names, n)
			}
		}
		if len(names) > 0 {
			return lang.L("改了 ", "edited ") + clip(strings.Join(names, "、"), 90), true
		}
	}
	return "", false
}
