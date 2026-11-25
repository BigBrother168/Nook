package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/video"

	"nook/core/lang"
)

// The files tab: the project's files as they are, read again while the
// tab shows, those changed since the last snapshot marked, found by
// typing; and a file shown beside the terminals: a picture, a video, a
// page, Markdown or text, else what it is and the app to open it with.

var errTooLarge = lang.Error{Zh: "文件太大，不显示", En: "The file is too large to show"}

// fileNode is a file or folder of the files tab.
type fileNode struct {
	name, path string // path is absolute
	dir, open  bool
	read       bool
	kids       []*fileNode
}

func (a *app) fileRoot(proj string) *fileNode {
	if a.fileRoots == nil {
		a.fileRoots = map[string]*fileNode{}
	}
	n := a.fileRoots[proj]
	if n == nil {
		n = &fileNode{path: proj, dir: true, open: true}
		a.fileRoots[proj] = n
	}
	return n
}

// readDir reads a folder's entries, keeping those open open.
func (n *fileNode) readDir() {
	n.read = true
	entries, err := os.ReadDir(n.path)
	if err != nil {
		n.kids = nil
		return
	}
	old := map[string]*fileNode{}
	for _, k := range n.kids {
		old[k.name] = k
	}
	kids := make([]*fileNode, 0, len(entries))
	for _, e := range entries {
		if e.Name() == ".git" {
			continue
		}
		k := old[e.Name()]
		if k == nil || k.dir != e.IsDir() {
			k = &fileNode{name: e.Name(), path: filepath.Join(n.path, e.Name()), dir: e.IsDir()}
		}
		kids = append(kids, k)
	}
	slices.SortFunc(kids, func(x, y *fileNode) int {
		if x.dir != y.dir {
			if x.dir {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(x.name), strings.ToLower(y.name))
	})
	n.kids = kids
}

// refresh reads the open folders again.
func (n *fileNode) refresh() {
	if !n.dir || !n.open || !n.read {
		return
	}
	n.readDir()
	for _, k := range n.kids {
		k.refresh()
	}
}

// The kinds of file, by what Nook does with them.
const (
	kindImage  = "image"
	kindGIF    = "gif"
	kindSVG    = "svg"
	kindVideo  = "video"
	kindAudio  = "audio"
	kindPage   = "page"
	kindMD     = "md"
	kindPDF    = "pdf"
	kindOffice = "office"
	kindZip    = "zip"
	kindApp    = "app"
	kindCode   = "code"
	kindData   = "data"
	kindText   = "text"
)

var kindsByExt = map[string]string{
	".png": kindImage, ".jpg": kindImage, ".jpeg": kindImage, ".webp": kindImage, ".bmp": kindImage, ".ico": kindImage,
	".gif": kindGIF, ".svg": kindSVG,
	".mp4": kindVideo, ".mov": kindVideo, ".m4v": kindVideo, ".webm": kindVideo, ".mkv": kindVideo, ".avi": kindVideo,
	".mp3": kindAudio, ".wav": kindAudio, ".flac": kindAudio, ".m4a": kindAudio, ".ogg": kindAudio, ".aac": kindAudio,
	".html": kindPage, ".htm": kindPage,
	".md": kindMD, ".markdown": kindMD,
	".pdf":  kindPDF,
	".docx": kindOffice, ".doc": kindOffice, ".xlsx": kindOffice, ".xls": kindOffice, ".pptx": kindOffice, ".ppt": kindOffice, ".csv": kindData,
	".zip": kindZip, ".7z": kindZip, ".rar": kindZip, ".tar": kindZip, ".gz": kindZip, ".tgz": kindZip,
	".exe": kindApp, ".msi": kindApp, ".dll": kindApp, ".bat": kindCode, ".cmd": kindCode, ".ps1": kindCode,
	".json": kindData, ".yaml": kindData, ".yml": kindData, ".toml": kindData, ".xml": kindData, ".ini": kindData, ".env": kindData, ".lock": kindData,
	".txt": kindText, ".log": kindText,
	".go": kindCode, ".js": kindCode, ".mjs": kindCode, ".cjs": kindCode, ".ts": kindCode, ".tsx": kindCode, ".jsx": kindCode, ".py": kindCode,
	".rs": kindCode, ".c": kindCode, ".h": kindCode, ".cpp": kindCode, ".cs": kindCode, ".java": kindCode, ".kt": kindCode, ".swift": kindCode,
	".rb": kindCode, ".php": kindCode, ".lua": kindCode, ".sh": kindCode, ".css": kindCode, ".scss": kindCode, ".vue": kindCode, ".svelte": kindCode, ".sql": kindCode,
}

func fileKind(path string) string {
	return kindsByExt[strings.ToLower(filepath.Ext(path))]
}

// kindIcon is the icon of a file of a kind, and its color.
func kindIcon(t *ui.Theme, kind string) (*ui.SVG, ui.Color) {
	switch kind {
	case kindImage, kindGIF, kindSVG:
		return iconFileImage, ui.Hex("#2f9e6e")
	case kindVideo:
		return iconFileVideo, ui.Hex("#c2410c")
	case kindAudio:
		return iconFileAudio, ui.Hex("#be185d")
	case kindPage:
		return iconGlobe, ui.Hex("#e8590c")
	case kindMD, kindText:
		return iconFileText, ui.Hex("#4c6ef5")
	case kindPDF:
		return iconFileText, ui.Hex("#e03131")
	case kindOffice, kindData:
		return iconFileSheet, ui.Hex("#2b8a3e")
	case kindZip:
		return iconFileArchive, ui.Hex("#a16207")
	case kindApp:
		return iconFileApp, t.TextMuted
	case kindCode:
		return iconFileCode, ui.Hex("#7048e8")
	}
	return iconFile, t.TextMuted
}

// rel is path relative to project proj, or path itself when outside.
func rel(proj, path string) string {
	if r, err := filepath.Rel(proj, path); err == nil && !strings.HasPrefix(r, "..") {
		return r
	}
	return path
}

// fileMenu is a file's right-click menu.
func (a *app) fileMenu(m *ui.Menu, proj, path string, dir bool) {
	if !dir {
		if m.Item(L("打开", "Open")).Chosen() {
			a.openFile(path)
		}
	}
	if m.Item(L("用默认程序打开", "Open with its app")).Chosen() {
		mygo.Shell.OpenPath(path)
	}
	if fileKind(path) == kindPage && m.Item(L("在预览里打开", "Open in the preview")).Chosen() {
		a.openPage(path)
	}
	if m.Item(L("在资源管理器中显示", "Show in Explorer")).Chosen() {
		mygo.Shell.ShowItemInFolder(path)
	}
	m.Separator()
	if s := a.focused(); s != nil && s.term != nil {
		if m.Item(L("把路径打进终端", "Type its path in the terminal")).Tooltip(L("让 AI 看这个文件时很方便", "Handy to point the AI at it")).Chosen() {
			s.term.Paste(quotePaths(s.shell.Quote, []string{path}))
			a.pendingFocus = s
		}
	}
	if m.Item(L("复制路径", "Copy the path")).Chosen() {
		mygo.Clipboard.WriteText(path)
	}
	if m.Item(L("复制相对路径", "Copy the relative path")).Chosen() {
		mygo.Clipboard.WriteText(filepath.ToSlash(rel(proj, path)))
	}
}

// fileIndex is every file of a project, for finding by name.
type fileIndex struct {
	paths []string // relative, slash-separated
	more  bool     // there were more than were kept
	at    time.Time
	busy  bool
}

// skipDirs are folders finding leaves out: what tools make, and large.
var skipDirs = map[string]bool{".git": true, "node_modules": true, ".venv": true, "venv": true, "__pycache__": true, ".next": true, ".cache": true, "target": true, "dist": true, "build": true, ".idea": true, ".vs": true}

const maxIndex = 20000

// indexFiles lists proj's files on a goroutine, at most every few seconds.
func (a *app) indexFiles(proj string) *fileIndex {
	if a.fileIndexes == nil {
		a.fileIndexes = map[string]*fileIndex{}
	}
	ix := a.fileIndexes[proj]
	if ix == nil {
		ix = &fileIndex{}
		a.fileIndexes[proj] = ix
	}
	if ix.busy || time.Since(ix.at) < 5*time.Second {
		return ix
	}
	ix.busy = true
	go func() {
		var paths []string
		more := false
		filepath.WalkDir(proj, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if p != proj && skipDirs[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			if len(paths) >= maxIndex {
				more = true
				return filepath.SkipAll
			}
			paths = append(paths, filepath.ToSlash(rel(proj, p)))
			return nil
		})
		a.update(func() { ix.paths, ix.more, ix.at, ix.busy = paths, more, time.Now(), false })
	}()
	return ix
}

// filesTab is the right panel's tab of files, for project proj.
func (a *app) filesTab(c *ui.Context, proj string) {
	t := c.Theme()
	if proj == "" {
		ui.Text(c, L("先在左边选一个项目。", "Pick a project on the left.")).FontSize(12).TextColor(t.TextMuted).Padding(12)
		return
	}
	root := a.fileRoot(proj)
	if !root.read {
		root.readDir()
	}
	// Changed since the last snapshot: the files, and the folders holding them.
	changed := map[string]bool{}
	if pc := a.changes[proj]; pc != nil {
		for _, ch := range pc.changes {
			p := filepath.Join(proj, filepath.FromSlash(ch.Path))
			for ; len(p) > len(proj); p = filepath.Dir(p) {
				changed[strings.ToLower(p)] = true
			}
		}
	}
	ui.Row(c).FillWidth().Padding(8, 8, 6, 8).Gap(4).AlignItems(ui.Center).Children(func() {
		ui.TextInput(c, &a.fileQuery).Placeholder(L("找文件…", "Find a file…")).Grow(1).Shrink(1)
		if iconButton(c, iconFolder, L("在资源管理器中打开项目文件夹", "Open the project's folder in Explorer")).Clicked() {
			mygo.Shell.OpenPath(proj)
		}
	})
	// A row of a file or folder: its icon, its name, marked when changed.
	row := func(path, name, sub string, dir bool, open *bool, depth int, item ui.TreeItemParts) {
		var r ui.Element
		if open != nil || depth >= 0 {
			r = item.Row
		} else {
			r = ui.Row(c)
		}
		r.FillWidth().Height(26).PaddingStart(6 + 14*float32(max(depth, 0))).PaddingEnd(6).Gap(6).AlignItems(ui.Center).Radius(5).Cursor(ui.CursorPointer)
		if a.file != nil && a.viewing == "file:"+path {
			r.Background(t.Selection)
		} else if r.Hovered() {
			r.Background(t.SurfaceHover)
		}
		r.Tooltip(path)
		r.ContextMenu(func(m *ui.Menu) { a.fileMenu(m, proj, path, dir) })
		r.Children(func() {
			icon, col := kindIcon(t, fileKind(path))
			if dir {
				icon, col = iconFolder, t.TextMuted
				if open != nil && *open {
					icon = iconFolderOpen
				}
			}
			ui.Icon(c, icon).FontSize(14).TextColor(col).Shrink(0)
			txt := ui.Text(c, name).FontSize(12).SingleLine().Shrink(0)
			switch {
			case changed[strings.ToLower(path)]:
				txt.TextColor(t.Warning)
			case strings.HasPrefix(name, "."):
				txt.TextColor(t.TextMuted)
			}
			if sub != "" {
				ui.Text(c, sub).FontSize(11).TextColor(t.TextMuted).SingleLine().Shrink(1)
			}
			if changed[strings.ToLower(path)] && !dir {
				ui.Spacer(c)
				ui.Box(c).Width(6).Height(6).Radius(3).Background(t.Warning).Shrink(0)
			}
		})
		if r.DoubleClicked() && !dir {
			a.openDouble(path)
		} else if r.Clicked() {
			if dir {
				if open != nil {
					*open = !*open
				}
			} else {
				a.openFile(path)
			}
		}
	}
	if q := strings.ToLower(strings.TrimSpace(a.fileQuery)); q != "" {
		ix := a.indexFiles(proj)
		ui.Scroll(c).Grow(1).FillWidth().Children(func() {
			ui.Column(c).FillWidth().PaddingX(4).PaddingY(2).Children(func() {
				shown := 0
				for _, p := range ix.paths {
					if !strings.Contains(strings.ToLower(p), q) {
						continue
					}
					if shown++; shown > 300 {
						ui.Text(c, L("还有更多，再多打几个字…", "More: type more…")).FontSize(11).TextColor(t.TextMuted).Padding(6)
						break
					}
					c.Key("q:" + p)
					dir := filepath.Dir(filepath.FromSlash(p))
					if dir == "." {
						dir = ""
					}
					row(filepath.Join(proj, filepath.FromSlash(p)), filepath.Base(p), dir, false, nil, -1, ui.TreeItemParts{})
				}
				if shown == 0 {
					msg := L("没找到。", "Nothing found.")
					if ix.busy && len(ix.paths) == 0 {
						msg = L("正在列出文件…", "Listing the files…")
					}
					ui.Text(c, msg).FontSize(12).TextColor(t.TextMuted).Padding(8)
				}
			})
		})
		return
	}
	ui.Scroll(c).Grow(1).FillWidth().Children(func() {
		tree := ui.TreeBase(c)
		tree.Root.FillWidth().PaddingX(4).PaddingY(2).Children(func() {
			var node func(n *fileNode)
			node = func(n *fileNode) {
				c.Key(n.path)
				var open *bool
				if n.dir {
					open = &n.open
				}
				item := tree.Item(open)
				row(n.path, n.name, "", n.dir, open, item.Depth(), item)
				if n.dir {
					item.Group(func() {
						if !n.read {
							n.readDir()
						}
						for _, k := range n.kids {
							node(k)
						}
					})
				}
			}
			for _, k := range root.kids {
				node(k)
			}
		})
	})
}

// fileView is a file shown in the middle.
type fileView struct {
	path   string
	kind   string
	size   int64
	mod    time.Time
	bmp    *ui.Bitmap
	svg    *ui.SVG
	player *ui.VideoPlayer // a video, or a GIF moving
	md     string
	code   *editor.State
	note   string
	actual bool // a picture at its own size, not fitted
	done   bool
}

// openFile shows a file in the middle, read on a goroutine.
func (a *app) openFile(path string) {
	// Open already: it shows again, read afresh.
	for _, it := range a.items {
		if it.file != nil && it.file.path == path {
			a.closeItem(it.key)
			break
		}
	}
	f := &fileView{path: path, kind: fileKind(path)}
	a.file = f
	a.viewing = "file:" + path
	a.loadFile(f)
}

// closeFile stops what the file shown plays.
func (a *app) closeFile() {
	if a.file != nil && a.file.player != nil {
		a.file.player.Close()
		a.file.player = nil
	}
}

// openPage shows an HTML file of the computer in the preview.
func (a *app) openPage(path string) {
	a.showWeb(&url.URL{Scheme: "file", Path: "/" + filepath.ToSlash(path)})
}

func (a *app) loadFile(f *fileView) {
	if f.kind == kindVideo || f.kind == kindAudio {
		fi, err := os.Stat(f.path)
		f.done = true
		if err != nil {
			f.note = err.Error()
			return
		}
		f.size, f.mod = fi.Size(), fi.ModTime()
		if f.kind == kindAudio {
			return // shown as what it is, with its app
		}
		if f.player == nil {
			p, err := video.Open(f.path)
			if err != nil {
				f.note = L("Nook 播放不了这个视频，用默认程序打开试试。", "Nook cannot play this video: try its app.")
				return
			}
			f.player = p
		}
		return
	}
	go func() {
		fi, err := os.Stat(f.path)
		var data []byte
		if err == nil {
			switch f.kind {
			case kindPDF, kindOffice, kindZip, kindApp:
				// Not read: shown as what it is, with its app.
			default:
				data, err = readSmall(f.path)
			}
		}
		a.update(func() {
			f.done = true
			f.bmp, f.svg, f.md, f.code, f.note = nil, nil, "", nil, ""
			if fi != nil {
				f.size, f.mod = fi.Size(), fi.ModTime()
			}
			switch {
			case errors.Is(err, os.ErrNotExist):
				f.note = L("这个文件已经不在了。", "This file is gone.")
			case errors.Is(err, errTooLarge):
				f.note = L("文件太大，Nook 不打开，用默认程序打开吧。", "Too large for Nook: open it with its app.")
			case err != nil:
				f.note = err.Error()
			case f.kind == kindImage:
				if bmp, err := ui.DecodeBitmap(data); err == nil {
					f.bmp = bmp
				} else {
					f.note = L("这张图片打不开：", "This picture does not open: ") + err.Error()
				}
			case f.kind == kindGIF:
				if d, err := ui.NewGIFDecoder(data); err == nil {
					if p, err := ui.NewVideoPlayer(d); err == nil {
						if f.player != nil {
							f.player.Close()
						}
						f.player = p.SetLoop(true).SetMuted(true)
						break
					}
				}
				if bmp, err := ui.DecodeBitmap(data); err == nil {
					f.bmp = bmp
				}
			case f.kind == kindSVG:
				if svg, err := ui.ParseSVG(data); err == nil {
					f.svg = svg
				} else {
					f.code = codeState(f.path, data)
				}
			case data == nil:
				// Shown as what it is.
			case bytes.IndexByte(data[:min(len(data), 8000)], 0) >= 0:
				f.kind = kindApp
			case f.kind == kindMD:
				f.md = string(data)
			default:
				f.code = codeState(f.path, data)
			}
		})
	}()
}

func codeState(path string, data []byte) *editor.State {
	buf := editor.NewBuffer(string(data))
	buf.SetLanguage(editor.LanguageFor(path))
	return &editor.State{Buffer: buf, ReadOnly: true}
}

// sizeText says a file's size, as 12 KB.
func sizeText(n int64) string {
	switch {
	case n < 1<<10:
		return fmt.Sprintf("%d B", n)
	case n < 1<<20:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	case n < 1<<30:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	}
	return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
}

// filePage is the middle when a file shows.
func (a *app) filePage(c *ui.Context) {
	t := c.Theme()
	f := a.file
	if f == nil {
		return // closed as the frame built
	}
	ui.Column(c).Grow(1).FillWidth().Children(func() {
		ui.Row(c).FillWidth().PaddingX(14).PaddingY(8).Gap(6).AlignItems(ui.Center).Children(func() {
			icon, col := kindIcon(t, f.kind)
			ui.Icon(c, icon).FontSize(16).TextColor(col)
			ui.Column(c).Grow(1).Shrink(1).Gap(1).Children(func() {
				ui.Text(c, filepath.Base(f.path)).FontSize(13).Bold().SingleLine()
				info := filepath.Dir(rel(a.currentProject(), f.path))
				if info == "." {
					info = ""
				}
				if f.done && f.size > 0 {
					if info != "" {
						info += " · "
					}
					info += sizeText(f.size)
				}
				if f.bmp != nil {
					w, h := f.bmp.Size()
					info += " · " + itoa(w) + " × " + itoa(h)
				}
				if info != "" {
					ui.Text(c, info).FontSize(11).TextColor(t.TextMuted).SingleLine()
				}
			})
			if f.bmp != nil {
				label := L("原始大小", "Actual size")
				if f.actual {
					label = L("适应窗口", "Fit")
				}
				if smallButton(c, label, false).Clicked() {
					f.actual = !f.actual
				}
			}
			if f.kind == kindPage && smallButton(c, L("在预览里打开", "Open in the preview"), false).Tooltip(L("像浏览器那样显示这个网页", "Show the page as a browser does")).Clicked() {
				a.openPage(f.path)
				return
			}
			if iconButton(c, iconExternal, L("用默认程序打开", "Open with its app")).Clicked() {
				mygo.Shell.OpenPath(f.path)
			}
			if iconButton(c, iconFolder, L("在资源管理器中显示", "Show in Explorer")).Clicked() {
				mygo.Shell.ShowItemInFolder(f.path)
			}
			a.viewerButtons(c)
		})
		ui.Divider(c)
		switch {
		case !f.done:
			ui.Spinner(c).Margin(16)
		case f.note != "":
			ui.Text(c, f.note).FontSize(13).TextColor(t.TextMuted).Padding(16).Wrap()
		case f.player != nil:
			ui.Video(c, f.player).Grow(1).FillWidth().Fit(ui.Contain).Margin(8)
			if f.kind == kindVideo {
				ui.Row(c).FillWidth().Padding(6, 12).Gap(8).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
					label := L("暂停", "Pause")
					if !f.player.Playing() {
						label = L("播放", "Play")
					}
					if smallButton(c, label, false).Clicked() {
						if f.player.Playing() {
							f.player.Pause()
						} else {
							f.player.Play()
						}
					}
				})
			}
		case f.bmp != nil && f.actual:
			ui.ScrollBoth(c).Grow(1).FillWidth().Children(func() {
				w, h := f.bmp.Size()
				ui.Image(c, f.bmp).Size(float32(w), float32(h)).Margin(8)
			})
		case f.bmp != nil:
			ui.Image(c, f.bmp).Grow(1).FillWidth().Fit(ui.Contain).Margin(12)
		case f.svg != nil:
			ui.Image(c, f.svg).Grow(1).FillWidth().Fit(ui.Contain).Margin(24)
		case f.code != nil:
			editor.View(c, f.code).Grow(1).FillWidth()
		case f.md != "":
			ui.Scroll(c).Grow(1).FillWidth().Children(func() {
				ui.Column(c).FillWidth().MaxWidth(860).Padding(16, 24).Selectable().Children(func() {
					markdown.View(c, f.md, &a.mdOpts)
				})
			})
		default:
			// What Nook does not show: what it is, and the app to open it with.
			ui.Column(c).Grow(1).FillWidth().Center().Gap(10).Children(func() {
				icon, col := kindIcon(t, f.kind)
				ui.Icon(c, icon).FontSize(56).TextColor(col)
				ui.Text(c, filepath.Base(f.path)).FontSize(14).Bold()
				ui.Text(c, sizeText(f.size)+" · "+L("修改于 ", "changed ")+ago(f.mod)).FontSize(12).TextColor(t.TextMuted)
				ui.Row(c).Gap(8).PaddingY(6).Children(func() {
					if ui.PrimaryButton(c, L("用默认程序打开", "Open with its app")).Clicked() {
						mygo.Shell.OpenPath(f.path)
					}
					if ui.Button(c, L("在资源管理器中显示", "Show in Explorer")).Clicked() {
						mygo.Shell.ShowItemInFolder(f.path)
					}
				})
			})
		}
	})
}

// closeMainView goes back from a comparison, a file or a conversation to
// the terminals.
func (a *app) closeMainView() {
	a.closeItem(a.viewing)
	if s := a.focused(); s != nil {
		a.pendingFocus = s
	}
}

// openDouble opens a file double-clicked in the files tab: with its app,
// unless opening it would run it, as a program or a script an AI just
// wrote: that shows in Nook, and runs only from "Open with its app".
func (a *app) openDouble(path string) {
	if runnable(path) {
		a.openFile(path)
		return
	}
	mygo.Shell.OpenPath(path)
}

// runnable reports whether Windows runs the file opened with its app.
func runnable(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".exe", ".com", ".scr", ".msi", ".msix", ".appx", ".bat", ".cmd", ".ps1", ".psm1", ".vbs", ".vbe",
		".js", ".jse", ".wsf", ".wsh", ".hta", ".jar", ".lnk", ".url", ".reg", ".cpl", ".pif", ".application":
		return true
	}
	return false
}
