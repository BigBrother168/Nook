package netguard

import (
	"encoding/binary"
	"net/netip"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var procGetExtendedTcpTable = windows.NewLazySystemDLL("iphlpapi.dll").NewProc("GetExtendedTcpTable")

const (
	tcpTableOwnerPIDListener    = 3
	tcpTableOwnerPIDConnections = 4
	afInet                      = 2
	afInet6                     = 23
	errInsufficientBuffer       = 122
)

// tcpTable returns Windows' table of TCP endpoints of class for family.
func tcpTable(family, class uint32) []byte {
	size := uint32(16 << 10)
	for range 8 {
		buf := make([]byte, size)
		r, _, _ := procGetExtendedTcpTable.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0, uintptr(family), uintptr(class), 0)
		switch r {
		case 0:
			return buf[:size]
		case errInsufficientBuffer:
			size += 4 << 10 // the table grew meanwhile
		default:
			return nil
		}
	}
	return nil
}

// port reads a port as the tables keep it, in network order in the low
// 16 bits of a DWORD.
func port(b []byte) int { return int(b[0])<<8 | int(b[1]) }

// rows4 parses MIB_TCPTABLE_OWNER_PID: state, local address and port,
// remote address and port, owner, each a DWORD.
func rows4(t []byte, fn func(local, remote Endpoint, state, pid uint32)) {
	if len(t) < 4 {
		return
	}
	n := int(binary.LittleEndian.Uint32(t))
	for i := range n {
		r := t[4+i*24:]
		if len(r) < 24 {
			return
		}
		local := Endpoint{netip.AddrFrom4([4]byte(r[4:8])), port(r[8:])}
		remote := Endpoint{netip.AddrFrom4([4]byte(r[12:16])), port(r[16:])}
		fn(local, remote, binary.LittleEndian.Uint32(r), binary.LittleEndian.Uint32(r[20:]))
	}
}

// rows6 parses MIB_TCP6TABLE_OWNER_PID: local address, scope and port,
// remote address, scope and port, state, owner.
func rows6(t []byte, fn func(local, remote Endpoint, state, pid uint32)) {
	if len(t) < 4 {
		return
	}
	n := int(binary.LittleEndian.Uint32(t))
	for i := range n {
		r := t[4+i*56:]
		if len(r) < 56 {
			return
		}
		local := Endpoint{netip.AddrFrom16([16]byte(r[0:16])), port(r[20:])}
		remote := Endpoint{netip.AddrFrom16([16]byte(r[24:40])), port(r[44:])}
		fn(local, remote, binary.LittleEndian.Uint32(r[48:]), binary.LittleEndian.Uint32(r[52:]))
	}
}

// Listeners returns the TCP endpoints programs listen on.
func Listeners() []Endpoint {
	var out []Endpoint
	add := func(local, _ Endpoint, _, _ uint32) { out = append(out, local) }
	rows4(tcpTable(afInet, tcpTableOwnerPIDListener), add)
	rows6(tcpTable(afInet6, tcpTableOwnerPIDListener), add)
	return out
}

// Connections returns every TCP connection on this computer and its owner.
func Connections() []Conn {
	var out []Conn
	add := func(local, remote Endpoint, state, pid uint32) { out = append(out, Conn{pid, local, remote, state}) }
	rows4(tcpTable(afInet, tcpTableOwnerPIDConnections), add)
	rows6(tcpTable(afInet6, tcpTableOwnerPIDConnections), add)
	return out
}

// systemProxy returns Windows' proxy for HTTPS, as "host:port", "" when
// it is off. A server per protocol ("http=a:1;https=b:2") gives https's,
// else http's.
func systemProxy() string {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	if on, _, err := k.GetIntegerValue("ProxyEnable"); err != nil || on == 0 {
		return ""
	}
	server, _, err := k.GetStringValue("ProxyServer")
	if err != nil {
		return ""
	}
	if !strings.Contains(server, "=") {
		return server
	}
	byProto := map[string]string{}
	for _, part := range strings.Split(server, ";") {
		if proto, hp, ok := strings.Cut(part, "="); ok {
			byProto[strings.ToLower(strings.TrimSpace(proto))] = strings.TrimSpace(hp)
		}
	}
	for _, proto := range []string{"https", "http"} {
		if hp := byProto[proto]; hp != "" {
			return hp
		}
	}
	return ""
}

// Adapter is a network adapter that is up.
type Adapter struct {
	Name     string
	Addrs    []netip.Addr
	Internet bool // the way out to the internet goes through it
}

// Carrying returns the tunnels the way out to the internet goes through:
// a VPN to the office, or Tailscale, may be up and carry none of it.
func Carrying(ts []Adapter) []Adapter {
	var out []Adapter
	for _, t := range ts {
		if t.Internet {
			out = append(out, t)
		}
	}
	return out
}

// bestInterface returns the adapter Windows' routing table sends sa's
// traffic to; false when none. It looks the table up and sends nothing.
func bestInterface(sa windows.Sockaddr) (uint32, bool) {
	var i uint32
	return i, windows.GetBestInterfaceEx(sa, &i) == nil
}

// Any address out on the internet stands for all of it.
var (
	internet4 = &windows.SockaddrInet4{Addr: [4]byte{1, 1, 1, 1}}
	internet6 = &windows.SockaddrInet6{Addr: netip.MustParseAddr("2606:4700:4700::1111").As16()}
)

// tunWords mark the adapters of tunnels: sing-box, Clash and mihomo,
// WireGuard's wintun, and the like.
var tunWords = []string{"tun", "wintun", "sing", "clash", "mihomo", "meta", "wireguard", "tap-"}

// Tunnels returns the tunnel adapters that are up, with their addresses,
// and whether the internet goes through each: IPv4 and IPv6 both, where
// this computer has a route for them, else one could leak around it.
func Tunnels() []Adapter {
	v4, has4 := bestInterface(internet4)
	v6, has6 := bestInterface(internet6)
	size := uint32(16 << 10)
	var buf []byte
	for range 4 {
		buf = make([]byte, size)
		err := windows.GetAdaptersAddresses(windows.AF_UNSPEC, windows.GAA_FLAG_SKIP_ANYCAST|windows.GAA_FLAG_SKIP_MULTICAST|windows.GAA_FLAG_SKIP_DNS_SERVER,
			0, (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])), &size)
		if err == nil {
			break
		}
		if err != windows.ERROR_BUFFER_OVERFLOW {
			return nil
		}
	}
	var out []Adapter
	for a := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])); a != nil; a = a.Next {
		if a.OperStatus != windows.IfOperStatusUp {
			continue
		}
		name := windows.UTF16PtrToString(a.FriendlyName)
		desc := windows.UTF16PtrToString(a.Description)
		low := strings.ToLower(name + " " + desc)
		// Tunnels by type, whatever their names: wintun and most VPNs are
		// virtual (53) or tunnel (131) interfaces; then by name.
		tun := a.IfType == 53 || a.IfType == 131
		for _, w := range tunWords {
			if strings.Contains(low, w) {
				tun = true
				break
			}
		}
		if !tun {
			continue
		}
		ad := Adapter{Name: name, Internet: (has4 || has6) && (!has4 || a.IfIndex == v4) && (!has6 || a.Ipv6IfIndex == v6)}
		for u := a.FirstUnicastAddress; u != nil; u = u.Next {
			if ip, ok := netip.AddrFromSlice(u.Address.IP()); ok {
				ad.Addrs = append(ad.Addrs, ip.Unmap())
			}
		}
		out = append(out, ad)
	}
	return out
}
