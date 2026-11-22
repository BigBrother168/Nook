package main

import (
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"nook/core/config"
	"nook/core/history"
)

// The sidebar's conversations: Nook lists the AI conversations of its
// terminals, and keeps those of the terminals open when it quits, for the
// next time. A terminal the user closes takes its conversation off. What
// was said stays in the tools' own files, which Nook only reads.

// track records that s holds conversation r, in place of the one it
// held before, as when the AI starts a new one.
func (a *app) track(s *session, r history.Ref) {
	if s.conv == r {
		return
	}
	old := s.conv
	s.conv = r
	a.st.Conversations = slices.DeleteFunc(a.st.Conversations, func(c config.Conversation) bool {
		return c.Ref == r || (old.ID != "" && c.Ref == old)
	})
	// The latest first: closed, it shows at the top of its history.
	a.st.Conversations = slices.Insert(a.st.Conversations, 0, config.Conversation{
		Ref: r, Project: s.proj, Dir: s.dir, Shell: s.shell.ID, Seen: time.Now(),
	})
	a.save()
}

// orderConversations puts the conversations Nook 1.0.0 kept, oldest
// first, in the order it showed them, the latest first, once: from then
// on their order is the sidebar's, as the user drags them.
func orderConversations(st *config.Settings) {
	if st.ConvOrdered {
		return
	}
	slices.SortStableFunc(st.Conversations, func(x, y config.Conversation) int { return y.Seen.Compare(x.Seen) })
	st.ConvOrdered = true
}

// moveConv puts conversation from where conversation to is, as one
// dragged in a history; Nook keeps the order.
func (a *app) moveConv(from, to history.Ref) {
	list := a.st.Conversations
	i := slices.IndexFunc(list, func(c config.Conversation) bool { return c.Ref == from })
	j := slices.IndexFunc(list, func(c config.Conversation) bool { return c.Ref == to })
	if i < 0 || j < 0 || i == j {
		return
	}
	c := list[i]
	a.st.Conversations = slices.Insert(slices.Delete(list, i, i+1), j, c)
	a.save()
}

// convDrag is a conversation dragged in the history of group, a
// project's folder or "" for the conversations of no project listed.
type convDrag struct {
	ref   history.Ref
	group string
}

// forget takes s's conversation off the list: the user closed s.
func (a *app) forget(s *session) {
	if s.conv.ID != "" {
		a.removeConv(s.conv)
	}
}

// removeConv takes conversation r off the list.
func (a *app) removeConv(r history.Ref) {
	n := len(a.st.Conversations)
	a.st.Conversations = slices.DeleteFunc(a.st.Conversations, func(c config.Conversation) bool { return c.Ref == r })
	if len(a.st.Conversations) != n {
		a.save()
	}
	if a.viewing == r.Key() {
		a.viewing = ""
	}
}

// holder returns the open terminal holding conversation r, nil for none.
func (a *app) holder(r history.Ref) *session {
	for _, s := range a.sessions() {
		if s.conv == r {
			return s
		}
	}
	return nil
}

// closedConvs returns the listed conversations no open terminal holds,
// of project proj, or with loose those of no project in the list; in the
// order kept, the latest first unless the user dragged them.
func (a *app) closedConvs(proj string, loose bool) []config.Conversation {
	var out []config.Conversation
	for _, c := range a.st.Conversations {
		mine := c.Project == proj
		if loose {
			mine = c.Project == "" || !a.hasProject(c.Project)
		}
		if mine && a.holder(c.Ref) == nil {
			out = append(out, c)
		}
	}
	return out
}

func (a *app) convByKey(key string) (config.Conversation, bool) {
	for _, c := range a.st.Conversations {
		if c.Key() == key {
			return c, true
		}
	}
	return config.Conversation{}, false
}

// resume continues conversation c in a new terminal in its folder: the
// command is typed, and the user presses Enter, which starts the AI.
func (a *app) resume(c config.Conversation) {
	if s := a.holder(c.Ref); s != nil {
		a.show(s)
		return
	}
	if a.viewing == c.Key() {
		a.viewing = "" // it goes on in its terminal
	}
	dir := dirFor(c.Dir)
	if dir != c.Dir { // the folder is gone
		dir = dirFor(c.Project)
	}
	s := a.openTabIn(c.Project, dir, shellByID(a.shells, c.Shell))
	if s == nil {
		return
	}
	s.conv = c.Ref
	cmd := history.Resume(c.Ref)
	if cmd == "" || s.conn == nil {
		return
	}
	// Typed once the shell has started, so that it takes the keys.
	go func() {
		select {
		case <-s.conn.ready:
		case <-time.After(10 * time.Second):
		}
		time.Sleep(600 * time.Millisecond)
		a.update(func() {
			if !s.closed {
				s.term.Send([]byte(cmd))
			}
		})
	}()
}

// convCache keeps what Nook read of conversations, read once on a
// goroutine of its own: their titles and what was said.
type convCache struct {
	titles  map[string]string
	reading map[string]bool
	texts   map[string]*transcript
	live    map[string]time.Time // when the title of a conversation going on was last read
}

// transcript is what was said in a conversation, as Nook shows it.
type transcript struct {
	msgs  []history.Message
	err   error
	done  bool
	shown int          // how many of the last messages show
	open  map[int]bool // the runs of tools unfolded, by where they start
}

// shownStep is how many more messages a click shows.
const shownStep = 200

// convTitle returns c's title, reading it the first time.
func (a *app) convTitle(c config.Conversation) string {
	cc := &a.convs
	key := c.Key()
	if t, ok := cc.titles[key]; ok {
		if t != "" {
			return t
		}
	} else if !cc.reading["t:"+key] {
		if cc.reading == nil {
			cc.reading, cc.titles = map[string]bool{}, map[string]string{}
		}
		cc.reading["t:"+key] = true
		go func() {
			t := history.Title(c.Ref)
			a.update(func() { cc.titles[key] = t })
		}()
	}
	return history.Names(c.Tool) + " · " + c.Seen.Local().Format("01-02 15:04")
}

// convText returns what was said in c, reading it the first time.
func (a *app) convText(c config.Conversation) *transcript {
	cc := &a.convs
	key := c.Key()
	if t := cc.texts[key]; t != nil {
		return t
	}
	if cc.texts == nil {
		cc.texts = map[string]*transcript{}
	}
	t := &transcript{shown: shownStep}
	cc.texts[key] = t
	go func() {
		msgs, err := history.Messages(c.Ref)
		a.update(func() { t.msgs, t.err, t.done = msgs, err, true })
	}()
	return t
}

// convView shows a closed conversation: what was said, read-only, and
// the button that continues it.
func (a *app) convView(c *ui.Context, conv config.Conversation) {
	t := c.Theme()
	text := a.convText(conv)
	ui.Column(c).Grow(1).FillWidth().Children(func() {
		ui.Row(c).FillWidth().PaddingX(20).PaddingY(12).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).Shrink(1).Gap(3).Children(func() {
				ui.Text(c, a.convTitle(conv)).FontSize(15).Bold().SingleLine()
				ui.Text(c, history.Names(conv.Tool)+" · "+conv.Dir+" · "+conv.Seen.Local().Format("2006-01-02 15:04")).FontSize(12).TextColor(t.TextMuted).SingleLine()
			})
			cont := ui.ButtonBase(c).Label(L("继续对话", "Continue")).Height(30).PaddingX(12).Radius(6).Background(t.Accent).
				Tooltip(L("在原来的文件夹打开终端，填好 ", "Opens a terminal in its folder with ") + history.Resume(conv.Ref) + L("，回车由你来按。按下回车 AI 才启动并联网。", " typed; you press Enter. The AI starts and goes online only then."))
			if cont.Hovered() {
				cont.Background(t.Accent.Alpha(0.85))
			}
			cont.Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, iconGlobe).FontSize(13).TextColor(t.AccentText)
					ui.Text(c, L("继续对话", "Continue")).FontSize(13).TextColor(t.AccentText).SingleLine()
				})
			})
			if cont.Clicked() {
				a.resume(conv)
			}
			if iconButton(c, iconTrash, L("从列表中移除（记录文件不删）", "Remove from the list (its record stays)")).Clicked() {
				a.removeConv(conv.Ref)
			}
			// The view's own buttons, apart from the conversation's.
			ui.Box(c).Width(1).Height(18).Margin(0, 4).Background(t.Border)
			a.viewerButtons(c)
		})
		ui.Divider(c)
		ui.Scroll(c).Grow(1).FillWidth().Children(func() {
			ui.Row(c).FillWidth().Justify(ui.Center).Children(func() {
				ui.Column(c).FillWidth().MaxWidth(820).PaddingX(24).PaddingY(16).Gap(12).Selectable().Children(func() {
					switch {
					case !text.done:
						ui.Spinner(c)
					case text.err != nil:
						ui.Text(c, L("找不到这个对话的记录：可能被 ", "The record of this conversation is not there: maybe ")+history.Names(conv.Tool)+L(" 删掉或移走了。", " deleted or moved it.")).FontSize(13).TextColor(t.TextMuted)
					case len(text.msgs) == 0:
						ui.Text(c, L("这个对话还没有内容。", "Nothing was said in this conversation yet.")).FontSize(13).TextColor(t.TextMuted)
					default:
						a.messages(c, text)
					}
				})
			})
		})
	})
}

// messages shows the last of what was said, a button for the earlier:
// what the user said on the right, the AI's replies as Markdown, and the
// tools it used in a row folded into one line.
func (a *app) messages(c *ui.Context, text *transcript) {
	t := c.Theme()
	from := max(0, len(text.msgs)-text.shown)
	if from > 0 {
		if ui.Button(c, L("显示更早的消息（还有 ", "Show earlier messages (")+itoa(from)+L(" 条）", " more)")).Clicked() {
			text.shown += shownStep
		}
	}
	msgs := text.msgs
	for i := from; i < len(msgs); i++ {
		m := msgs[i]
		c.Key(i)
		switch m.Role {
		case "user":
			ui.Row(c).FillWidth().Margin(10, 0, 0, 0).Children(func() {
				ui.Spacer(c)
				ui.Box(c).MaxWidth(560).Padding(8, 14).Radius(12).Background(t.Text.Alpha(0.08)).Children(func() {
					ui.Text(c, m.Text).FontSize(13).Wrap()
				})
			})
		case "tool":
			j := i
			for j+1 < len(msgs) && msgs[j+1].Role == "tool" {
				j++
			}
			a.toolRun(c, text, i, msgs[i:j+1])
			i = j
		case "note":
			ui.Box(c).FillWidth().Padding(8, 12).Radius(8).Border(1, t.Border).Children(func() {
				ui.Text(c, m.Text).FontSize(12).TextColor(t.TextMuted).Wrap()
			})
		default:
			markdown.View(c, strings.TrimSpace(m.Text), &a.mdOpts)
		}
	}
}

// toolRun shows tools the AI used one after another: one line, or, for
// several, a line saying how many, which unfolds to them all.
func (a *app) toolRun(c *ui.Context, text *transcript, at int, run []history.Message) {
	t := c.Theme()
	line := func(s string) {
		ui.Text(c, s).FontSize(12).TextColor(t.TextMuted).SingleLine()
	}
	if len(run) == 1 {
		// In line with a run's chevron: a dot where it would be.
		ui.Row(c).Gap(6).AlignItems(ui.Center).PaddingY(2).Children(func() {
			ui.Row(c).Width(12).Justify(ui.Center).Children(func() {
				ui.Box(c).Width(4).Height(4).Radius(2).Background(t.TextMuted)
			})
			line(run[0].Text)
		})
		return
	}
	open := text.open[at]
	head := ui.Row(c).Gap(6).AlignItems(ui.Center).PaddingY(2).Cursor(ui.CursorPointer)
	head.Children(func() {
		icon := iconChevronRight
		if open {
			icon = iconChevron
		}
		ui.Icon(c, icon).FontSize(12).TextColor(t.TextMuted)
		col := t.TextMuted
		if head.Hovered() {
			col = t.Text
		}
		ui.Text(c, L("用了 ", "Used ")+itoa(len(run))+L(" 次工具", " tools")).FontSize(12).TextColor(col)
	})
	if head.Clicked() {
		if text.open == nil {
			text.open = map[int]bool{}
		}
		text.open[at] = !open
	}
	if open {
		ui.Column(c).FillWidth().Gap(4).Padding(0, 0, 0, 18).Children(func() {
			for k, m := range run {
				c.Key(k)
				line(m.Text)
			}
		})
	}
}

// followLink opens a link of a conversation or a Markdown file: a page of
// the internet in the preview, through the proxy, or a file of the
// computer, as E:/Nook/a.txt or pelican.html:44, in the viewer.
func (a *app) followLink(dest string) {
	if u, err := url.Parse(dest); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		a.showWeb(u)
		return
	}
	p := strings.TrimPrefix(dest, "file:///")
	if s, err := url.PathUnescape(p); err == nil {
		p = s
	}
	p, _, _ = strings.Cut(p, "#")
	if i := strings.LastIndex(p, ":"); i > 1 {
		if _, err := strconv.Atoi(p[i+1:]); err == nil {
			p = p[:i]
		}
	}
	p = filepath.FromSlash(p)
	if fi, err := os.Stat(p); err == nil && !fi.IsDir() && filepath.IsAbs(p) {
		a.openFile(p)
	}
}

// liveTitle returns the title of r, a conversation going on in a
// terminal, "" until it has one. It is read again now and then, as the
// tool names the conversation once something is said: every few seconds
// until then, every minute after.
func (a *app) liveTitle(r history.Ref) string {
	cc := &a.convs
	key := r.Key()
	t := cc.titles[key]
	every := time.Minute
	if t == "" {
		every = 5 * time.Second
	}
	if cc.reading["l:"+key] || time.Since(cc.live[key]) < every {
		return t
	}
	if cc.reading == nil {
		cc.reading, cc.titles = map[string]bool{}, map[string]string{}
	}
	if cc.live == nil {
		cc.live = map[string]time.Time{}
	}
	cc.reading["l:"+key] = true
	go func() {
		title := history.Title(r)
		a.update(func() {
			cc.reading["l:"+key] = false
			cc.live[key] = time.Now()
			if title != "" {
				cc.titles[key] = title
			}
		})
	}()
	return t
}
