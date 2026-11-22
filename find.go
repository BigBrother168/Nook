package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

// Finding text in a terminal, its scrollback too (Ctrl+Shift+F, as in
// Windows Terminal; Ctrl+F stays the shell's): a bar over the terminal,
// the matches lit as they are found.

// findState is a terminal's find bar, while it shows.
type findState struct {
	open bool
	text string
}

// openFind shows the focused terminal's find bar, ready to type.
func (a *app) openFind() {
	if s := a.focused(); s != nil {
		s.find.open = true
		if s.find.text != "" && s.term != nil {
			s.term.Search(s.find.text)
		}
	}
}

// closeFind hides s's find bar, ends its search and gives it the keys.
func (a *app) closeFind(s *session) {
	s.find.open = false
	if s.term != nil {
		s.term.EndSearch()
	}
	a.pendingFocus = s
}

// findBar is the bar of s's search: what to find, how many found, older
// and newer, close.
func (a *app) findBar(c *ui.Context, s *session) {
	t := c.Theme()
	ui.Row(c).FillWidth().Height(34).PaddingX(8).Gap(4).AlignItems(ui.Center).Background(t.Surface).Border(1, t.Border).Children(func() {
		in := ui.TextInput(c, &s.find.text).Placeholder(L("在终端里找…", "Find in the terminal…")).AutoFocus().Grow(1).Shrink(1).
			OnTextKey(func(k ui.TextKey) bool {
				switch {
				case k.Key == ui.KeyEnter && k.Mods&ui.Shift != 0:
					s.term.SearchPrevious()
				case k.Key == ui.KeyEnter:
					s.term.SearchNext()
				case k.Key == ui.KeyEscape:
					a.closeFind(s)
				default:
					return false
				}
				return true
			})
		if in.Changed() && s.term != nil {
			s.term.Search(s.find.text)
		}
		if s.term != nil {
			ui.Text(c, findCount(s.term.SearchStatus().Text, s.term.SearchStatus().Total, s.term.SearchStatus().Selected, s.term.SearchStatus().Searching)).
				FontSize(12).TextColor(t.TextMuted).Shrink(0)
		}
		if iconButton(c, iconChevronUp, L("上一个（更早的，Enter）", "Previous, older (Enter)")).Clicked() && s.term != nil {
			s.term.SearchNext()
		}
		if iconButton(c, iconChevron, L("下一个（更新的，Shift+Enter）", "Next, newer (Shift+Enter)")).Clicked() && s.term != nil {
			s.term.SearchPrevious()
		}
		if iconButton(c, iconX, L("关闭（Esc）", "Close (Esc)")).Clicked() {
			a.closeFind(s)
		}
	})
}

// findCount says how a search goes: "2/5", "找中…", "没找到".
func findCount(text string, total, selected int, searching bool) string {
	switch {
	case text == "":
		return ""
	case total == 0 && searching:
		return L("找中…", "Finding…")
	case total == 0:
		return L("没找到", "Not found")
	case selected == 0:
		return fmt.Sprintf("%d", total)
	}
	return fmt.Sprintf("%d/%d", selected, total)
}
