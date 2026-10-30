// Command tls is a throw-away autocert prototype for the connectivity spike
// (#8): an HTTPS server that obtains a Let's Encrypt certificate for a DNS name
// pointing at the machine (for example 203-0-113-5.sslip.io). It does nothing
// unless -domain is given explicitly, because issuing needs ports 80/443 to be
// reachable from the internet. See ../README.md and ../REPORT.md.
package main

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

const (
	stagingDirectory = "https://acme-staging-v02.api.letsencrypt.org/directory"
)

func main() {
	domain := flag.String("domain", "", "DNS name to get a certificate for, e.g. 203-0-113-5.sslip.io (required; nothing starts without it)")
	email := flag.String("email", "", "contact e-mail for the ACME account (optional)")
	cache := flag.String("cache", "", "directory for the certificate cache (required: without a cache every start re-issues and burns the rate limit)")
	httpAddr := flag.String("http", ":80", "address for the HTTP-01 challenge and the redirect to HTTPS")
	httpsAddr := flag.String("https", ":443", "address for HTTPS (also answers TLS-ALPN-01)")
	staging := flag.Bool("staging", false, "use the Let's Encrypt staging CA (certificates are not trusted by browsers, but limits are generous)")
	flag.Parse()

	if *domain == "" || *cache == "" {
		fmt.Fprintln(os.Stderr, "refusing to start: -domain and -cache are required (this prototype contacts Let's Encrypt)")
		flag.Usage()
		os.Exit(2)
	}
	d := strings.ToLower(strings.TrimSpace(*domain))

	m := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(d), // only our own name: no issuing for arbitrary SNI
		Cache:      autocert.DirCache(*cache),
		Email:      *email,
	}
	if *staging {
		m.Client = &acme.Client{DirectoryURL: stagingDirectory}
		log.Printf("using Let's Encrypt STAGING: browsers will not trust the certificate")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "ok, served over HTTPS for %s\n", r.Host)
	})
	// Same shape as media/ /headers, so the page can be checked through a proxy.
	mux.HandleFunc("/headers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"remoteAddr": r.RemoteAddr, "host": r.Host, "tls": r.TLS != nil, "headers": r.Header})
	})

	// Port 80: HTTP-01 challenge responses; everything else redirects to HTTPS.
	go func() {
		log.Printf("HTTP-01 + redirect on %s", *httpAddr)
		log.Fatal((&http.Server{Addr: *httpAddr, Handler: m.HTTPHandler(nil), ReadHeaderTimeout: 10 * time.Second}).ListenAndServe())
	}()

	// Log the first issue so the time to a certificate is visible.
	start := time.Now()
	tc := m.TLSConfig() // advertises h2, http/1.1 and acme-tls/1 (TLS-ALPN-01)
	get := tc.GetCertificate
	tc.GetCertificate = func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		c, err := get(hello)
		if err != nil {
			log.Printf("certificate for %q (alpn %v): error: %v", hello.ServerName, hello.SupportedProtos, err)
		} else if len(hello.SupportedProtos) == 1 && hello.SupportedProtos[0] == acme.ALPNProto {
			log.Printf("TLS-ALPN-01 challenge answered for %q after %s", hello.ServerName, time.Since(start).Round(time.Millisecond))
		}
		return c, err
	}
	srv := &http.Server{Addr: *httpsAddr, Handler: mux, TLSConfig: tc, ReadHeaderTimeout: 10 * time.Second}
	log.Printf("serving %s on %s: the first HTTPS request triggers issuing, allow up to a minute", d, *httpsAddr)
	log.Fatal(srv.ListenAndServeTLS("", ""))
}
