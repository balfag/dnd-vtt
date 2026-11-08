// Package av is the embedded SFU: voice rooms on pion/webrtc with simulcast
// forwarding. It knows nothing about WebSocket; signalling is wired by the caller.
package av

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"

	"github.com/pion/interceptor"
	"github.com/pion/webrtc/v4"
)

// Config of the media transport.
type Config struct {
	// MediaPort is the UDP and TCP port for ICE (UDP mux and ICE-TCP passive
	// mux on the same number). 0 picks a free UDP port and uses the same
	// number for TCP (tests).
	MediaPort int
}

// Engine owns the pion API and the media sockets; one per server.
type Engine struct {
	api    *webrtc.API
	logger *slog.Logger
	port   int
	udpMux io.Closer
	tcpMux io.Closer

	mu     sync.Mutex
	rooms  map[*Room]struct{}
	closed bool
}

// NewEngine listens on 0.0.0.0:<MediaPort> for UDP ("udp4") and TCP ("tcp4")
// and builds the pion API: codecs VP8 (PT 96, feedback goog-remb, ccm fir,
// nack, nack pli) and Opus (PT 111, 48000/2, "minptime=10;useinbandfec=1"),
// webrtc.RegisterDefaultInterceptors, SettingEngine with SetICEUDPMux,
// SetICETCPMux (webrtc.NewICETCPMux(nil, tcpListener, 8)), network types
// UDP4 and TCP4, SetIncludeLoopbackCandidate(true).
func NewEngine(cfg Config, logger *slog.Logger) (*Engine, error) {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	m := &webrtc.MediaEngine{}
	err := m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType: webrtc.MimeTypeVP8, ClockRate: 90000,
			RTCPFeedback: []webrtc.RTCPFeedback{{Type: "goog-remb"}, {Type: "ccm", Parameter: "fir"}, {Type: "nack"}, {Type: "nack", Parameter: "pli"}},
		},
		PayloadType: 96,
	}, webrtc.RTPCodecTypeVideo)
	if err != nil {
		return nil, fmt.Errorf("av: register vp8: %w", err)
	}
	err = m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2, SDPFmtpLine: "minptime=10;useinbandfec=1",
		},
		PayloadType: 111,
	}, webrtc.RTPCodecTypeAudio)
	if err != nil {
		return nil, fmt.Errorf("av: register opus: %w", err)
	}
	// NACK, RTCP reports, TWCC and the simulcast header extensions (mid/rid).
	ir := &interceptor.Registry{}
	if err := webrtc.RegisterDefaultInterceptors(m, ir); err != nil {
		return nil, fmt.Errorf("av: interceptors: %w", err)
	}

	udp, tcp, err := listenBoth(cfg.MediaPort)
	if err != nil {
		return nil, err
	}
	port := udp.LocalAddr().(*net.UDPAddr).Port
	udpMux := webrtc.NewICEUDPMux(nil, udp)
	tcpMux := webrtc.NewICETCPMux(nil, tcp, 8)

	se := webrtc.SettingEngine{}
	se.SetICEUDPMux(udpMux)
	se.SetICETCPMux(tcpMux)
	se.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4, webrtc.NetworkTypeTCP4})
	se.SetIncludeLoopbackCandidate(true)

	e := &Engine{
		api:    webrtc.NewAPI(webrtc.WithMediaEngine(m), webrtc.WithInterceptorRegistry(ir), webrtc.WithSettingEngine(se)),
		logger: logger,
		port:   port,
		udpMux: udpMux,
		tcpMux: tcpMux,
		rooms:  map[*Room]struct{}{},
	}
	logger.Info("av: media transport listening", "port", port)
	return e, nil
}

// Port returns the media port actually listened on.
func (e *Engine) Port() int { return e.port }

// Close closes all rooms made by this engine and the media sockets.
func (e *Engine) Close() error {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return nil
	}
	e.closed = true
	rooms := make([]*Room, 0, len(e.rooms))
	for r := range e.rooms {
		rooms = append(rooms, r)
	}
	e.rooms = map[*Room]struct{}{}
	e.mu.Unlock()

	for _, r := range rooms {
		r.Close()
	}
	return errors.Join(e.udpMux.Close(), e.tcpMux.Close())
}

// NewRoom makes an empty room (one per game session).
func (e *Engine) NewRoom() *Room {
	r := &Room{e: e, peers: map[string]*Peer{}}
	e.mu.Lock()
	if e.closed {
		r.closed = true
	} else {
		e.rooms[r] = struct{}{}
	}
	e.mu.Unlock()
	return r
}

// listenBoth binds UDP and then TCP on the same port number. With port 0 the
// UDP port is picked by the system and the whole step is retried when that
// number is taken for TCP.
func listenBoth(port int) (*net.UDPConn, *net.TCPListener, error) {
	attempts := 1
	if port == 0 {
		attempts = 20
	}
	var lastErr error
	for range attempts {
		udp, err := net.ListenUDP("udp4", &net.UDPAddr{Port: port})
		if err != nil {
			return nil, nil, fmt.Errorf("av: listen udp: %w", err)
		}
		// TCP is bound after UDP so that port 0 gives the same number for both.
		tcp, err := net.ListenTCP("tcp4", &net.TCPAddr{Port: udp.LocalAddr().(*net.UDPAddr).Port})
		if err == nil {
			return udp, tcp, nil
		}
		_ = udp.Close()
		lastErr = fmt.Errorf("av: listen tcp: %w", err)
	}
	return nil, nil, lastErr
}
