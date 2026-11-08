package lan

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

const (
	certFile      = "cert.pem"
	keyFile       = "key.pem"
	minValidity   = 30 * 24 * time.Hour
	certLifetime  = 397 * 24 * time.Hour
	certBackdated = time.Hour
)

// LoadOrCreateCert returns the certificate stored in dir (cert.pem, key.pem)
// if it is valid for at least 30 more days at now and its IP SANs contain
// every address in hosts; otherwise it creates a new one, writes both files
// with mode 0600 (dir with 0700) and returns it. The new certificate:
// ECDSA P-256 key, self-signed, serial 128 random bits, NotBefore now-1h,
// NotAfter now+397 days, CN "dndvtt", DNSNames ["localhost"],
// IPAddresses 127.0.0.1, ::1 and every address of hosts,
// KeyUsage DigitalSignature, ExtKeyUsage ServerAuth, IsCA false.
// created reports whether a new certificate was written.
func LoadOrCreateCert(dir string, hosts []string, now time.Time) (cert tls.Certificate, created bool, err error) {
	if cert, ok := loadCert(dir, hosts, now); ok {
		return cert, false, nil
	}
	cert, err = createCert(dir, hosts, now)
	if err != nil {
		return tls.Certificate{}, false, err
	}
	return cert, true, nil
}

// loadCert returns the stored certificate if it is usable for hosts at now.
func loadCert(dir string, hosts []string, now time.Time) (tls.Certificate, bool) {
	cert, err := tls.LoadX509KeyPair(filepath.Join(dir, certFile), filepath.Join(dir, keyFile))
	if err != nil || len(cert.Certificate) == 0 {
		return tls.Certificate{}, false
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return tls.Certificate{}, false
	}
	if leaf.NotAfter.Sub(now) < minValidity {
		return tls.Certificate{}, false
	}
	for _, h := range hosts {
		ip := net.ParseIP(h)
		if ip == nil {
			continue
		}
		found := false
		for _, have := range leaf.IPAddresses {
			if have.Equal(ip) {
				found = true
				break
			}
		}
		if !found {
			return tls.Certificate{}, false
		}
	}
	cert.Leaf = leaf
	return cert, true
}

func createCert(dir string, hosts []string, now time.Time) (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate serial number: %w", err)
	}
	ips := []net.IP{net.ParseIP("127.0.0.1"), net.IPv6loopback}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			ips = append(ips, ip)
		}
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "dndvtt"},
		NotBefore:             now.Add(-certBackdated),
		NotAfter:              now.Add(certLifetime),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
		DNSNames:              []string{"localhost"},
		IPAddresses:           ips,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("create certificate: %w", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("marshal key: %w", err)
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return tls.Certificate{}, fmt.Errorf("create tls dir: %w", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(filepath.Join(dir, keyFile), keyPEM, 0o600); err != nil {
		return tls.Certificate{}, fmt.Errorf("write key: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, certFile), certPEM, 0o600); err != nil {
		return tls.Certificate{}, fmt.Errorf("write certificate: %w", err)
	}

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("load new certificate: %w", err)
	}
	return cert, nil
}
