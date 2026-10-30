// Command media is a throw-away echo server for the connectivity spike (#8):
// a browser sends a video stream, the server sends it back. The ICE transport
// is selected by -mode (udp, tcp, turn-tcp). See README.md and ../REPORT.md.
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/turn/v4"
	"github.com/pion/webrtc/v4"
)

//go:embed index.html
var indexHTML []byte

const (
	turnUser  = "spike"
	turnPass  = "spike-pass"
	turnRealm = "dnd-vtt-spike"
)

type server struct {
	mode      string
	mediaPort int
	api       *webrtc.API
	advertise string // host:port to add to the answer candidates (tunnels)
	bytesIn   atomic.Int64
	bytesOut  atomic.Int64
}

func main() {
	httpAddr := flag.String("http", ":17777", "HTTP(S) listen address (page, /ws signalling, /headers)")
	mode := flag.String("mode", "udp", "media transport: udp | tcp | turn-tcp")
	port := flag.Int("media-port", 17778, "port for ICE UDP mux (udp), ICE-TCP (tcp) or TURN/TCP (turn-tcp)")
	publicIP := flag.String("public-ip", "", "external IPv4 used as the TURN relay address in turn-tcp mode (udp and tcp modes use -advertise)")
	advertise := flag.String("advertise", "", "external host:port (forwarded port or tunnel) to add as an extra candidate to the answer, udp and tcp modes")
	useTLS := flag.Bool("tls", false, "serve HTTPS/WSS with an in-memory self-signed certificate (camera on non-localhost origins)")
	tlsHost := flag.String("tls-host", "", "extra IP or DNS name for the self-signed certificate SAN (external address)")
	flag.Parse()

	s := &server{mode: *mode, mediaPort: *port, advertise: *advertise}
	var err error
	if s.api, err = s.newAPI(*publicIP); err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(indexHTML)
	})
	mux.HandleFunc("/config", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"mode": s.mode, "turnPort": s.mediaPort, "turnUser": turnUser, "turnPass": turnPass,
		})
	})
	mux.HandleFunc("/headers", headersHandler)
	mux.HandleFunc("/stats", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]int64{"bytesIn": s.bytesIn.Load(), "bytesOut": s.bytesOut.Load()})
	})
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgradeWS(w, r)
		if err != nil {
			return
		}
		s.handle(ws)
	})

	srv := &http.Server{Addr: *httpAddr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	log.Printf("mode=%s media-port=%d", s.mode, s.mediaPort)
	if *useTLS {
		cert, hosts, err := selfSignedCert(*tlsHost)
		if err != nil {
			log.Fatal(err)
		}
		srv.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
		// WebSocket upgrade needs HTTP/1.1.
		srv.TLSNextProto = map[string]func(*http.Server, *tls.Conn, http.Handler){}
		_, p, _ := net.SplitHostPort(*httpAddr)
		for _, h := range hosts {
			log.Printf("open https://%s:%s/", h, p)
		}
		log.Fatal(srv.ListenAndServeTLS("", ""))
	}
	_, p, _ := net.SplitHostPort(*httpAddr)
	log.Printf("open http://localhost:%s/", p)
	log.Fatal(srv.ListenAndServe())
}

// newAPI builds the pion API for the selected mode.
func (s *server) newAPI(publicIP string) (*webrtc.API, error) {
	m := &webrtc.MediaEngine{}
	err := m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType: webrtc.MimeTypeVP8, ClockRate: 90000,
			RTCPFeedback: []webrtc.RTCPFeedback{{Type: "goog-remb"}, {Type: "ccm", Parameter: "fir"}, {Type: "nack"}, {Type: "nack", Parameter: "pli"}},
		},
		PayloadType: 96,
	}, webrtc.RTPCodecTypeVideo)
	if err != nil {
		return nil, err
	}
	ir := &interceptor.Registry{}
	if err := webrtc.RegisterDefaultInterceptors(m, ir); err != nil {
		return nil, err
	}

	se := webrtc.SettingEngine{}
	switch s.mode {
	case "udp":
		udp, err := net.ListenUDP("udp4", &net.UDPAddr{Port: s.mediaPort})
		if err != nil {
			return nil, err
		}
		se.SetICEUDPMux(webrtc.NewICEUDPMux(nil, udp))
		se.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4})
	case "tcp":
		ln, err := net.ListenTCP("tcp4", &net.TCPAddr{Port: s.mediaPort})
		if err != nil {
			return nil, err
		}
		// Only passive TCP candidates; no UDP sockets are opened at all.
		se.SetICETCPMux(webrtc.NewICETCPMux(nil, ln, 32))
		se.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeTCP4})
	case "turn-tcp":
		// The browser reaches the server only through the embedded TURN server
		// (TCP); the server's own peer connection talks to the relay over UDP.
		relayIP := net.ParseIP("127.0.0.1")
		if publicIP != "" {
			relayIP = net.ParseIP(publicIP)
		}
		ln, err := net.ListenTCP("tcp4", &net.TCPAddr{Port: s.mediaPort})
		if err != nil {
			return nil, err
		}
		_, err = turn.NewServer(turn.ServerConfig{
			Realm: turnRealm,
			AuthHandler: func(username, realm string, _ net.Addr) ([]byte, bool) {
				if username != turnUser {
					return nil, false
				}
				return turn.GenerateAuthKey(username, realm, turnPass), true
			},
			ListenerConfigs: []turn.ListenerConfig{{
				Listener: ln,
				RelayAddressGenerator: &turn.RelayAddressGeneratorPortRange{
					RelayAddress: relayIP, Address: "0.0.0.0",
					MinPort: uint16(s.mediaPort + 1), MaxPort: uint16(s.mediaPort + 50),
				},
			}},
		})
		if err != nil {
			return nil, err
		}
		udp, err := net.ListenUDP("udp4", &net.UDPAddr{Port: s.mediaPort})
		if err != nil {
			return nil, err
		}
		se.SetICEUDPMux(webrtc.NewICEUDPMux(nil, udp))
		se.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4})
		se.SetIncludeLoopbackCandidate(true)
	default:
		return nil, fmt.Errorf("unknown -mode %q (want udp, tcp or turn-tcp)", s.mode)
	}
	return webrtc.NewAPI(webrtc.WithMediaEngine(m), webrtc.WithInterceptorRegistry(ir), webrtc.WithSettingEngine(se)), nil
}

// headersHandler echoes what the server sees: the peer address, the Host and
// all request headers. Used to learn which headers a tunnel adds and from
// which address it connects (security.md: isLoopback).
func headersHandler(w http.ResponseWriter, r *http.Request) {
	h := map[string][]string{}
	for k, v := range r.Header {
		h[k] = v
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	_, hasXFF := r.Header["X-Forwarded-For"]
	_, hasFwd := r.Header["Forwarded"]
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(map[string]any{
		"remoteAddr": r.RemoteAddr,
		"host":       r.Host,
		"tls":        r.TLS != nil,
		"proto":      r.Proto,
		"headers":    h,
		// What the current isLoopback (internal/httpapi/respond.go) would decide.
		"currentIsLoopback": net.ParseIP(host).IsLoopback() && !hasXFF && !hasFwd,
	})
}

// selfSignedCert makes an in-memory ECDSA certificate valid for localhost,
// 127.0.0.1, every IPv4 address of this machine and the optional extra host.
func selfSignedCert(extra string) (tls.Certificate, []string, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "dnd-vtt connectivity spike"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(7 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	hosts := []string{"localhost"}
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil && !ipn.IP.IsLoopback() {
			tpl.IPAddresses = append(tpl.IPAddresses, ipn.IP)
			hosts = append(hosts, ipn.IP.String())
		}
	}
	if extra = strings.TrimSpace(extra); extra != "" {
		if ip := net.ParseIP(extra); ip != nil {
			tpl.IPAddresses = append(tpl.IPAddresses, ip)
		} else {
			tpl.DNSNames = append(tpl.DNSNames, extra)
		}
		hosts = append(hosts, extra)
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, hosts, nil
}
