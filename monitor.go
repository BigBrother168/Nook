package main

import (
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"nook/core/config"
	"nook/core/history"
	"nook/core/netguard"
)

// netState is what the monitor last saw, for the title bar and the panel.
type netState struct {
	proxy   netguard.Proxy
	tunnels []netguard.Adapter
	self    int           // Nook's own connections: always 0
	ai      map[int]aiUse // by session ID: AI tools running in Nook's terminals
	others  []otherAI     // AI tools running elsewhere
}

// aiUse is the AI tools running in one terminal and where they connect.
type aiUse struct {
	tools  []string
	pids   []uint32 // the tools' topmost processes
	routes netguard.Routes
	conv   history.Ref // the conversation they are in, when known
}

// otherAI is an AI tool Nook did not start, and its connections.
type otherAI struct {
	pid        uint32
	name       string
	parent     string // the program it runs under, as "ChatGPT.exe"
	parentPath string // that program's file
	cmd        string // how it was started, which tells what it does
	routes     netguard.Routes
}

// running counts the terminals running an AI tool and their direct
// connections.
func (n netState) running() (terminals, direct int) {
	for _, u := range n.ai {
		terminals++
		direct += u.routes[netguard.Direct]
	}
	return
}

// online counts the connections of AI tools, in Nook or not, that go
// out: through the proxy, a tunnel or straight; none, they have not gone
// online yet, and nothing is said to go through the proxy.
func (n netState) online() int {
	c := 0
	add := func(r netguard.Routes) { c += r[netguard.ViaProxy] + r[netguard.ViaTUN] + r[netguard.Direct] }
	for _, u := range n.ai {
		add(u.routes)
	}
	for _, o := range n.others {
		add(o.routes)
	}
	return c
}

// elsewhereDirect counts the direct connections of AI tools outside Nook.
func (n netState) elsewhereDirect() int {
	d := 0
	for _, o := range n.others {
		d += o.routes[netguard.Direct]
	}
	return d
}

// live is the sessions the monitor looks at, kept apart from the view's
// state, which only the main thread touches.
type live struct {
	mu       sync.Mutex
	sessions map[int]*session
	tools    map[string]bool
	proxy    string // set by hand
}

func (l *live) add(s *session) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.sessions == nil {
		l.sessions = map[int]*session{}
	}
	l.sessions[s.id] = s
}

func (l *live) remove(s *session) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.sessions, s.id)
}

func (l *live) configure(s config.Settings) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.tools = map[string]bool{}
	for _, t := range s.AITools {
		l.tools[strings.ToLower(t)] = true
	}
	l.proxy = s.Proxy
}

// monitor looks every two seconds, and tells the window what changed.
func (a *app) monitor() {
	last := ""
	for {
		n := a.scan()
		if sum := n.summary(); sum != last {
			last = sum
			a.probe.log("net %s", sum)
			a.win.Update(func() { a.setNet(n) })
		}
		a.win.Update(a.onTick)
		time.Sleep(2 * time.Second)
	}
}

// scan reads the proxy, the tunnels, the processes and every TCP
// connection on this computer, all from Windows' own tables.
func (a *app) scan() netState {
	a.live.mu.Lock()
	type sess struct {
		id    int
		pid   uint32
		since time.Time
		dir   string
	}
	var ss []sess
	for _, s := range a.live.sessions {
		if pid := s.conn.Pid(); pid != 0 {
			ss = append(ss, sess{s.id, uint32(pid), s.conn.Since(), s.dir})
		}
	}
	tools, manual := a.live.tools, a.live.proxy
	a.live.mu.Unlock()

	n := netState{ai: map[int]aiUse{}}
	n.proxy = netguard.DetectProxy(manual)
	n.tunnels = netguard.Tunnels()
	var tunIPs []netip.Addr
	for _, t := range n.tunnels {
		tunIPs = append(tunIPs, t.Addrs...)
	}
	procs := netguard.Snapshot()
	kids := netguard.Children(procs)
	byPID := map[uint32][]netguard.Conn{}
	for _, c := range netguard.Connections() {
		byPID[c.PID] = append(byPID[c.PID], c)
	}
	n.self = len(byPID[uint32(os.Getpid())])

	isTool := func(pid uint32) bool { return tools[procs[pid].Tool()] }
	// topmost: an AI tool whose parent is not one, as the guard named
	// claude.exe over the real claude.exe.
	topmost := func(pid uint32) bool { return isTool(pid) && !isTool(procs[pid].Parent) }
	// underTool: pid runs under an AI tool further up, as the real
	// codex.exe under node under the guard named codex.exe.
	underTool := func(pid uint32) bool {
		for range 32 {
			p, ok := procs[pid]
			if !ok {
				return false
			}
			parent, ok := procs[p.Parent]
			if !ok || p.Parent == pid || netguard.Started(p.Parent).After(netguard.Started(pid)) {
				return false // ended, or its ID taken by a later process
			}
			if isTool(p.Parent) {
				return true
			}
			pid = parent.PID
		}
		return false
	}
	count := func(pids map[uint32]bool) netguard.Routes {
		return netguard.Count(byPID, pids, n.proxy, tunIPs)
	}

	inNook := map[uint32]bool{}
	taken := map[string]bool{} // conversations found in other terminals
	for _, s := range ss {
		var use aiUse
		// Each tool running, with all its processes and when it started.
		type run struct {
			pids  []uint32
			since time.Time
		}
		runs := map[string]*run{}
		mine := map[uint32]bool{} // the tools and all they started, each once
		for _, p := range netguard.Tree(kids, s.pid, s.since) {
			inNook[p] = true
			if topmost(p) {
				tool := procs[p].Tool()
				if !slices.Contains(use.tools, tool) {
					use.tools = append(use.tools, tool)
				}
				use.pids = append(use.pids, p)
				started := netguard.Started(p)
				tree := netguard.Tree(kids, p, started)
				for _, q := range tree {
					mine[q] = true
				}
				rn := runs[tool]
				if rn == nil {
					rn = &run{since: started}
					runs[tool] = rn
				}
				if started.Before(rn.since) {
					rn.since = started
				}
				rn.pids = append(rn.pids, tree...)
			}
		}
		use.routes = count(mine)
		// The terminal's conversation: that of the tool started last.
		var latest time.Time
		for tool, r := range runs {
			if ref := history.Find(tool, r.pids, s.dir, r.since, taken); ref.ID != "" && !r.since.Before(latest) {
				use.conv, latest = ref, r.since
			}
		}
		if use.conv.ID != "" {
			taken[use.conv.Key()] = true
		}
		if len(use.pids) > 0 {
			n.ai[s.id] = use
		}
	}
	for pid, p := range procs {
		if inNook[pid] || !topmost(pid) || underTool(pid) || pid == uint32(os.Getpid()) {
			continue
		}
		parent, parentPath := "—", ""
		if pp, ok := procs[p.Parent]; ok {
			parent, parentPath = pp.Name, netguard.ImagePath(p.Parent)
		}
		n.others = append(n.others, otherAI{pid: pid, name: p.Name, parent: parent, parentPath: parentPath, cmd: netguard.CommandLine(pid), routes: count(treeSet(netguard.Tree(kids, pid, netguard.Started(pid))))})
	}
	sort.Slice(n.others, func(i, j int) bool { return n.others[i].pid < n.others[j].pid })
	return n
}

// treeSet returns pids as a set.
func treeSet(pids []uint32) map[uint32]bool {
	out := make(map[uint32]bool, len(pids))
	for _, p := range pids {
		out[p] = true
	}
	return out
}

// summary is what the view shows of n, to redraw only when it changes.
func (n netState) summary() string {
	ids := make([]int, 0, len(n.ai))
	for id := range n.ai {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s %v %v %d|", n.proxy.Addr(), n.proxy.Source, n.proxy.Listening, n.tunnels, n.self)
	for _, id := range ids {
		fmt.Fprintf(&b, "%d %v %v %s|", id, n.ai[id].tools, n.ai[id].routes, n.ai[id].conv.Key())
	}
	for _, o := range n.others {
		fmt.Fprintf(&b, "%d %s %v|", o.pid, o.parent, o.routes)
	}
	return b.String()
}

// setNet takes what the monitor saw, on the main thread, and says so when
// an AI tool in Nook connects straight out, around the proxy.
func (a *app) setNet(n netState) {
	for id, u := range n.ai {
		if d := u.routes[netguard.Direct]; d > 0 && a.net.ai[id].routes[netguard.Direct] == 0 {
			if s := a.sessionByID(id); s != nil {
				a.notice = fmt.Sprintf(L("⚠ %s 里的 %s 有 %s没走代理，会露出真实 IP", "⚠ %[2]s in %[1]s has %[3]s around the proxy, showing your real IP"), a.projectLabel(s), strings.Join(u.tools, L("、", ", ")), conns(d))
				a.tell(s, fmt.Sprintf(L("⚠ %s没走代理，会露出真实 IP", "⚠ %s around the proxy, showing your real IP"), conns(d)))
			}
		}
	}
	if d := n.elsewhereDirect(); d > 0 && a.net.elsewhereDirect() == 0 {
		a.notice = fmt.Sprintf(L("⚠ Nook 外面的 AI 有 %s没走代理，会露出真实 IP（点顶栏看是哪个）", "⚠ AI tools outside Nook have %s around the proxy, showing your real IP (see the top bar)"), conns(d))
	}
	// An AI just found in a terminal: the files as it finds them are kept,
	// for undoing what it changes.
	for id := range n.ai {
		if _, was := a.net.ai[id]; !was {
			if s := a.sessionByID(id); s != nil {
				a.aiStarts(s.proj)
			}
		}
	}
	a.net = n
	for id, u := range n.ai {
		if s := a.sessionByID(id); s != nil && u.conv.ID != "" {
			a.track(s, u.conv)
		}
	}
}

// endAI ends the AI tools running in Nook's terminals, and what they
// started; the shells stay.
func (a *app) endAI() {
	procs := netguard.Snapshot()
	kids := netguard.Children(procs)
	for _, u := range a.net.ai {
		for _, pid := range u.pids {
			tree := netguard.Tree(kids, pid, netguard.Started(pid))
			for i := len(tree) - 1; i >= 0; i-- {
				terminate(tree[i])
			}
		}
	}
}

func (a *app) sessionByID(id int) *session {
	for _, s := range a.sessions() {
		if s.id == id {
			return s
		}
	}
	return nil
}

// projectLabel names a session's project, or "终端" for none.
func (a *app) projectLabel(s *session) string {
	if s.proj == "" {
		return L("终端", "Terminal")
	}
	return a.projectName(s.proj)
}

// proxyEnv is what Nook adds to a new terminal's environment: the proxy,
// when one runs, for the programs that read HTTP(S)_PROXY, as AI tools
// do. Without one, nothing.
func (a *app) proxyEnv() []string {
	p := a.net.proxy
	if !p.Works() {
		return nil
	}
	return []string{"HTTP_PROXY=" + p.URL(), "HTTPS_PROXY=" + p.URL(), "NO_PROXY=localhost,127.0.0.1,::1"}
}

// termLabel is what tabs and the sidebar call a terminal: the AI running
// in it, as people know it, and which of its conversations, so that two
// terminals of one AI tell apart: the conversation's title, else the
// terminal's when that says more than the folder or the shell (Claude
// Code titles itself with the task, agy with the folder), else a number.
func (a *app) termLabel(s *session) string {
	base := a.termName(s)
	// Terminals named alike opened before this one: it is the next. By
	// when they opened, not where they are, so that a number stays with
	// its terminal as it is dragged.
	n := 1
	for _, o := range a.sessions() {
		if o.id < s.id && a.termName(o) == base {
			n++
		}
	}
	if n > 1 {
		return base + " " + itoa(n)
	}
	return base
}

// termName is termLabel without the number.
func (a *app) termName(s *session) string {
	t := s.label()
	u := a.net.ai[s.id]
	if len(u.tools) == 0 {
		return t
	}
	tool := u.tools[0]
	name := history.Names(tool)
	if s.conv.Tool == tool {
		if title := a.liveTitle(s.conv); title != "" {
			return name + " · " + clip(strings.Join(strings.Fields(title), " "), 18)
		}
	}
	low := strings.ToLower(t)
	if t == s.shell.Name || strings.EqualFold(t, a.projectLabel(s)) || strings.EqualFold(t, filepath.Base(s.dir)) ||
		strings.Contains(low, strings.ToLower(name)) || strings.Contains(low, strings.ToLower(tool)) {
		return name
	}
	return name + " · " + t
}

// clip shortens s to n characters.
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
