// Command sfu is a throw-away prototype of an embedded pion SFU room with
// simulcast (spike #7). See README.md and REPORT.md.
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
	"log"
	"math/big"
	"net"
	"net/http"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/webrtc/v4"
)

//go:embed index.html
var indexHTML []byte

func main() {
	httpAddr := flag.String("http", ":7777", "HTTP(S) listen address (page + /ws signalling)")
	udpPort := flag.Int("udp-port", 7778, "single UDP port for all ICE/media traffic")
	useTLS := flag.Bool("tls", false, "serve HTTPS/WSS with an in-memory self-signed certificate (for getUserMedia on LAN devices)")
	rewrite := flag.Bool("rewrite", true, "rewrite seq/timestamp when switching simulcast layer")
	flag.Parse()

	api, err := newAPI(*udpPort)
	if err != nil {
		log.Fatal(err)
	}
	r := newRoom(api, *rewrite)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/" {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(indexHTML)
	})
	mux.HandleFunc("/ws", func(w http.ResponseWriter, req *http.Request) {
		ws, err := upgradeWS(w, req)
		if err != nil {
			return
		}
		r.handle(ws)
	})
	// Server-side RTP byte counters, used by the measurements.
	mux.HandleFunc("/stats", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]int64{"bytesIn": r.bytesIn.Load(), "bytesOut": r.bytesOut.Load()})
	})

	srv := &http.Server{Addr: *httpAddr, Handler: mux}
	if *useTLS {
		cert, ips, err := selfSignedCert()
		if err != nil {
			log.Fatal(err)
		}
		srv.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
		// WebSocket upgrade needs HTTP/1.1.
		srv.TLSNextProto = map[string]func(*http.Server, *tls.Conn, http.Handler){}
		for _, ip := range ips {
			log.Printf("open https://%s%s", ip, *httpAddr)
		}
		log.Fatal(srv.ListenAndServeTLS("", ""))
	}
	log.Printf("open http://localhost%s", *httpAddr)
	log.Fatal(srv.ListenAndServe())
}

func newAPI(udpPort int) (*webrtc.API, error) {
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
	err = m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2, SDPFmtpLine: "minptime=10;useinbandfec=1",
		},
		PayloadType: 111,
	}, webrtc.RTPCodecTypeAudio)
	if err != nil {
		return nil, err
	}
	// NACK, RTCP reports, TWCC and the simulcast header extensions (mid/rid).
	ir := &interceptor.Registry{}
	if err := webrtc.RegisterDefaultInterceptors(m, ir); err != nil {
		return nil, err
	}

	udp, err := net.ListenUDP("udp4", &net.UDPAddr{Port: udpPort})
	if err != nil {
		return nil, err
	}
	se := webrtc.SettingEngine{}
	se.SetICEUDPMux(webrtc.NewICEUDPMux(nil, udp))
	se.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4})
	return webrtc.NewAPI(webrtc.WithMediaEngine(m), webrtc.WithInterceptorRegistry(ir), webrtc.WithSettingEngine(se)), nil
}

// selfSignedCert makes an in-memory ECDSA certificate valid for localhost,
// 127.0.0.1 and every IPv4 address of this machine.
func selfSignedCert() (tls.Certificate, []string, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "dnd-vtt sfu spike"},
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
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, hosts, nil
}
