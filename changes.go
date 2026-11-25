package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"nook/core/checkpoint"
)

// The changes tab, as in Claude's desktop app: what the AI changed, to
// keep or to undo. As an AI first starts working in a project, Nook
// keeps the files as they are (in a repository of its own,
// core/checkpoint), the one copy undoing needs. The changes since add up,
// turn after turn, until the user keeps them or undoes them, file by
// file or all at once; then the copy goes.

// projChanges is what the changes tab shows of a project.
type projChanges struct {
	err     error
	base    checkpoint.Checkpoint // the files as the AI found them; none when nothing is pending
	changes []checkpoint.Change
	busy    bool
	read    time.Time // when last read
	done    string    // what was just done, as "已保留"
	doneAt  time.Time
	skip    error // why no copy was made as the AI started: none is tried again until asked
}

// saved counts the changes there is a copy to undo.
func (pc *projChanges) saved() int {
	n := 0
	for _, ch := range pc.changes {
		if !ch.Unsaved {
			n++
		}
	}
	return n
}

func (a *app) projChanges(proj string) *projChanges {
	if a.changes == nil {
		a.changes = map[string]*projChanges{}
	}
	pc := a.changes[proj]
	if pc == nil {
		pc = &projChanges{}
		a.changes[proj] = pc
	}
	return pc
}

// refreshChanges reads what changed since the files were kept, on a
// goroutine.
func (a *app) refreshChanges(proj string) {
	pc := a.projChanges(proj)
	if proj == "" || pc.busy {
		return
	}
	pc.busy = true
	go func() {
		var base checkpoint.Checkpoint
		var changes []checkpoint.Change
		// Only a look: no copy is made until an AI starts working.
		repo, err := checkpoint.Find(proj)
		var cps []checkpoint.Checkpoint
		if err == nil && repo != nil {
			cps, err = repo.List(1)
		}
		if err == nil && len(cps) > 0 {
			base = cps[0]
			changes, err = repo.Changes(base.ID)
		}
		a.update(func() {
			pc.busy, pc.read = false, time.Now()
			pc.err, pc.base, pc.changes = err, base, changes
		})
	}()
}

// ago says when, as people do: 刚刚, 5 分钟前, 昨天 14:32.
func ago(t time.Time) string {
	d := time.Since(t)
	now := time.Now()
	switch {
	case d < time.Minute:
		return L("刚刚", "just now")
	case d < time.Hour:
		return fmt.Sprintf(L("%d 分钟前", "%d min ago"), int(d.Minutes()))
	case t.YearDay() == now.YearDay() && t.Year() == now.Year():
		return L("今天 ", "today ") + t.Format("15:04")
	case t.YearDay() == now.AddDate(0, 0, -1).YearDay() && now.Sub(t) < 48*time.Hour:
		return L("昨天 ", "yesterday ") + t.Format("15:04")
	}
	return t.Format("01-02 15:04")
}

// aiStarts keeps the files of proj as they are, as an AI starts working
// there, unless they are kept already: the changes then add up until the
// user keeps or undoes them.
func (a *app) aiStarts(proj string) {
	if proj == "" || a.win == nil {
		return
	}
	if a.snapped == nil {
		a.snapped = map[string]time.Time{}
	}
	pc := a.projChanges(proj)
	if time.Since(a.snapped[proj]) < 3*time.Second || pc.skip != nil {
		return
	}
	a.snapped[proj] = time.Now()
	go func() {
		repo, err := checkpoint.Open(proj)
		if err == nil {
			var cps []checkpoint.Checkpoint
			if cps, err = repo.List(1); err != nil || len(cps) > 0 {
				return
			}
			var made bool
			if _, made, err = repo.Snapshot(L("AI 改之前", "Before the AI")); err != nil {
				repo.Clear() // no half a copy left on the drive
			} else if !made {
				return
			}
		}
		if err != nil {
			a.probe.log("snapshot %s: %v", proj, err)
		}
		a.update(func() {
			pc.skip = err
			a.refreshChanges(proj)
		})
	}()
}

// retryCopy tries again to keep the files of proj, as the user asks.
func (a *app) retryCopy(proj string) {
	a.projChanges(proj).skip = nil
	delete(a.snapped, proj)
	a.aiStarts(proj)
}

// onEnter keeps the files of the project of s when an AI runs there:
// Enter there most likely sends it a request.
func (a *app) onEnter(s *session) {
	if _, ai := a.net.ai[s.id]; ai {
		a.aiStarts(s.proj)
	}
}

// confirmAsk is a question waiting for the user's yes.
type confirmAsk struct {
	title, text, yes string
	danger           bool
	do               func()
}

// confirmDialog asks the question waiting, if any.
func (a *app) confirmDialog(c *ui.Context) {
	ask := a.confirm
	if ask == nil {
		return
	}
	t := c.Theme()
	open := true
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(400).Gap(12).Children(func() {
			ui.Text(c, ask.title).FontSize(15).Bold()
			ui.Text(c, ask.text).FontSize(13).TextColor(t.TextMuted).Wrap()
			ui.Row(c).FillWidth().Gap(8).Children(func() {
				ui.Spacer(c)
				if ui.Button(c, L("取消", "Cancel")).Clicked() {
					a.confirm = nil
				}
				yes := ui.PrimaryButton(c, ask.yes)
				if ask.danger {
					yes.Background(t.Danger)
				}
				if yes.Clicked() {
					a.confirm = nil
					ask.do()
				}
			})
		})
	})
	if !open {
		a.confirm = nil
	}
}

// changesDone is after the changes of proj were kept or undone: the tab
// says so, and what shows follows.
func (a *app) changesDone(proj, done string, err error) {
	pc := a.projChanges(proj)
	if err != nil {
		a.notice = err.Error()
	} else {
		pc.done, pc.doneAt = done, time.Now()
	}
	a.refreshChanges(proj)
	a.closeItems(func(it viewItem) bool { return it.diff != nil && it.diff.proj == proj })
	if a.file != nil {
		a.loadFile(a.file)
	}
}

// keepAll keeps every change: the copy of the files as they were goes.
func (a *app) keepAll(proj string) {
	done := L("已保留 ✓", "Kept ✓")
	if a.projChanges(proj).saved() == 0 {
		done = L("知道了 ✓", "Got it ✓")
	}
	go func() {
		repo, err := checkpoint.Open(proj)
		if err == nil {
			err = repo.Clear()
		}
		a.update(func() { a.changesDone(proj, done, err) })
	}()
}

// askUndoAll asks, then puts every changed file back as it was, and the
// copy goes.
func (a *app) askUndoAll(proj string) {
	pc := a.projChanges(proj)
	base, n, unsaved := pc.base, pc.saved(), len(pc.changes)-pc.saved()
	title := fmt.Sprintf(L("撤销这 %d 个文件的改动？", "Undo the changes to these %d files?"), n)
	if n == 1 {
		title = L("撤销这个文件的改动？", "Undo the changes to this file?")
	}
	text := L("变回 AI 改之前，新建的会删掉，不能恢复。", "Back as before the AI; files it added go. This cannot be undone.")
	if unsaved > 0 {
		text += L("没备份的大文件不动。", " Large files not kept stay as they are.")
	}
	for _, s := range a.sessions() {
		if s.proj == proj && s.status == "working" {
			text += L("AI 还在干活，先让它停下。", " The AI is still working: stop it first.")
			break
		}
	}
	a.confirm = &confirmAsk{
		title:  title,
		text:   text,
		yes:    L("撤销", "Undo"),
		danger: true,
		do: func() {
			go func() {
				repo, err := checkpoint.Open(proj)
				if err == nil {
					_, err = repo.Restore(base.ID, "")
				}
				if err == nil {
					err = repo.Clear()
				}
				a.update(func() { a.changesDone(proj, L("已撤销 ✓", "Undone ✓"), err) })
			}()
		},
	}
}

// keepFile keeps the changes to one file.
func (a *app) keepFile(proj string, ch checkpoint.Change) {
	last := len(a.projChanges(proj).changes) == 1
	done := L("已保留 ✓", "Kept ✓")
	if ch.Unsaved {
		done = L("知道了 ✓", "Got it ✓")
	}
	go func() {
		repo, err := checkpoint.Open(proj)
		if err == nil {
			err = repo.KeepFile(ch.Path)
		}
		if err == nil && last {
			err = repo.Clear() // nothing left pending
		}
		a.update(func() { a.changesDone(proj, done, err) })
	}()
}

// askFileBack asks, then puts one file back as it was.
func (a *app) askFileBack(proj string, base checkpoint.Checkpoint, ch checkpoint.Change) {
	name := filepath.Base(ch.Path)
	text := L("变回 AI 改之前，不能恢复。", "Back as before the AI. This cannot be undone.")
	if ch.Status == 'A' {
		text = L("AI 新建的文件，撤销就是删掉，不能恢复。", "The AI added it: undoing deletes it, for good.")
	}
	last := len(a.projChanges(proj).changes) == 1
	a.confirm = &confirmAsk{
		title:  fmt.Sprintf(L("撤销 %s 的改动？", "Undo the changes to %s?"), name),
		text:   text,
		yes:    L("撤销", "Undo"),
		danger: true,
		do: func() {
			go func() {
				repo, err := checkpoint.Open(proj)
				if err == nil {
					_, err = repo.RestoreFile(base.ID, ch.Path, "")
				}
				if err == nil && last {
					err = repo.Clear()
				}
				a.update(func() { a.changesDone(proj, L("已撤销 ✓", "Undone ✓"), err) })
			}()
		},
	}
}

// smallButton is a button of the panels' size.
func smallButton(c *ui.Context, label string, primary bool) ui.Element {
	t := c.Theme()
	b := ui.ButtonBase(c).Label(label).Padding(5, 12).Radius(6)
	fg := t.Text
	switch {
	case primary:
		bg := t.Accent
		if b.Hovered() {
			bg = t.Accent.Mix(t.Text, 0.12)
		}
		b.Background(bg)
		fg = t.AccentText
	case b.Hovered():
		b.Background(t.SurfaceHover).Border(1, t.Border)
	default:
		b.Border(1, t.Border)
	}
	b.Children(func() { ui.Text(c, label).FontSize(12).TextColor(fg) })
	return b
}

// changesTab is the right panel's tab of changes, for project proj.
func (a *app) changesTab(c *ui.Context, proj string) {
	t := c.Theme()
	if proj == "" {
		ui.Text(c, L("先在左边选一个项目。", "Pick a project on the left.")).FontSize(12).TextColor(t.TextMuted).Padding(14)
		return
	}
	pc := a.projChanges(proj)
	if pc.read.IsZero() && !pc.busy {
		a.refreshChanges(proj)
	}
	ui.Scroll(c).Grow(1).FillWidth().Children(func() {
		ui.Column(c).FillWidth().Padding(10).Children(func() {
			card := ui.Column(c).FillWidth().Padding(12).Gap(8).Radius(10).Border(1, t.Border).Background(t.Background.Alpha(0.6))
			card.Children(func() {
				switch {
				case pc.err != nil:
					ui.Text(c, pc.err.Error()).FontSize(12).TextColor(t.Danger).Wrap()
				case pc.skip != nil && len(pc.changes) == 0:
					ui.Text(c, L("没有记录改动", "Changes not tracked")).FontSize(13).Bold()
					ui.Text(c, pc.skip.Error()).FontSize(12).TextColor(t.TextMuted).Wrap()
					ui.Row(c).Children(func() {
						if smallButton(c, L("再试一次", "Try again"), false).Tooltip(L("AI 改文件之前，重新备份一次", "Keep the files again, before the AI changes them")).Clicked() {
							a.retryCopy(proj)
						}
					})
				case len(pc.changes) == 0:
					title := L("没有改动", "No changes")
					if pc.done != "" && time.Since(pc.doneAt) < 30*time.Second {
						title = pc.done
					}
					ui.Text(c, title).FontSize(13).Bold()
					ui.Text(c, L("AI 改的文件会列在这里。", "Files the AI changes show here.")).FontSize(12).TextColor(t.TextMuted).Wrap()
				default:
					ui.Column(c).Gap(2).Children(func() {
						ui.Text(c, L("AI 改了 ", "The AI changed ")+count(len(pc.changes), "个文件", "file", "files")).FontSize(13).Bold()
						if pc.saved() > 0 {
							ui.Text(c, L("点文件看改了哪里", "Click a file to see what changed")).FontSize(11).TextColor(t.TextMuted)
						}
					})
					ui.Column(c).FillWidth().Gap(1).Children(func() {
						for _, ch := range pc.changes {
							a.changeRow(c, proj, pc.base, ch)
						}
					})
					saved := pc.saved()
					if saved < len(pc.changes) {
						ui.Text(c, L("灰色：大文件没备份，撤销不了。", "Grey: large files not kept; they cannot be undone.")).FontSize(11).TextColor(t.TextMuted).Wrap()
					}
					ui.Row(c).FillWidth().Gap(8).PaddingY(2).Children(func() {
						keep, keepTip := L("全部保留", "Keep all"), L("改动都留下", "Keep every change")
						if saved == 0 {
							keep, keepTip = L("都知道了", "Got it"), L("这些大文件不再提示，再被改时才会出现", "These go until they change again")
						}
						if smallButton(c, keep, true).Tooltip(keepTip).Grow(1).Clicked() {
							a.keepAll(proj)
						}
						if saved > 0 && smallButton(c, L("全部撤销", "Undo all"), false).Tooltip(L("文件全部变回 AI 改之前", "Every file goes back as it was before the AI")).Grow(1).Clicked() {
							a.askUndoAll(proj)
						}
					})
				}
			})
		})
	})
}

// changeRow is a changed file: its name and folder, a word when it is
// new or gone, and, under the pointer, keep and undo. A click compares
// it.
func (a *app) changeRow(c *ui.Context, proj string, base checkpoint.Checkpoint, ch checkpoint.Change) {
	if ch.Unsaved {
		a.unsavedRow(c, proj, ch)
		return
	}
	t := c.Theme()
	c.Key("ch:" + ch.Path)
	row := ui.Row(c).FillWidth().Height(28).PaddingX(6).Gap(6).AlignItems(ui.Center).Radius(6).Cursor(ui.CursorPointer)
	hover := row.Hovered()
	if a.diff != nil && a.viewing == "diff:"+ch.Path {
		row.Background(t.Selection)
	} else if hover {
		row.Background(t.SurfaceHover)
	}
	row.Tooltip(ch.Path)
	var keep, undo bool
	row.ContextMenu(func(m *ui.Menu) {
		if m.Item(L("看改了哪里", "See what changed")).Chosen() {
			a.openDiff(proj, base, ch)
		}
		if ch.Status != 'D' && m.Item(L("打开文件", "Open the file")).Chosen() {
			a.openFile(filepath.Join(proj, filepath.FromSlash(ch.Path)))
		}
		if ch.Status != 'D' && m.Item(L("在资源管理器中显示", "Show in Explorer")).Chosen() {
			mygo.Shell.ShowItemInFolder(filepath.Join(proj, filepath.FromSlash(ch.Path)))
		}
		m.Separator()
		if m.Item(L("保留这个文件", "Keep this file")).Chosen() {
			keep = true
		}
		if m.Item(L("撤销这个文件", "Undo this file")).Chosen() {
			undo = true
		}
	})
	row.Children(func() {
		icon, col := kindIcon(t, fileKind(ch.Path))
		ui.Icon(c, icon).FontSize(13).TextColor(col).Shrink(0)
		name := ui.Text(c, filepath.Base(ch.Path)).FontSize(12).SingleLine().Shrink(0)
		if ch.Status == 'D' {
			name.TextColor(t.TextMuted)
		}
		if dir := filepath.Dir(filepath.FromSlash(ch.Path)); dir != "." {
			ui.Text(c, dir).FontSize(11).TextColor(t.TextMuted).SingleLine().Shrink(1)
		}
		ui.Spacer(c)
		if hover {
			if iconButton(c, iconCheck, L("保留这个文件", "Keep this file")).Clicked() {
				keep = true
			}
			if iconButton(c, iconUndo, L("撤销这个文件", "Undo this file")).Clicked() {
				undo = true
			}
		} else if ch.Status != 'M' {
			ui.Text(c, statusWord(ch.Status)).FontSize(11).TextColor(t.TextMuted).Shrink(0)
		}
	})
	switch {
	case keep:
		a.keepFile(proj, ch)
	case undo:
		a.askFileBack(proj, base, ch)
	case row.Clicked():
		a.openDiff(proj, base, ch)
	}
}

// unsavedRow is a large file left out of the copy that the AI changed,
// added or deleted: in grey, to be known, with nothing to compare or
// undo.
func (a *app) unsavedRow(c *ui.Context, proj string, ch checkpoint.Change) {
	t := c.Theme()
	c.Key("ch:" + ch.Path)
	row := ui.Row(c).FillWidth().Height(28).PaddingX(6).Gap(6).AlignItems(ui.Center).Radius(6)
	hover := row.Hovered()
	if hover {
		row.Background(t.SurfaceHover)
	}
	path := filepath.Join(proj, filepath.FromSlash(ch.Path))
	row.Tooltip(ch.Path + "\n" + L("大文件没备份，撤销不了。", "A large file not kept: it cannot be undone."))
	known := false
	row.ContextMenu(func(m *ui.Menu) {
		if ch.Status != 'D' && m.Item(L("打开文件", "Open the file")).Chosen() {
			a.openFile(path)
		}
		if ch.Status != 'D' && m.Item(L("在资源管理器中显示", "Show in Explorer")).Chosen() {
			mygo.Shell.ShowItemInFolder(path)
		}
		m.Separator()
		if m.Item(L("知道了", "Got it")).Chosen() {
			known = true
		}
	})
	row.Children(func() {
		icon, _ := kindIcon(t, fileKind(ch.Path))
		ui.Icon(c, icon).FontSize(13).TextColor(t.TextMuted).Shrink(0)
		ui.Text(c, filepath.Base(ch.Path)).FontSize(12).TextColor(t.TextMuted).SingleLine().Shrink(0)
		if dir := filepath.Dir(filepath.FromSlash(ch.Path)); dir != "." {
			ui.Text(c, dir).FontSize(11).TextColor(t.TextMuted).SingleLine().Shrink(1)
		}
		ui.Spacer(c)
		if hover {
			if iconButton(c, iconCheck, L("知道了：不再提示，再被改时才会出现", "Got it: gone until it changes again")).Clicked() {
				known = true
			}
			return
		}
		word := L("没有备份", "not kept")
		if ch.Status != 'M' {
			word = statusWord(ch.Status) + L(" · 没有备份", " · not kept")
		}
		ui.Text(c, word).FontSize(11).TextColor(t.TextMuted).Shrink(0)
	})
	if known {
		a.keepFile(proj, ch)
	}
}

func statusWord(s byte) string {
	switch s {
	case 'A':
		return L("新建", "new")
	case 'D':
		return L("已删除", "deleted")
	}
	return L("修改", "changed")
}

// diffView is a file compared side by side in the middle: as a snapshot
// kept it, and as it is now.
type diffView struct {
	proj  string
	cp    checkpoint.Checkpoint
	ch    checkpoint.Change
	state *editor.DiffState
	note  string // why there is no comparison
}

// openDiff shows a changed file side by side.
func (a *app) openDiff(proj string, cp checkpoint.Checkpoint, ch checkpoint.Change) {
	d := &diffView{proj: proj, cp: cp, ch: ch}
	a.diff = d
	a.viewing = "diff:" + ch.Path
	go func() {
		var old, cur string
		repo, err := checkpoint.Open(proj)
		if err == nil {
			old, err = repo.Content(cp.ID, ch.Path)
		}
		if b, rerr := readSmall(filepath.Join(proj, filepath.FromSlash(ch.Path))); rerr == nil {
			cur = string(b)
		}
		a.update(func() {
			switch {
			case err != nil:
				d.note = err.Error()
			case bytes.IndexByte([]byte(old), 0) >= 0 || bytes.IndexByte([]byte(cur), 0) >= 0:
				d.note = L("这是二进制文件，不显示逐行对比。", "This is a binary file: no line-by-line comparison.")
			default:
				lang := editor.LanguageFor(ch.Path)
				ob, nb := editor.NewBuffer(old), editor.NewBuffer(cur)
				ob.SetLanguage(lang)
				nb.SetLanguage(lang)
				d.state = &editor.DiffState{Old: ob, New: nb, Layout: editor.DiffSplit}
			}
		})
	}()
}

// maxView is the largest file Nook shows.
const maxView = 5 << 20

// readSmall reads a file, unless it is too large to show.
func readSmall(path string) ([]byte, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if fi.Size() > maxView {
		return nil, errTooLarge
	}
	return os.ReadFile(path)
}

// diffPage is the middle when a comparison shows.
func (a *app) diffPage(c *ui.Context) {
	t := c.Theme()
	d := a.diff
	if d == nil {
		return // closed as the frame built
	}
	ui.Column(c).Grow(1).FillWidth().Children(func() {
		ui.Row(c).FillWidth().PaddingX(16).PaddingY(10).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				ui.Text(c, d.ch.Path).FontSize(14).Bold().SingleLine()
				ui.Text(c, L("左边是 AI 改之前，右边是现在", "Left: before the AI; right: now")).FontSize(12).TextColor(t.TextMuted).SingleLine()
			})
			if smallButton(c, L("保留", "Keep"), true).Clicked() {
				a.keepFile(d.proj, d.ch)
			}
			if smallButton(c, L("撤销", "Undo"), false).Tooltip(L("这个文件变回左边的样子", "This file goes back as on the left")).Clicked() {
				a.askFileBack(d.proj, d.cp, d.ch)
			}
			a.viewerButtons(c)
		})
		ui.Divider(c)
		switch {
		case d.note != "":
			ui.Text(c, d.note).FontSize(13).TextColor(t.TextMuted).Padding(16)
		case d.state == nil:
			ui.Spinner(c).Margin(16)
		default:
			editor.DiffView(c, d.state).Grow(1).FillWidth()
		}
	})
}
