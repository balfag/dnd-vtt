// Package lan prepares the server for players on the local network: it lists
// the LAN addresses, keeps a self-signed certificate and serves TLS and HTTP
// on one port (ADR-0015).
package lan

import (
	"fmt"
	"net"
	"slices"
)

// Hosts returns the IP addresses of up network interfaces that players on the
// LAN can use, best first. Loopback, link-local (169.254.0.0/16, fe80::/10),
// unspecified and multicast addresses are skipped. IPv6 addresses are returned
// without brackets.
func Hosts() ([]string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("list network interfaces: %w", err)
	}
	var ips []net.IP
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, fmt.Errorf("list addresses of %s: %w", iface.Name, err)
		}
		for _, addr := range addrs {
			if n, ok := addr.(*net.IPNet); ok {
				ips = append(ips, n.IP)
			}
		}
	}
	return rankHosts(ips), nil
}

var (
	private192 = mustCIDR("192.168.0.0/16")
	private10  = mustCIDR("10.0.0.0/8")
	private172 = mustCIDR("172.16.0.0/12")
)

func mustCIDR(s string) *net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return n
}

// rankHosts sorts ips by priority: 192.168.0.0/16, 10.0.0.0/8, 172.16.0.0/12,
// other IPv4, then IPv6; inside a group the order of the input is kept
// (stable sort). It filters out the skipped kinds listed in Hosts and removes
// duplicates.
func rankHosts(ips []net.IP) []string {
	type ranked struct {
		host string
		rank int
	}
	seen := make(map[string]bool, len(ips))
	var list []ranked
	for _, ip := range ips {
		if ip == nil || ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() ||
			ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			continue
		}
		host := ip.String()
		if seen[host] {
			continue
		}
		seen[host] = true
		list = append(list, ranked{host: host, rank: rankOf(ip)})
	}
	slices.SortStableFunc(list, func(a, b ranked) int { return a.rank - b.rank })
	out := make([]string, 0, len(list))
	for _, r := range list {
		out = append(out, r.host)
	}
	return out
}

func rankOf(ip net.IP) int {
	v4 := ip.To4()
	switch {
	case v4 == nil:
		return 4
	case private192.Contains(v4):
		return 0
	case private10.Contains(v4):
		return 1
	case private172.Contains(v4):
		return 2
	default:
		return 3
	}
}
