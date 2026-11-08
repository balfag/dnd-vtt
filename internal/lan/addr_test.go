package lan

import (
	"net"
	"slices"
	"testing"
)

func TestRankHosts(t *testing.T) {
	ip := net.ParseIP
	tests := []struct {
		name string
		in   []net.IP
		want []string
	}{
		{name: "empty", in: nil, want: []string{}},
		{
			name: "group order",
			in:   []net.IP{ip("fd00::5"), ip("8.8.4.4"), ip("172.16.0.2"), ip("10.0.0.3"), ip("192.168.1.5")},
			want: []string{"192.168.1.5", "10.0.0.3", "172.16.0.2", "8.8.4.4", "fd00::5"},
		},
		{
			name: "stable inside a group",
			in:   []net.IP{ip("192.168.2.1"), ip("10.1.1.1"), ip("192.168.1.1"), ip("10.0.0.1")},
			want: []string{"192.168.2.1", "192.168.1.1", "10.1.1.1", "10.0.0.1"},
		},
		{
			name: "skipped kinds",
			in: []net.IP{
				ip("127.0.0.1"), ip("::1"), ip("169.254.1.1"), ip("fe80::1"),
				ip("0.0.0.0"), ip("::"), ip("224.0.0.1"), ip("ff02::1"), ip("192.168.1.5"),
			},
			want: []string{"192.168.1.5"},
		},
		{
			name: "duplicates",
			in:   []net.IP{ip("192.168.1.5"), ip("192.168.1.5").To4(), ip("10.0.0.3"), ip("10.0.0.3")},
			want: []string{"192.168.1.5", "10.0.0.3"},
		},
		{name: "172 outside the private range", in: []net.IP{ip("172.32.0.1"), ip("172.31.0.1")}, want: []string{"172.31.0.1", "172.32.0.1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rankHosts(tt.in)
			if got == nil || !slices.Equal(got, tt.want) {
				t.Errorf("rankHosts = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestHosts(t *testing.T) {
	hosts, err := Hosts()
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hosts {
		ip := net.ParseIP(h)
		if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			t.Errorf("Hosts returned %q", h)
		}
	}
}
