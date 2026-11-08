package av

import (
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
)

// Signal is a message the SFU sends to one participant's client.
type Signal struct {
	Type      string                   // "offer" or "ice"
	SDP       string                   // for "offer"; the answer to the client's offer is returned by HandleOffer
	Candidate *webrtc.ICECandidateInit // for "ice"; nil means end of candidates
}

// Signaler delivers a Signal to the client. It is called from pion and room
// goroutines, must be safe for concurrent use and must not block for long.
type Signaler func(Signal)

var (
	vp8Cap  = webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8, ClockRate: 90000}
	opusCap = webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2}
)

// Room is one voice room.
type Room struct {
	e *Engine

	// Lock order: Room.mu, then Peer.mu.
	mu     sync.Mutex
	peers  map[string]*Peer
	closed bool

	// Payload+12 bytes of RTP counted on the way in and out of the SFU.
	bytesIn, bytesOut atomic.Int64
}

// Peer is one participant's connection to the room.
type Peer struct {
	id       string
	room     *Room
	send     Signaler
	onFailed func()
	pc       *webrtc.PeerConnection
	log      *slog.Logger

	audioBlocked atomic.Bool
	failedOnce   sync.Once

	// opMu serialises the client-facing calls against each other and against
	// closing the connection.
	opMu sync.Mutex

	negMu        sync.Mutex
	negBusy      bool
	negDirty     bool
	pendingOffer bool

	mu       sync.RWMutex
	gotOffer bool // HandleOffer succeeded
	ready    bool // the peer takes part in subscriptions
	closed   bool
	video    map[Layer]*webrtc.TrackRemote // published video layers
	subs     map[string]*sub               // receiver id -> subscription to my media
	recv     map[string]*sub               // sender id -> subscription I receive
	lastKF   map[Layer]time.Time
}

// Join adds a participant. id is the playerId; it is the msid stream id of
// every track the SFU sends to others for this participant (track ids
// "audio" and "video"). onFailed is called once, in its own goroutine, if the
// participant's PeerConnection reaches failed or closed state other than
// through Leave or Close. Errors: ErrAlreadyJoined, ErrRoomClosed.
func (r *Room) Join(id string, send Signaler, onFailed func()) (*Peer, error) {
	r.mu.Lock()
	err := r.canJoinLocked(id)
	r.mu.Unlock()
	if err != nil {
		return nil, err
	}
	pc, err := r.e.api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		return nil, fmt.Errorf("av: new peer connection: %w", err)
	}
	p := &Peer{
		id: id, room: r, send: send, onFailed: onFailed, pc: pc,
		log:   r.e.logger.With("peer", id),
		video: map[Layer]*webrtc.TrackRemote{}, subs: map[string]*sub{},
		recv: map[string]*sub{}, lastKF: map[Layer]time.Time{},
	}
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			p.send(Signal{Type: "ice"})
			return
		}
		ci := c.ToJSON()
		p.log.Debug("av: local candidate", "candidate", ci.Candidate)
		p.send(Signal{Type: "ice", Candidate: &ci})
	})
	pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		p.log.Debug("av: connection state", "state", s.String())
		if s != webrtc.PeerConnectionStateFailed && s != webrtc.PeerConnectionStateClosed {
			return
		}
		p.mu.RLock()
		closed := p.closed
		p.mu.RUnlock()
		if closed || p.onFailed == nil {
			return
		}
		p.failedOnce.Do(func() { go p.onFailed() })
	})
	pc.OnTrack(p.onTrack)

	r.mu.Lock()
	if err := r.canJoinLocked(id); err != nil {
		r.mu.Unlock()
		_ = pc.Close()
		return nil, err
	}
	r.peers[id] = p
	n := len(r.peers)
	r.mu.Unlock()
	r.e.logger.Info("av: peer joined", "peer", id, "in_room", n)
	return p, nil
}

func (r *Room) canJoinLocked(id string) error {
	if r.closed {
		return ErrRoomClosed
	}
	if _, ok := r.peers[id]; ok {
		return ErrAlreadyJoined
	}
	return nil
}

// Leave removes the participant, closes its PeerConnection, removes its tracks
// from everybody else and renegotiates them. No-op for an unknown id.
func (r *Room) Leave(id string) {
	r.mu.Lock()
	p := r.peers[id]
	if p == nil {
		r.mu.Unlock()
		return
	}
	delete(r.peers, id)
	mySubs, myRecv := p.markClosed()
	for _, s := range mySubs {
		s.stop()
	}
	for _, s := range myRecv {
		s.stop()
	}
	for _, o := range r.peers {
		o.mu.Lock()
		s := o.recv[id]
		delete(o.recv, id)
		delete(o.subs, id)
		o.mu.Unlock()
		if s != nil {
			_ = s.vtr.Stop()
			_ = s.atr.Stop()
			o.negotiate()
		}
	}
	n := len(r.peers)
	r.mu.Unlock()
	p.shutdown()
	r.e.logger.Info("av: peer left", "peer", id, "in_room", n)
}

// Close removes every participant; Join afterwards returns ErrRoomClosed.
func (r *Room) Close() {
	r.mu.Lock()
	r.closed = true
	peers := make([]*Peer, 0, len(r.peers))
	for _, p := range r.peers {
		peers = append(peers, p)
		mySubs, myRecv := p.markClosed()
		for _, s := range mySubs {
			s.stop()
		}
		for _, s := range myRecv {
			s.stop()
		}
	}
	r.peers = map[string]*Peer{}
	r.mu.Unlock()
	for _, p := range peers {
		p.shutdown()
	}
	r.e.mu.Lock()
	delete(r.e.rooms, r)
	r.e.mu.Unlock()
}

// Len is the number of participants.
func (r *Room) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.peers)
}

// Traffic returns RTP bytes (payload + 12) received from clients and sent to
// clients since the room was made.
func (r *Room) Traffic() (in, out int64) {
	return r.bytesIn.Load(), r.bytesOut.Load()
}

// markClosed flags the peer as closed and returns its subscriptions.
func (p *Peer) markClosed() (subs, recv map[string]*sub) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	return p.subs, p.recv
}

// shutdown closes the connection once no client call is in flight.
func (p *Peer) shutdown() {
	p.opMu.Lock()
	defer p.opMu.Unlock()
	_ = p.pc.Close()
}

func (p *Peer) isClosed() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.closed
}

// HandleOffer applies the client's first (publishing) offer and returns the
// answer SDP; afterwards the peer is subscribed to everybody and everybody to
// it, and the SFU sends its own offers through Signaler. Second call:
// ErrBadState. Invalid SDP: the pion error, wrapped.
func (p *Peer) HandleOffer(sdp string) (answer string, err error) {
	p.opMu.Lock()
	defer p.opMu.Unlock()
	p.mu.RLock()
	closed, got := p.closed, p.gotOffer
	p.mu.RUnlock()
	if closed {
		return "", ErrRoomClosed
	}
	if got {
		return "", ErrBadState
	}
	p.log.Debug("av: client offer", "sdp", sdp)
	if err := p.pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: sdp}); err != nil {
		return "", fmt.Errorf("av: set offer: %w", err)
	}
	a, err := p.pc.CreateAnswer(nil)
	if err == nil {
		err = p.pc.SetLocalDescription(a)
	}
	if err != nil {
		return "", fmt.Errorf("av: create answer: %w", err)
	}
	p.mu.Lock()
	p.gotOffer = true
	p.mu.Unlock()
	p.log.Debug("av: answer", "sdp", a.SDP)
	p.room.attach(p)
	return a.SDP, nil
}

// HandleAnswer applies the client's answer to the last SFU offer.
// No pending SFU offer: ErrBadState.
func (p *Peer) HandleAnswer(sdp string) error {
	p.opMu.Lock()
	defer p.opMu.Unlock()
	if p.isClosed() {
		return ErrRoomClosed
	}
	p.negMu.Lock()
	pending := p.pendingOffer
	p.pendingOffer = false
	p.negMu.Unlock()
	if !pending {
		return ErrBadState
	}
	p.log.Debug("av: client answer", "sdp", sdp)
	err := p.pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: sdp})
	p.negDone()
	if err != nil {
		return fmt.Errorf("av: set answer: %w", err)
	}
	return nil
}

// AddICE adds a remote candidate; an empty Candidate string is the end of
// candidates and is ignored.
func (p *Peer) AddICE(c webrtc.ICECandidateInit) error {
	if c.Candidate == "" {
		return nil
	}
	p.opMu.Lock()
	defer p.opMu.Unlock()
	if p.isClosed() {
		return ErrRoomClosed
	}
	p.log.Debug("av: remote candidate", "candidate", c.Candidate)
	if err := p.pc.AddICECandidate(c); err != nil {
		return fmt.Errorf("av: add candidate: %w", err)
	}
	return nil
}

// SetView sets the wanted layer per sender id; a sender missing from the map
// gets LayerLow. Unknown ids are ignored.
func (p *Peer) SetView(layers map[string]Layer) {
	p.mu.RLock()
	list := make(map[string]*sub, len(p.recv))
	for id, s := range p.recv {
		list[id] = s
	}
	p.mu.RUnlock()
	for id, s := range list {
		l, ok := layers[id]
		if !ok || !validLayer(l) {
			l = LayerLow
		}
		s.setWant(l)
	}
}

// SetAudioBlocked stops (true) or resumes (false) forwarding this peer's
// audio to everybody.
func (p *Peer) SetAudioBlocked(blocked bool) { p.audioBlocked.Store(blocked) }

// requestKeyframe sends a PLI to the sender for one simulcast layer, at most
// every 300 ms per layer.
func (p *Peer) requestKeyframe(rid Layer) {
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

// negotiate sends a server offer. Offers never overlap: while one waits for
// its answer a new request only marks the peer dirty. It does nothing before
// the client's first offer (the server never initiates earlier, so there is no
// glare) and after the peer was closed.
func (p *Peer) negotiate() {
	p.mu.RLock()
	skip := !p.ready || p.closed
	p.mu.RUnlock()
	if skip {
		return
	}
	p.negMu.Lock()
	if p.negBusy {
		p.negDirty = true
		p.negMu.Unlock()
		return
	}
	p.negBusy = true
	p.negMu.Unlock()
	// Not in the caller's goroutine: it may be a pion callback.
	go func() {
		if p.isClosed() {
			return
		}
		offer, err := p.pc.CreateOffer(nil)
		if err == nil {
			err = p.pc.SetLocalDescription(offer)
		}
		if err != nil {
			p.log.Debug("av: create offer", "err", err)
			p.negDone()
			return
		}
		p.negMu.Lock()
		p.pendingOffer = true
		p.negMu.Unlock()
		p.log.Debug("av: server offer", "sdp", offer.SDP)
		p.send(Signal{Type: "offer", SDP: offer.SDP})
	}()
}

func (p *Peer) negDone() {
	p.negMu.Lock()
	p.negBusy = false
	again := p.negDirty
	p.negDirty = false
	p.negMu.Unlock()
	if again {
		p.negotiate()
	}
}

func (p *Peer) onTrack(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
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
	rid := Layer(track.RID())
	if rid == "" { // sender without simulcast: treat as the middle layer
		rid = LayerMid
	}
	p.log.Debug("av: video layer", "rid", string(rid), "ssrc", uint32(track.SSRC()))
	p.mu.Lock()
	p.video[rid] = track
	var mask uint8
	for l := range p.video {
		mask |= layerBit(l)
	}
	subs := make([]*sub, 0, len(p.subs))
	for _, s := range p.subs {
		subs = append(subs, s)
	}
	p.mu.Unlock()
	for _, s := range subs {
		s.setAvail(mask)
	}
	for {
		pkt, _, err := track.ReadRTP()
		if err != nil {
			return
		}
		p.room.bytesIn.Add(int64(len(pkt.Payload) + 12))
		p.mu.RLock()
		for _, s := range p.subs {
			s.forwardVideo(rid, pkt)
		}
		p.mu.RUnlock()
	}
}

// attach marks p ready and subscribes it to every other ready peer (and them
// to p). Called after p's first answer.
func (r *Room) attach(p *Peer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.peers[p.id] != p {
		return // left meanwhile
	}
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
		for _, pair := range [][2]*Peer{{o, p}, {p, o}} {
			if err := subscribe(pair[0], pair[1]); err != nil {
				r.e.logger.Warn("av: subscribe", "from", pair[0].id, "to", pair[1].id, "err", err)
			}
		}
	}
}

// subscribe makes `to` receive the media of `from`.
func subscribe(from, to *Peer) error {
	vt, err := webrtc.NewTrackLocalStaticRTP(vp8Cap, "video", from.id)
	if err != nil {
		return err
	}
	at, err := webrtc.NewTrackLocalStaticRTP(opusCap, "audio", from.id)
	if err != nil {
		return err
	}
	// A new send-only transceiver per track: AddTrack could take over the
	// receive-only one that carries the participant's own published media.
	sendOnly := webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionSendonly}
	vtr, err := to.pc.AddTransceiverFromTrack(vt, sendOnly)
	if err != nil {
		return err
	}
	atr, err := to.pc.AddTransceiverFromTrack(at, sendOnly)
	if err != nil {
		_ = vtr.Stop()
		return err
	}
	vs, as := vtr.Sender(), atr.Sender()
	s := &sub{
		from: from, to: to, vtr: vtr, atr: atr,
		vw: vt, aw: at, blocked: &from.audioBlocked,
		requestKeyframe: from.requestKeyframe,
		countOut:        func(n int) { from.room.bytesOut.Add(int64(n)) },
		want:            LayerLow, done: make(chan struct{}),
	}
	from.mu.Lock()
	for l := range from.video {
		s.avail |= layerBit(l)
	}
	from.subs[to.id] = s
	from.mu.Unlock()
	to.mu.Lock()
	to.recv[from.id] = s
	to.mu.Unlock()
	go s.readRTCP(vs)
	go s.readRTCP(as)
	go s.pendingLoop()
	s.mu.Lock()
	t := s.target()
	s.mu.Unlock()
	if t != "" {
		from.requestKeyframe(t)
	}
	to.negotiate()
	return nil
}

// readRTCP relays PLI/FIR from the receiver to the sender (current layer).
// REMB is ignored: layer selection by bandwidth is out of scope.
func (s *sub) readRTCP(sender *webrtc.RTPSender) {
	for {
		pkts, _, err := sender.ReadRTCP()
		if err != nil {
			return
		}
		for _, pk := range pkts {
			switch pk.(type) {
			case *rtcp.PictureLossIndication, *rtcp.FullIntraRequest:
				if l := s.keyframeLayer(); l != LayerOff && l != "" {
					s.requestKeyframe(l)
				}
			}
		}
	}
}
