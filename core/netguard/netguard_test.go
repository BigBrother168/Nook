package netguard

import (
	"net"
	"net/netip"
	"testing"
)

func ep(s string) Endpoint {
	ap := netip.MustParseAddrPort(s)
	return Endpoint{ap.Addr(), int(ap.Port())}
}

func TestClassify(t *testing.T) {
	p := Proxy{Host: "127.0.0.1", Port: 10808}
	tun := []netip.Addr{netip.MustParseAddr("172.18.0.1")}
	cases := []struct {
		local, remote string
		want          Route
	}{
		{"127.0.0.1:50000", "127.0.0.1:10808", ViaProxy},
		{"[::1]:50000", "[::1]:10808", ViaProxy},
		{"127.0.0.1:50000", "127.0.0.1:5173", Local},
		{"172.18.0.1:50000", "160.79.104.10:443", ViaTUN},
		{"192.168.1.5:50000", "192.168.1.1:80", LAN},
		{"192.168.1.5:50000", "160.79.104.10:443", Direct},
		{"[2001:db8::5]:50000", "[2606:4700::1]:443", Direct},
	}
	for _, c := range cases {
		got := Classify(Conn{Local: ep(c.local), Remote: ep(c.remote)}, p, tun)
		if got != c.want {
			t.Errorf("%s → %s: %d, want %d", c.local, c.remote, got, c.want)
		}
	}
	// Without a proxy, a loopback connection is local.
	if got := Classify(Conn{Local: ep("127.0.0.1:1"), Remote: ep("127.0.0.1:10808")}, Proxy{}, nil); got != Local {
		t.Errorf("no proxy: %d", got)
	}
}

func TestParseHostPort(t *testing.T) {
	cases := map[string]string{
		"127.0.0.1:10808":          "127.0.0.1:10808",
		"http://127.0.0.1:7890/":   "127.0.0.1:7890",
		"socks5://user:pw@[::1]:1": "::1:1",
		"localhost:8080":           "127.0.0.1:8080",
		"no port":                  "",
		"host:99999":               "",
	}
	for in, want := range cases {
		p := parseHostPort(in)
		got := ""
		if p.Port != 0 {
			got = p.Host + ":" + itoa(p.Port)
		}
		if got != want {
			t.Errorf("parseHostPort(%q) = %q, want %q", in, got, want)
		}
	}
}

func itoa(n int) string {
	s := ""
	for ; n > 0; n /= 10 {
		s = string(rune('0'+n%10)) + s
	}
	return s
}

// TestLive reads this computer's tables: they parse, and Nook's test
// process itself has no connection.
func TestLive(t *testing.T) {
	ls := Listeners()
	if len(ls) == 0 {
		t.Fatal("no listeners: the table did not parse")
	}
	p := DetectProxy("")
	t.Logf("proxy %s (%s), listening %v; tunnels %+v (the internet through %v); %d connections", p.Addr(), p.Source, p.Listening, Tunnels(), Carrying(Tunnels()), len(Connections()))
	procs := Snapshot()
	if len(procs) < 10 {
		t.Fatalf("%d processes", len(procs))
	}
}

func TestEnvProxy(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if p := EnvProxy([]string{"PATH=x", "https_proxy=http://" + l.Addr().String()}); !p.Works() {
		t.Errorf("listening proxy in env: %+v", p)
	}
	if p := EnvProxy([]string{"HTTP_PROXY=http://127.0.0.1:1"}); p.Port != 1 || p.Works() {
		t.Errorf("dead proxy in env: %+v", p)
	}
	if p := EnvProxy([]string{"=C:=C:\\", "PATH=x"}); p.Port != 0 {
		t.Errorf("no proxy in env: %+v", p)
	}
}

// TestCount: a program talking to itself, or closing a connection, adds
// nothing; a direct connection counts even as it closes.
func TestCount(t *testing.T) {
	p := Proxy{Host: "127.0.0.1", Port: 10808}
	conn := func(pid uint32, local, remote string, state uint32) Conn {
		return Conn{PID: pid, Local: ep(local), Remote: ep(remote), State: state}
	}
	byPID := map[uint32][]Conn{}
	for _, c := range []Conn{
		conn(1, "127.0.0.1:5000", "127.0.0.1:10808", 5), // through the proxy
		conn(1, "127.0.0.1:5001", "127.0.0.1:10808", 7), // closing
		conn(1, "127.0.0.1:6000", "127.0.0.1:6001", 5),  // 1 and 2 talking
		conn(2, "127.0.0.1:6001", "127.0.0.1:6000", 5),
		conn(1, "127.0.0.1:6100", "127.0.0.1:6101", 5), // 1 talking to itself
		conn(1, "127.0.0.1:6101", "127.0.0.1:6100", 5),
		conn(2, "127.0.0.1:7000", "127.0.0.1:3000", 5), // to another program here
		conn(99, "127.0.0.1:3000", "127.0.0.1:7000", 5),
		conn(1, "10.0.0.2:7100", "1.2.3.4:443", 8), // straight out, closing
	} {
		byPID[c.PID] = append(byPID[c.PID], c)
	}
	got := Count(byPID, map[uint32]bool{1: true, 2: true}, p, nil)
	want := Routes{ViaProxy: 1, Local: 1, Direct: 1}
	if got != want {
		t.Errorf("Count = %v, want %v", got, want)
	}
}
