package main

import (
	"nook/core/config"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Nook 0.2 kept the whole list of AI tools, which then missed the tools
// Nook learned later: only the user's own are kept now.
func TestAIToolsGrowWithNook(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AppData", dir)
	old := `{"projects":[],"aiTools":["claude","codex","gemini","mytool"]}`
	os.MkdirAll(filepath.Join(dir, "Nook"), 0o755)
	if err := os.WriteFile(filepath.Join(dir, "Nook", "state.json"), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	s := config.Load()
	if !slices.Contains(s.AITools, "agy") || !slices.Contains(s.AITools, "mytool") {
		t.Fatalf("AITools = %v, want the defaults (agy too) and mytool", s.AITools)
	}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "Nook", "state.json"))
	if strings.Contains(string(b), `"aiTools"`) || !strings.Contains(string(b), `"moreAITools": [`) {
		t.Fatalf("saved %s, want only moreAITools", b)
	}
	if got := config.Load().AITools; !slices.Equal(got, s.AITools) {
		t.Fatalf("reloaded %v, want %v", got, s.AITools)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AppData", dir)
	if s := config.Load(); len(s.Projects) != 0 {
		t.Fatalf("fresh settings have projects: %+v", s)
	}
	s := config.Settings{Projects: []config.Project{{Name: "Nook", Path: `E:\Nook`}}, DefaultShell: "pwsh", LastProject: `E:\Nook`}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "Nook", "state.json")); err != nil {
		t.Fatal(err)
	}
	got := config.Load()
	if len(got.Projects) != 1 || got.Projects[0].Path != `E:\Nook` || got.LastProject != `E:\Nook` {
		t.Fatalf("got %+v", got)
	}
}
