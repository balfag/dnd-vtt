package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
)

type msg struct {
	Type string  `json:"type"`
	SDP  string  `json:"sdp,omitempty"`
	T    float64 `json:"t,omitempty"`
}

// handle serves one signalling connection: the client sends one offer (after
// ICE gathering, no trickle) and gets the answer back; "ping" is answered
// with "pong" so the page can measure the WebSocket round trip.
func (s *server) handle(ws *wsConn) {
	defer ws.close()
	var pc *webrtc.PeerConnection
	defer func() {
		if pc != nil {
			_ = pc.Close()
		}
	}()
	for {
		raw, err := ws.readMessage()
		if err != nil {
			return
		}
		var m msg
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		switch m.Type {
		case "ping":
			b, _ := json.Marshal(msg{Type: "pong", T: m.T})
			_ = ws.writeText(b)
		case "offer":
			if pc != nil {
				continue
			}
			var answer string
			pc, answer, err = s.answer(m.SDP)
			if err != nil {
				log.Printf("offer: %v", err)
				b, _ := json.Marshal(msg{Type: "error", SDP: err.Error()})
				_ = ws.writeText(b)
				return
			}
			b, _ := json.Marshal(msg{Type: "answer", SDP: answer})
			_ = ws.writeText(b)
		}
	}
}

// answer creates the echo peer connection and returns the final answer SDP
// (with all candidates, ICE gathering complete).
func (s *server) answer(offerSDP string) (*webrtc.PeerConnection, string, error) {
	pc, err := s.api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		return nil, "", err
	}
	out, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8, ClockRate: 90000}, "echo", "echo")
	if err != nil {
		_ = pc.Close()
		return nil, "", err
	}
	sender, err := pc.AddTrack(out)
	if err != nil {
		_ = pc.Close()
		return nil, "", err
	}
	go func() { // drain RTCP so the interceptors (NACK, reports) keep working
		buf := make([]byte, 1500)
		for {
			if _, _, err := sender.Read(buf); err != nil {
				return
			}
		}
	}()

	pc.OnICEConnectionStateChange(func(st webrtc.ICEConnectionState) {
		log.Printf("ice state: %s", st)
	})
	pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		log.Printf("track: %s ssrc=%d", track.Codec().MimeType, track.SSRC())
		go func() { // keep keyframes coming so a late joiner decodes quickly
			t := time.NewTicker(2 * time.Second)
			defer t.Stop()
			for range t.C {
				if pc.ConnectionState() == webrtc.PeerConnectionStateClosed {
					return
				}
				if pc.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: uint32(track.SSRC())}}) != nil {
					return
				}
			}
		}()
		for {
			pkt, _, err := track.ReadRTP()
			if err != nil {
				return
			}
			s.bytesIn.Add(int64(len(pkt.Payload) + 12))
			if err := out.WriteRTP(pkt); err != nil {
				return
			}
			s.bytesOut.Add(int64(len(pkt.Payload) + 12))
		}
	})

	if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: offerSDP}); err != nil {
		_ = pc.Close()
		return nil, "", err
	}
	ans, err := pc.CreateAnswer(nil)
	if err != nil {
		_ = pc.Close()
		return nil, "", err
	}
	gathered := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(ans); err != nil {
		_ = pc.Close()
		return nil, "", err
	}
	<-gathered
	sdp := pc.LocalDescription().SDP
	if s.advertise != "" {
		sdp, err = addAdvertised(sdp, s.mode, s.advertise)
		if err != nil {
			_ = pc.Close()
			return nil, "", err
		}
	}
	for _, l := range strings.Split(sdp, "\r\n") {
		if strings.HasPrefix(l, "a=candidate:") {
			log.Printf("answer %s", l)
		}
	}
	return pc, sdp, nil
}

// addAdvertised appends, after the first host candidate of each component, a copy that points at hostPort (a tunnel or forwarded port whose number differs
// from the local one). pion can only rewrite the IP, not the port, so the answer
// is edited as text. A host name is resolved here because browsers are not
// guaranteed to resolve names in remote candidates.
func addAdvertised(sdp, mode, hostPort string) (string, error) {
	host, port, err := net.SplitHostPort(hostPort)
	if err != nil {
		return "", fmt.Errorf("-advertise: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		ips, err := net.LookupIP(host)
		if err != nil || len(ips) == 0 {
			return "", fmt.Errorf("-advertise: cannot resolve %q: %v", host, err)
		}
		for _, c := range ips {
			if c.To4() != nil {
				ip = c
				break
			}
		}
		if ip == nil {
			return "", fmt.Errorf("-advertise: %q has no IPv4 address", host)
		}
	}
	proto := "udp"
	if mode == "tcp" {
		proto = "tcp"
	}
	var b strings.Builder
	done := map[string]bool{} // one advertised copy per component
	for _, line := range strings.Split(strings.TrimSuffix(sdp, "\r\n"), "\r\n") {
		b.WriteString(line)
		b.WriteString("\r\n")
		f := strings.Fields(line)
		// a=candidate:<foundation> <component> <proto> <priority> <ip> <port> typ host ...
		if len(f) < 8 || !strings.HasPrefix(f[0], "a=candidate:") || !strings.EqualFold(f[2], proto) || f[7] != "host" {
			continue
		}
		if done[f[1]] {
			continue
		}
		done[f[1]] = true
		f[0] = "a=candidate:adv" + f[1]
		f[4], f[5] = ip.String(), port
		b.WriteString(strings.Join(f, " "))
		b.WriteString("\r\n")
	}
	return b.String(), nil
}
