package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
)

// msg is the signalling envelope, both directions. See README.md.
type msg struct {
	Type      string                   `json:"type"`
	Name      string                   `json:"name,omitempty"`
	ID        string                   `json:"id,omitempty"`
	SDP       string                   `json:"sdp,omitempty"`
	Candidate *webrtc.ICECandidateInit `json:"candidate,omitempty"`
	Peer      string                   `json:"peer,omitempty"`
	Layer     string                   `json:"layer,omitempty"`
	Peers     []peerInfo               `json:"peers,omitempty"`
	Error     string                   `json:"error,omitempty"`
}

type peerInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type room struct {
	api     *webrtc.API
	rewrite bool

	mu    sync.Mutex
	peers map[string]*peer

	// Payload+12 bytes of RTP counted on the way in and out of the SFU.
	bytesIn, bytesOut atomic.Int64
}

func newRoom(api *webrtc.API, rewrite bool) *room {
	return &room{api: api, rewrite: rewrite, peers: map[string]*peer{}}
}

type peer struct {
	id, name string
	room     *room
	ws       *wsConn
	pc       *webrtc.PeerConnection

	negMu    sync.Mutex
	negBusy  bool
	negDirty bool

	mu     sync.RWMutex
	ready  bool
	closed bool
	video  map[string]*webrtc.TrackRemote // rid -> published video layer
	subs   map[string]*sub                // receiver id -> subscription to my media
	recv   map[string]*sub                // sender id -> subscription I receive
	lastKF map[string]time.Time
}

func newID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func (p *peer) send(m *msg) {
	b, err := json.Marshal(m)
	if err != nil {
		return
	}
	if err := p.ws.writeText(b); err != nil {
		log.Printf("peer %s: send %s: %v", p.id, m.Type, err)
	}
}

// requestKeyframe sends a PLI to the sender for one simulcast layer, at most
// every 300 ms per layer.
func (p *peer) requestKeyframe(rid string) {
	p.mu.Lock()
	tr := p.video[rid]
	if tr == nil || time.Since(p.lastKF[rid]) < 300*time.Millisecond {
		p.mu.Unlock()
		return
	}
	p.lastKF[rid] = time.Now()
	p.mu.Unlock()
	_ = p.pc.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: uint32(tr.SSRC())}})
}

// negotiate sends a server offer; offers never overlap (server always
// initiates after the first client offer, so there is no glare).
func (p *peer) negotiate() {
	p.negMu.Lock()
	if p.negBusy {
		p.negDirty = true
		p.negMu.Unlock()
		return
	}
	p.negBusy = true
	p.negMu.Unlock()
	go func() {
		offer, err := p.pc.CreateOffer(nil)
		if err == nil {
			err = p.pc.SetLocalDescription(offer)
		}
		if err != nil {
			log.Printf("peer %s: create offer: %v", p.id, err)
			p.negDone()
			return
		}
		p.send(&msg{Type: "offer", SDP: offer.SDP})
	}()
}

func (p *peer) negDone() {
	p.negMu.Lock()
	p.negBusy = false
	again := p.negDirty
	p.negDirty = false
	p.negMu.Unlock()
	if again {
		p.negotiate()
	}
}

func (r *room) handle(ws *wsConn) {
	defer ws.close()
	raw, err := ws.readMessage()
	if err != nil {
		return
	}
	var first msg
	if json.Unmarshal(raw, &first) != nil || first.Type != "join" {
		return
	}
	pc, err := r.api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		log.Printf("new peer connection: %v", err)
		return
	}
	p := &peer{
		id: newID(), name: first.Name, room: r, ws: ws, pc: pc,
		video: map[string]*webrtc.TrackRemote{}, subs: map[string]*sub{},
		recv: map[string]*sub{}, lastKF: map[string]time.Time{},
	}
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c != nil {
			ci := c.ToJSON()
			p.send(&msg{Type: "candidate", Candidate: &ci})
		}
	})
	pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		log.Printf("peer %s (%s): connection %s", p.id, p.name, s)
		if s == webrtc.PeerConnectionStateFailed || s == webrtc.PeerConnectionStateClosed {
			r.remove(p)
		}
	})
	pc.OnTrack(p.onTrack)

	r.mu.Lock()
	var others []peerInfo
	for _, o := range r.peers {
		others = append(others, peerInfo{ID: o.id, Name: o.name})
	}
	r.peers[p.id] = p
	r.mu.Unlock()
	log.Printf("peer %s (%s) joined, %d in room", p.id, p.name, len(others)+1)

	p.send(&msg{Type: "joined", ID: p.id, Peers: others})
	r.broadcast(p, &msg{Type: "peer-joined", ID: p.id, Name: p.name})
	defer r.remove(p)

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
		case "offer":
			p.onClientOffer(m.SDP)
		case "answer":
			if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: m.SDP}); err != nil {
				log.Printf("peer %s: set answer: %v", p.id, err)
			}
			p.negDone()
		case "candidate":
			if m.Candidate != nil {
				if err := pc.AddICECandidate(*m.Candidate); err != nil {
					log.Printf("peer %s: add candidate: %v", p.id, err)
				}
			}
		case "layer":
			p.mu.RLock()
			s := p.recv[m.Peer]
			p.mu.RUnlock()
			if s != nil && validLayer(m.Layer) {
				s.setWant(m.Layer)
			}
		case "leave":
			return
		}
	}
}

// onClientOffer answers the client's first offer (its publishing side) and
// then attaches the other participants' streams with a server offer.
func (p *peer) onClientOffer(sdp string) {
	if err := p.pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: sdp}); err != nil {
		log.Printf("peer %s: set offer: %v", p.id, err)
		return
	}
	answer, err := p.pc.CreateAnswer(nil)
	if err == nil {
		err = p.pc.SetLocalDescription(answer)
	}
	if err != nil {
		log.Printf("peer %s: create answer: %v", p.id, err)
		return
	}
	p.send(&msg{Type: "answer", SDP: answer.SDP})
	p.room.attach(p)
}

func (p *peer) onTrack(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
	if track.Kind() == webrtc.RTPCodecTypeAudio {
		for {
			pkt, _, err := track.ReadRTP()
			if err != nil {
				return
			}
			p.room.bytesIn.Add(int64(len(pkt.Payload) + 12))
			p.mu.RLock()
			for _, s := range p.subs {
				s.forwardAudio(pkt)
			}
			p.mu.RUnlock()
		}
	}
	rid := track.RID()
	if rid == "" { // sender without simulcast: treat as the middle layer
		rid = layerMid
	}
	log.Printf("peer %s: video layer %q ssrc %d", p.id, rid, track.SSRC())
	p.mu.Lock()
	p.video[rid] = track
	p.mu.Unlock()
	for {
		pkt, _, err := track.ReadRTP()
		if err != nil {
			return
		}
		p.room.bytesIn.Add(int64(len(pkt.Payload) + 12))
		p.mu.RLock()
		for _, s := range p.subs {
			s.forwardVideo(rid, pkt, p.room.rewrite)
		}
		p.mu.RUnlock()
	}
}

var (
	vp8Cap  = webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8, ClockRate: 90000}
	opusCap = webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2}
)

// attach marks p ready and subscribes it to every other ready peer (and them
// to p). Called after p's first answer.
func (r *room) attach(p *peer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p.mu.Lock()
	p.ready = true
	p.mu.Unlock()
	for _, o := range r.peers {
		o.mu.RLock()
		ready := o.ready && !o.closed
		o.mu.RUnlock()
		if o == p || !ready {
			continue
		}
		for _, pair := range [][2]*peer{{o, p}, {p, o}} {
			if err := subscribe(pair[0], pair[1]); err != nil {
				log.Printf("subscribe %s->%s: %v", pair[0].id, pair[1].id, err)
			}
		}
	}
	p.negotiate()
}

// subscribe makes `to` receive the media of `from`.
func subscribe(from, to *peer) error {
	vt, err := webrtc.NewTrackLocalStaticRTP(vp8Cap, "video-"+from.id, from.id)
	if err != nil {
		return err
	}
	at, err := webrtc.NewTrackLocalStaticRTP(opusCap, "audio-"+from.id, from.id)
	if err != nil {
		return err
	}
	vs, err := to.pc.AddTrack(vt)
	if err != nil {
		return err
	}
	as, err := to.pc.AddTrack(at)
	if err != nil {
		return err
	}
	s := &sub{from: from, to: to, vtrack: vt, atrack: at, vsender: vs, asender: as, want: layerMid, done: make(chan struct{})}
	from.mu.Lock()
	from.subs[to.id] = s
	from.mu.Unlock()
	to.mu.Lock()
	to.recv[from.id] = s
	to.mu.Unlock()
	go s.readRTCP(vs)
	go s.readRTCP(as)
	go s.pendingLoop()
	from.requestKeyframe(layerMid)
	to.negotiate()
	return nil
}

// readRTCP relays PLI/FIR from the receiver to the sender (current layer).
// REMB is only counted: layer selection by bandwidth is out of scope here.
func (s *sub) readRTCP(sender *webrtc.RTPSender) {
	for {
		pkts, _, err := sender.ReadRTCP()
		if err != nil {
			return
		}
		for _, pk := range pkts {
			switch pk.(type) {
			case *rtcp.PictureLossIndication, *rtcp.FullIntraRequest:
				s.mu.Lock()
				l := s.cur
				if l == "" {
					l = s.want
				}
				s.mu.Unlock()
				s.from.requestKeyframe(l)
			}
		}
	}
}

func (r *room) broadcast(except *peer, m *msg) {
	r.mu.Lock()
	var list []*peer
	for _, o := range r.peers {
		if o != except {
			list = append(list, o)
		}
	}
	r.mu.Unlock()
	for _, o := range list {
		o.send(m)
	}
}

// remove drops p from the room: unsubscribes everybody from its tracks and
// renegotiates their connections.
func (r *room) remove(p *peer) {
	r.mu.Lock()
	if r.peers[p.id] != p {
		r.mu.Unlock()
		return
	}
	delete(r.peers, p.id)
	p.mu.Lock()
	p.closed = true
	mySubs := p.subs
	myRecv := p.recv
	p.mu.Unlock()
	for _, s := range mySubs {
		s.stop()
	}
	for _, s := range myRecv {
		s.stop()
	}
	for _, o := range r.peers {
		o.mu.Lock()
		s := o.recv[p.id]
		delete(o.recv, p.id)
		delete(o.subs, p.id)
		o.mu.Unlock()
		if s != nil {
			_ = o.pc.RemoveTrack(s.vsender)
			_ = o.pc.RemoveTrack(s.asender)
			o.negotiate()
		}
	}
	n := len(r.peers)
	r.mu.Unlock()
	_ = p.pc.Close()
	r.broadcast(p, &msg{Type: "peer-left", ID: p.id})
	log.Printf("peer %s (%s) left, %d in room", p.id, p.name, n)
}
