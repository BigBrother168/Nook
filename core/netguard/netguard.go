// Package netguard looks at the network from this computer alone: where
// the proxy is and whether it runs, which adapters are tunnels (TUN), and
// where programs' connections go. It reads Windows' own tables; it never
// connects anywhere itself.
package netguard

import (
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
)

// Proxy is the local proxy AI tools should go through.
type Proxy struct {
	Host      string // "127.0.0.1"
	Port      int
	Source    string // where Nook found it: 手动设置, 系统代理, 环境变量, 常用端口
	Listening bool   // a program listens on it now
}

// Addr returns "host:port", "" without a proxy.
func (p Proxy) Addr() string {
	if p.Port == 0 {
		return ""
	}
	return net.JoinHostPort(p.Host, strconv.Itoa(p.Port))
}

// Works reports whether p runs, as far as this computer can tell: a local
// proxy listens; one elsewhere on the network is taken at its word.
func (p Proxy) Works() bool {
	return p.Port != 0 && (p.Listening || !IsLocal(p.Host))
}

// URL returns the proxy as HTTP_PROXY takes it.
func (p Proxy) URL() string {
	if p.Port == 0 {
		return ""
	}
	return "http://" + p.Addr()
}

// CommonPorts are the ports local proxies listen on by default: v2rayN
// and xray (10808, 10809), Clash and mihomo (7890, 7897), sing-box
// (2080), shadowsocks (1080).
var CommonPorts = []int{10808, 10809, 7890, 7897, 7891, 2080, 1080, 1081, 8080, 20171, 20172}

// DetectProxy finds the proxy: the one set by hand (manual, "host:port"),
// else Windows' system proxy, else HTTPS_PROXY / HTTP_PROXY, else a
// common port something listens on; and says whether it listens now.
func DetectProxy(manual string) Proxy {
	listeners := Listeners()
	var p Proxy
	switch {
	case manual != "":
		p = parseHostPort(manual)
		p.Source = "手动设置"
	default:
		if hp := systemProxy(); hp != "" {
			p = parseHostPort(hp)
			p.Source = "系统代理"
		}
		if p.Port == 0 {
			if p = fromEnv(os.Environ()); p.Port != 0 {
				p.Source = "环境变量"
			}
		}
		if p.Port == 0 {
			for _, port := range CommonPorts {
				if listens(listeners, port) {
					p = Proxy{Host: "127.0.0.1", Port: port, Source: "常用端口"}
					break
				}
			}
		}
	}
	if p.Port != 0 && IsLocal(p.Host) {
		p.Listening = listens(listeners, p.Port)
	}
	return p
}

// ProxyVars are the variables programs read their proxy from.
var ProxyVars = []string{"HTTPS_PROXY", "HTTP_PROXY", "ALL_PROXY"}

// fromEnv finds the proxy that env names. Windows' variable names are in
// any case.
func fromEnv(env []string) Proxy {
	for _, key := range ProxyVars {
		for _, kv := range env {
			if k, v, _ := strings.Cut(kv, "="); strings.EqualFold(k, key) {
				if p := parseHostPort(v); p.Port != 0 {
					return p
				}
			}
		}
	}
	return Proxy{}
}

// EnvProxy is the proxy a program started with env goes through, and
// whether it runs now.
func EnvProxy(env []string) Proxy {
	p := fromEnv(env)
	if p.Port != 0 {
		p.Source = "环境变量"
		p.Listening = IsLocal(p.Host) && listens(Listeners(), p.Port)
	}
	return p
}

// parseHostPort reads "host:port", "http://host:port/", "socks5://…".
func parseHostPort(s string) Proxy {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.LastIndex(s, "@"); i >= 0 {
		s = s[i+1:]
	}
	s = strings.TrimSuffix(s, "/")
	host, port, err := net.SplitHostPort(s)
	if err != nil {
		return Proxy{}
	}
	n, err := strconv.Atoi(port)
	if err != nil || n <= 0 || n > 65535 {
		return Proxy{}
	}
	if host == "" || host == "localhost" {
		host = "127.0.0.1"
	}
	return Proxy{Host: host, Port: n}
}

// isLocal reports whether host is this computer.
func IsLocal(host string) bool {
	if host == "localhost" {
		return true
	}
	a, err := netip.ParseAddr(host)
	return err == nil && (a.IsLoopback() || a.IsUnspecified())
}

// listens reports whether a listener on a loopback or every address has
// port.
func listens(ls []Endpoint, port int) bool {
	for _, l := range ls {
		if l.Port == port && (l.Addr.IsLoopback() || l.Addr.IsUnspecified()) {
			return true
		}
	}
	return false
}

// Endpoint is an address and a port.
type Endpoint struct {
	Addr netip.Addr
	Port int
}

// Conn is a TCP connection and the process that owns it.
type Conn struct {
	PID    uint32
	Local  Endpoint
	Remote Endpoint
	State  uint32 // as Windows' MIB_TCP_STATE: 3 and 4 connecting, 5 open, 6 to 12 closing
}

// Open reports whether c is connecting or open, not on its way out: a
// connection one side has closed may linger for minutes.
func (c Conn) Open() bool { return c.State >= 3 && c.State <= 5 }

// Count counts the connections of the processes pids, as a program and
// its helpers. Left out: those between two of them, as a program talking
// to itself on this computer (both ends would count, and it is not going
// online), and those closing, unless they went straight out: a direct
// connection, even short-lived, is what must not go unseen.
func Count(byPID map[uint32][]Conn, pids map[uint32]bool, p Proxy, tun []netip.Addr) Routes {
	ends := map[[2]Endpoint]uint32{} // a connection's two ends, local first: its owner
	for pid := range pids {
		for _, c := range byPID[pid] {
			ends[[2]Endpoint{c.Local, c.Remote}] = pid
		}
	}
	var r Routes
	for pid := range pids {
		for _, c := range byPID[pid] {
			if _, inside := ends[[2]Endpoint{c.Remote, c.Local}]; inside {
				continue
			}
			route := Classify(c, p, tun)
			if c.Open() || route == Direct {
				r[route]++
			}
		}
	}
	return r
}

// Route is where a connection goes.
type Route int

const (
	ViaProxy Route = iota // to the local proxy
	ViaTUN                // out through a tunnel adapter
	Local                 // to another program on this computer
	LAN                   // to the local network
	Direct                // straight out: neither proxy nor tunnel
	nRoutes
)

// Routes counts connections by route.
type Routes [nRoutes]int

// Total is the number of connections counted.
func (r Routes) Total() int {
	n := 0
	for _, v := range r {
		n += v
	}
	return n
}

// Classify says where c goes, given the proxy and the tunnels' addresses.
func Classify(c Conn, p Proxy, tun []netip.Addr) Route {
	remote := c.Remote.Addr.Unmap()
	if remote.IsLoopback() || remote.IsUnspecified() {
		if p.Port != 0 && c.Remote.Port == p.Port && IsLocal(p.Host) {
			return ViaProxy
		}
		return Local
	}
	if p.Port != 0 && c.Remote.Port == p.Port && remote.String() == p.Host {
		return ViaProxy // a proxy elsewhere on the network
	}
	local := c.Local.Addr.Unmap()
	for _, t := range tun {
		if local == t.Unmap() {
			return ViaTUN
		}
	}
	if remote.IsPrivate() || remote.IsLinkLocalUnicast() {
		return LAN
	}
	return Direct
}
