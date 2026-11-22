package checkpoint

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"nook/core/lang"
)

// What a copy leaves out. Besides the folders of excludes and what the
// project's .gitignore says, a copy leaves out files too large to keep:
// any over maxFile, and data over bigData (pictures, videos, archives,
// models, datasets) that an AI writing code hardly changes. They are
// noted, by size and time, in a list of the repository's own
// (nook-skipped.txt): what the AI does to them shows, to be known, not
// undone. And a first copy over maxTotal is not made: changes are not
// tracked there, until the project's .gitignore leaves out enough.

var (
	// bigData is the size over which data, not text, is left out.
	bigData int64 = 1 << 20
	// maxTotal is the most a first copy keeps.
	maxTotal int64 = 1 << 30
)

// dataExt are the kinds of files that are data, told by name: the rest
// are told by their first bytes, as git tells binary files.
var dataExt = map[string]bool{}

func init() {
	for _, e := range strings.Fields(`
		jpg jpeg png gif bmp tif tiff webp heic heif ico psd raw dng arw cr2 cr3 nef orf rw2 exr hdr
		mp4 mov avi mkv wmv flv webm m4v mts m2ts 3gp mp3 wav flac aac ogg m4a wma
		zip rar 7z tar gz tgz bz2 xz zst lz4 iso dmg cab msi exe dll so dylib lib a
		npy npz h5 hdf5 pt pth ckpt onnx safetensors gguf tflite pb pkl pickle joblib
		parquet feather arrow tfrecord lmdb mdb db sqlite sqlite3 mat las laz ply pcd bin dat
		pdf docx xlsx pptx woff woff2 ttf otf`) {
		dataExt["."+e] = true
	}
}

// leaveOut reports whether the file at path, of size bytes, stays out of
// copies.
func leaveOut(path string, size int64) bool {
	if size > maxFile {
		return true
	}
	if size <= bigData {
		return false
	}
	if dataExt[strings.ToLower(filepath.Ext(path))] {
		return true
	}
	return isBinary(path)
}

// isBinary reports whether a file is not text: a zero byte in its first
// 8000, as git decides. A file that cannot be read is not.
func isBinary(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	b := make([]byte, 8000)
	n, _ := io.ReadFull(f, b)
	return bytes.IndexByte(b[:n], 0) >= 0
}

// skipped is a file left out of the copies, as it was when noted.
type skipped struct {
	base  bool // there before the AI; else the AI made it
	size  int64
	mtime int64 // UnixNano
}

func (r *Repo) skippedFile() string { return filepath.Join(r.gitDir, "nook-skipped.txt") }

// readSkipped reads the list of files left out, by path.
func (r *Repo) readSkipped() map[string]skipped {
	m := map[string]skipped{}
	b, err := os.ReadFile(r.skippedFile())
	if err != nil {
		return m
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.SplitN(line, "\t", 4)
		if len(f) != 4 || f[3] == "" {
			continue
		}
		size, _ := strconv.ParseInt(f[1], 10, 64)
		mtime, _ := strconv.ParseInt(f[2], 10, 64)
		m[f[3]] = skipped{base: f[0] == "B", size: size, mtime: mtime}
	}
	return m
}

func (r *Repo) writeSkipped(m map[string]skipped) error {
	paths := make([]string, 0, len(m))
	for p := range m {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var b strings.Builder
	for _, p := range paths {
		s := m[p]
		kind := "N"
		if s.base {
			kind = "B"
		}
		fmt.Fprintf(&b, "%s\t%d\t%d\t%s\n", kind, s.size, s.mtime, p)
	}
	return os.WriteFile(r.skippedFile(), []byte(b.String()), 0o644)
}

// noted is a file as it is now, for the list: ok is false when it is gone.
func (r *Repo) noted(path string, base bool) (s skipped, ok bool) {
	fi, err := os.Stat(filepath.Join(r.Project, filepath.FromSlash(path)))
	if err != nil {
		return skipped{}, false
	}
	return skipped{base: base, size: fi.Size(), mtime: fi.ModTime().UnixNano()}, true
}

// unsaved returns what changed of the files left out: those there before
// changed or gone, those the AI made.
func (r *Repo) unsaved(m map[string]skipped) []Change {
	var cs []Change
	for p, s := range m {
		now, ok := r.noted(p, s.base)
		switch {
		case !s.base && ok:
			cs = append(cs, Change{Path: p, Status: 'A', Binary: true, Unsaved: true})
		case s.base && !ok:
			cs = append(cs, Change{Path: p, Status: 'D', Binary: true, Unsaved: true})
		case s.base && (now.size != s.size || now.mtime != s.mtime):
			cs = append(cs, Change{Path: p, Status: 'M', Binary: true, Unsaved: true})
		}
	}
	sort.Slice(cs, func(i, j int) bool { return cs[i].Path < cs[j].Path })
	return cs
}

// TooMuch is why no first copy was made: the files to keep come to Size.
type TooMuch struct{ Size int64 }

func (e TooMuch) Error() string {
	return fmt.Sprintf(lang.L("要备份的文件有 %s，超过 1 GB，没有记录改动。把不用备份的文件夹写进项目的 .gitignore 就行。", "The files to keep come to %s, over 1 GB: changes are not tracked. Add the folders not to keep to the project's .gitignore."), sizeText(e.Size))
}

func sizeText(n int64) string {
	if n >= 1<<30 {
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	}
	return fmt.Sprintf("%d MB", n>>20)
}
