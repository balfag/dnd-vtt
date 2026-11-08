package lan

import (
	"crypto/x509"
	"net"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

var certNow = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

func leafOf(t *testing.T, certDER [][]byte) *x509.Certificate {
	t.Helper()
	leaf, err := x509.ParseCertificate(certDER[0])
	if err != nil {
		t.Fatal(err)
	}
	return leaf
}

func TestLoadOrCreateCertCreates(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tls")
	cert, created, err := LoadOrCreateCert(dir, []string{"192.168.1.5", "fd00::5"}, certNow)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Error("created = false, want true")
	}
	for _, name := range []string{"cert.pem", "key.pem"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	leaf := leafOf(t, cert.Certificate)
	for _, s := range []string{"127.0.0.1", "::1", "192.168.1.5", "fd00::5"} {
		ip := net.ParseIP(s)
		if !slices.ContainsFunc(leaf.IPAddresses, ip.Equal) {
			t.Errorf("SAN misses %s: %v", s, leaf.IPAddresses)
		}
	}
	if !slices.Equal(leaf.DNSNames, []string{"localhost"}) {
		t.Errorf("DNSNames = %v", leaf.DNSNames)
	}
	if leaf.IsCA || leaf.Subject.CommonName != "dndvtt" {
		t.Errorf("IsCA = %v, CN = %q", leaf.IsCA, leaf.Subject.CommonName)
	}
	if want := certNow.Add(397 * 24 * time.Hour); !leaf.NotAfter.Equal(want) {
		t.Errorf("NotAfter = %v, want %v", leaf.NotAfter, want)
	}
	if want := certNow.Add(-time.Hour); !leaf.NotBefore.Equal(want) {
		t.Errorf("NotBefore = %v, want %v", leaf.NotBefore, want)
	}
}

func TestLoadOrCreateCertReuses(t *testing.T) {
	dir := t.TempDir()
	hosts := []string{"192.168.1.5"}
	first, _, err := LoadOrCreateCert(dir, hosts, certNow)
	if err != nil {
		t.Fatal(err)
	}
	second, created, err := LoadOrCreateCert(dir, hosts, certNow.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Error("created = true on the second call")
	}
	if a, b := leafOf(t, first.Certificate), leafOf(t, second.Certificate); a.SerialNumber.Cmp(b.SerialNumber) != 0 {
		t.Errorf("serial changed: %v -> %v", a.SerialNumber, b.SerialNumber)
	}
}

func TestLoadOrCreateCertReissues(t *testing.T) {
	tests := []struct {
		name    string
		hosts   []string
		now     time.Time
		corrupt bool
	}{
		{name: "new address", hosts: []string{"192.168.1.5", "10.0.0.3"}, now: certNow},
		{name: "expires in 29 days", hosts: []string{"192.168.1.5"}, now: certNow.Add(397*24*time.Hour - 29*24*time.Hour)},
		{name: "broken cert.pem", hosts: []string{"192.168.1.5"}, now: certNow, corrupt: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			first, _, err := LoadOrCreateCert(dir, []string{"192.168.1.5"}, certNow)
			if err != nil {
				t.Fatal(err)
			}
			if tt.corrupt {
				if err := os.WriteFile(filepath.Join(dir, "cert.pem"), []byte("garbage"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			second, created, err := LoadOrCreateCert(dir, tt.hosts, tt.now)
			if err != nil {
				t.Fatal(err)
			}
			if !created {
				t.Fatal("created = false, want true")
			}
			if a, b := leafOf(t, first.Certificate), leafOf(t, second.Certificate); a.SerialNumber.Cmp(b.SerialNumber) == 0 {
				t.Error("serial did not change")
			}
		})
	}
}
