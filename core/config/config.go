// Package config holds Nook's settings, %APPDATA%\Nook\state.json, which
// Nook writes and its guard (cmd/nook-guard) reads.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"time"

	"nook/core/history"
)

// Project is a folder the user works in.
type Project struct {
	Name     string   `json:"name"`
	Path     string   `json:"path"`
	Previews []string `json:"previews,omitempty"` // local addresses its terminals printed, the latest first
}

// Conversation is an AI conversation in a terminal of Nook, and where to
// continue it. What was said stays in the tool's own files.
type Conversation struct {
	history.Ref
	Project string    `json:"project,omitempty"`
	Dir     string    `json:"dir"`
	Shell   string    `json:"shell,omitempty"`
	Seen    time.Time `json:"seen"` // when Nook found it in a terminal
}

// Settings are what Nook keeps between runs. Terminals are not kept:
// closing Nook ends them.
type Settings struct {
	Projects     []Project `json:"projects"`
	DefaultShell string    `json:"defaultShell"`
	LastProject  string    `json:"lastProject"`

	// RequireProxy keeps AI tools from starting without a proxy or a
	// tunnel running (没有代理时，不让 AI 启动): on where one is needed to
	// reach them, off where they are reached directly. Nil until Nook
	// first decides it, by whether this computer has one.
	RequireProxy *bool  `json:"requireProxy,omitempty"`
	Locked       bool   `json:"locked"`          // the network lock: no AI tool starts
	Proxy        string `json:"proxy,omitempty"` // host:port given by hand, else detected

	Language  string `json:"language,omitempty"` // "zh", "en", or "" for the system's
	Theme     string `json:"theme,omitempty"`    // "light", "dark", or "" for the system's
	FontSize  int    `json:"fontSize,omitempty"` // the terminals' text, in DIPs
	Onboarded bool   `json:"onboarded"`          // the first run's welcome was seen
	ToldHide  bool   `json:"toldHide,omitempty"` // putting a tab away was explained once

	Palette   string `json:"palette,omitempty"`   // the window's colors: a palette's id, "ghostty:<theme>", or "" for the system's
	Accent    string `json:"accent,omitempty"`    // "#rrggbb" over the palette's, or ""
	TermTheme string `json:"termTheme,omitempty"` // a Ghostty theme for the terminals, or "" for the palette's
	// TermFlavors keeps, for each palette left, the terminals' theme it
	// had, to have again as it is chosen again.
	TermFlavors map[string]string `json:"termFlavors,omitempty"`
	TermFont    string            `json:"termFont,omitempty"` // the terminals' font family, or "" for the system's monospaced one

	SidebarWidth  float32            `json:"sidebarWidth,omitempty"`  // in DIPs
	SidebarHidden bool               `json:"sidebarHidden,omitempty"` // the sidebar is hidden, the terminals taking its room
	Designs       []Design           `json:"designs,omitempty"`       // the user's own looks, made on the drawing board; Palette "my:<id>" chooses one
	SideWidth     float32            `json:"sideWidth,omitempty"`     // the right panel, in DIPs
	NetWidth      float32            `json:"netWidth,omitempty"`      // the network panel, in DIPs
	ViewerSplit   map[string]float32 `json:"viewerSplit,omitempty"`   // the terminals' share of the middle, by what shows beside them

	// CloseAction is what the window's × does: "tray" hides it, the
	// terminals going on; "quit" ends Nook; "" asks.
	CloseAction string `json:"closeAction,omitempty"`

	// BackupDir is where the copies for undoing the AI's changes go: ""
	// for each project's own drive (a hidden .nook folder at its root), else a folder of
	// the user's.
	BackupDir string `json:"backupDir,omitempty"`

	// AITools are the commands that are AI tools: DefaultAITools, which
	// grow with Nook, and the user's own (MoreAITools), the only ones kept.
	AITools     []string `json:"-"`
	MoreAITools []string `json:"moreAITools,omitempty"`

	// Conversations are the AI conversations the sidebar lists: those of
	// open terminals, and those of terminals open when Nook last quit. A
	// terminal the user closes takes its conversation off.
	Conversations []Conversation `json:"conversations,omitempty"`
	// ConvOrdered: Conversations are in the order the sidebar shows them,
	// the latest first unless the user dragged them, not as Nook 1.0.0
	// kept them, oldest first, sorted as shown.
	ConvOrdered bool `json:"convOrdered,omitempty"`

	// NetRule is the rule of Nook 0.2 ("proxy", "system", "none"), read
	// once into RequireProxy.
	NetRule string `json:"netRule,omitempty"`
	// OldAITools is the whole list Nook 0.2 kept, which then stopped
	// growing with Nook; read once into MoreAITools.
	OldAITools []string `json:"aiTools,omitempty"`
}

// DefaultAITools are the AI command-line tools Nook knows by name.
var DefaultAITools = []string{
	"claude", "codex", "gemini", "agy", "opencode", "qwen", "kimi", "grok", "crush",
	"aider", "copilot", "cursor-agent", "amp", "droid", "goose", "iflow",
}

// Path returns where the settings are kept.
func Path() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "Nook", "state.json")
}

// Load reads the settings, with defaults for what they lack.
func Load() Settings {
	var s Settings
	if p := Path(); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			json.Unmarshal(b, &s)
		}
	}
	// The old whole list: what it has beyond the defaults was the user's.
	for _, t := range s.OldAITools {
		if !slices.Contains(DefaultAITools, t) && !slices.Contains(s.MoreAITools, t) {
			s.MoreAITools = append(s.MoreAITools, t)
		}
	}
	s.OldAITools = nil
	if s.FontSize == 0 {
		s.FontSize = 13
	}
	s.AITools = slices.Clone(DefaultAITools)
	for _, t := range s.MoreAITools {
		if !slices.Contains(s.AITools, t) {
			s.AITools = append(s.AITools, t)
		}
	}
	if s.RequireProxy == nil && s.NetRule != "" {
		s.SetRequireProxy(s.NetRule == "proxy")
	}
	s.NetRule = ""
	return s
}

// Requires reports whether AI tools need a proxy or a tunnel to start.
func (s Settings) Requires() bool { return s.RequireProxy != nil && *s.RequireProxy }

// SetRequireProxy sets whether AI tools need a proxy or a tunnel.
func (s *Settings) SetRequireProxy(on bool) { s.RequireProxy = &on }

// Save writes the settings, through a temporary file so that a crash, or
// the guard reading them meanwhile, never sees half of them.
func (s Settings) Save() error {
	p := Path()
	if p == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// Design is a look the user made on the drawing board: the window's
// colors, the terminals', and what lies behind them.
type Design struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Dark bool   `json:"dark"` // the controls' look: dark or light

	Background string `json:"background"` // "#rrggbb", as the others
	Surface    string `json:"surface"`    // the sidebar, the panels, the tab bar
	Text       string `json:"text"`
	Muted      string `json:"muted"`
	Border     string `json:"border"`
	Accent     string `json:"accent"`
	TermBg     string `json:"termBg"`
	TermFg     string `json:"termFg"`

	// Gradient, when set, fills the window's background from Background
	// to it (through Middle, when set), at GradientAngle degrees; Radial
	// spreads it from a point instead, off the middle toward the angle.
	Gradient      string  `json:"gradient,omitempty"`
	Middle        string  `json:"middle,omitempty"`
	GradientAngle float32 `json:"gradientAngle,omitempty"`
	Radial        bool    `json:"radial,omitempty"`

	// Image is a picture behind everything, a copy in Nook's folder.
	Image        string  `json:"image,omitempty"`
	ImageOpacity float32 `json:"imageOpacity,omitempty"` // 0 to 1
	ImageBlur    float32 `json:"imageBlur,omitempty"`    // in DIPs
	// PanelOpacity is how opaque the panels and terminals are over the
	// picture or the gradient, 0.3 to 1.
	PanelOpacity float32 `json:"panelOpacity,omitempty"`

	// Parts colored apart from Surface, "" for Surface's.
	TitleBar string `json:"titleBar,omitempty"`
	Sidebar  string `json:"sidebar,omitempty"`
	TabBar   string `json:"tabBar,omitempty"`
	Panel    string `json:"panel,omitempty"` // the right panel
	// Selection is the background of the rows chosen, "" for one mixed
	// of Background and Accent.
	Selection string `json:"selection,omitempty"`

	// TermTheme is a terminal theme of Ghostty's the terminals start from
	// (its sixteen colors), TermBg and TermFg over it; "" for none.
	TermTheme string `json:"termTheme,omitempty"`

	// TermOwn: the terminals have colors of their own, not the window's.
	TermOwn bool `json:"termOwn,omitempty"`
}

// Dir is Nook's own folder, beside the settings.
func Dir() string { return filepath.Dir(Path()) }
