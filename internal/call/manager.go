package call

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/emiago/sipgo"
	"github.com/pion/rtp"
	psdp "github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v3"

	"github.com/pccr10001/callfxo/internal/config"
	"github.com/pccr10001/callfxo/internal/sipx"
	"github.com/pccr10001/callfxo/internal/store"
)

type SignalCallbacks struct {
	OnICECandidate func(c webrtc.ICECandidateInit)
	OnState        func(state string, detail string)
	OnHangup       func(reason string)
}

type Manager struct {
	cfg    config.MediaConfig
	sipCfg config.SIPConfig
	store  *store.Store
	sip    *sipx.Service
	api    *webrtc.API
	log    *slog.Logger

	mu           sync.RWMutex
	callsByID    map[string]*CallSession
	callsBySIPID map[string]*CallSession
}

type CallSession struct {
	ID     string
	BoxID  int64
	Number string

	dialog *sipgo.DialogClientSession
	pc     *webrtc.PeerConnection

	localTrack *webrtc.TrackLocalStaticRTP
	rtpConn    *net.UDPConn

	remoteMu    sync.RWMutex
	remoteRTP   *net.UDPAddr
	remoteReady chan struct{}
	readyOnce   sync.Once
	sipPT       uint8
	webRTCPT    uint8

	callbacks SignalCallbacks
	callLogID int64

	manager   *Manager
	closeOnce sync.Once
}

func NewManager(cfg config.MediaConfig, sipCfg config.SIPConfig, st *store.Store, sipSvc *sipx.Service, log *slog.Logger) (*Manager, error) {
	if log == nil {
		log = slog.Default()
	}
	m := webrtc.MediaEngine{}
	if err := m.RegisterDefaultCodecs(); err != nil {
		return nil, fmt.Errorf("register webrtc codecs: %w", err)
	}
	api := webrtc.NewAPI(webrtc.WithMediaEngine(&m))

	mgr := &Manager{
		cfg:          cfg,
		sipCfg:       sipCfg,
		store:        st,
		sip:          sipSvc,
		api:          api,
		log:          log,
		callsByID:    make(map[string]*CallSession),
		callsBySIPID: make(map[string]*CallSession),
	}
	sipSvc.SetRemoteByeHandler(mgr.onRemoteBye)
	return mgr, nil
}

func (m *Manager) StartCall(ctx context.Context, userID, boxID int64, number string, offerSDP string, cb SignalCallbacks) (*CallSession, string, error) {
	number = strings.TrimSpace(number)
	if number == "" {
		return nil, "", fmt.Errorf("number is required")
	}

	box, err := m.store.GetFXOBoxByID(ctx, boxID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, "", fmt.Errorf("fxo box not found")
		}
		return nil, "", err
	}

	reg, err := m.store.GetActiveRegistration(ctx, boxID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, "", fmt.Errorf("fxo box is offline")
		}
		return nil, "", err
	}

	logID, _ := m.store.CreateCallLog(ctx, userID, boxID, number, "dialing", "")

	ip := net.ParseIP(m.cfg.RTPBindIP)
	if ip == nil {
		ip = net.IPv4zero
	}
	rtpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: ip, Port: 0})
	if err != nil {
		_ = m.store.EndCallLog(ctx, logID, "failed", "rtp socket error")
		return nil, "", fmt.Errorf("create rtp socket: %w", err)
	}

	iceServers := make([]webrtc.ICEServer, 0, 1)
	if len(m.cfg.ICESTUNURLs) > 0 {
		iceServers = append(iceServers, webrtc.ICEServer{URLs: m.cfg.ICESTUNURLs})
	}
	pc, err := m.api.NewPeerConnection(webrtc.Configuration{ICEServers: iceServers})
	if err != nil {
		_ = rtpConn.Close()
		_ = m.store.EndCallLog(ctx, logID, "failed", "webrtc create error")
		return nil, "", fmt.Errorf("create peer connection: %w", err)
	}

	sess := &CallSession{
		ID:          newID(),
		BoxID:       boxID,
		Number:      number,
		pc:          pc,
		rtpConn:     rtpConn,
		remoteReady: make(chan struct{}),
		callbacks:   cb,
		callLogID:   logID,
		manager:     m,
	}

	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		if sess.callbacks.OnICECandidate != nil {
			sess.callbacks.OnICECandidate(c.ToJSON())
		}
	})

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if sess.callbacks.OnState != nil {
			sess.callbacks.OnState(state.String(), "")
		}
		switch state {
		case webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateClosed:
			sess.close(true, "failed", "webrtc disconnected")
		}
	})

	pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		if track.Kind() != webrtc.RTPCodecTypeAudio {
			return
		}
		if !strings.EqualFold(track.Codec().MimeType, webrtc.MimeTypePCMU) {
			sess.close(true, "failed", "browser codec is not PCMU, transcoder required")
			return
		}
		go sess.forwardWebRTCToSIP(track)
	})

	track, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypePCMU, ClockRate: 8000, Channels: 1},
		"audio", "callfxo",
	)
	if err != nil {
		sess.close(false, "failed", "create local track failed")
		return nil, "", fmt.Errorf("create local track: %w", err)
	}
	sess.localTrack = track

	sender, err := pc.AddTrack(track)
	if err != nil {
		sess.close(false, "failed", "add local track failed")
		return nil, "", fmt.Errorf("add local track: %w", err)
	}
	go drainRTCP(sender)

	if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: offerSDP}); err != nil {
		sess.close(false, "failed", "set remote SDP failed")
		return nil, "", fmt.Errorf("set remote description: %w", err)
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		sess.close(false, "failed", "create answer failed")
		return nil, "", fmt.Errorf("create answer: %w", err)
	}
	if err := pc.SetLocalDescription(answer); err != nil {
		sess.close(false, "failed", "set local SDP failed")
		return nil, "", fmt.Errorf("set local description: %w", err)
	}
	webRTCPT, err := parseWebRTCAudioPCMUPayload(answer.SDP)
	if err != nil {
		sess.close(false, "failed", "browser codec must support PCMU")
		return nil, "", fmt.Errorf("webrtc PCMU negotiation failed: %w", err)
	}
	sess.webRTCPT = webRTCPT

	publicIP := strings.TrimSpace(m.cfg.PublicIP)
	if publicIP == "" {
		publicIP = strings.TrimSpace(m.sipCfg.AdvertisedIP)
	}
	if publicIP == "" {
		sess.close(false, "failed", "media public ip missing")
		return nil, "", fmt.Errorf("media.public_ip is required")
	}

	localPort := rtpConn.LocalAddr().(*net.UDPAddr).Port
	sdpOffer := buildSIPSDPOffer(publicIP, localPort)
	sipSess, sipAnswer, err := m.sip.Invite(ctx, box, reg, number, []byte(sdpOffer))
	if err != nil {
		sess.close(false, "failed", err.Error())
		return nil, "", err
	}
	sess.dialog = sipSess

	remoteRTP, sipPT, err := parseSIPAudioTarget(sipAnswer)
	if err != nil {
		sess.close(true, "failed", "bad remote SDP")
		return nil, "", fmt.Errorf("parse sip SDP answer: %w", err)
	}
	sess.setRemoteRTP(remoteRTP)
	sess.sipPT = sipPT
	sess.markRemoteReady()

	sipCallID := ""
	if sipSess.InviteResponse != nil && sipSess.InviteResponse.CallID() != nil {
		sipCallID = string(*sipSess.InviteResponse.CallID())
	}

	m.register(sess, sipCallID)
	go sess.forwardSIPToWebRTC()

	if sess.callbacks.OnState != nil {
		sess.callbacks.OnState("sip_connected", reg.SourceAddr)
	}

	ld := pc.LocalDescription()
	if ld == nil {
		sess.close(true, "failed", "local description missing")
		return nil, "", fmt.Errorf("local description not ready")
	}
	return sess, ld.SDP, nil
}

func (m *Manager) onRemoteBye(callID string) {
	m.mu.RLock()
	sess := m.callsBySIPID[callID]
	m.mu.RUnlock()
	if sess != nil {
		sess.close(false, "ended", "remote hangup")
	}
}

func (m *Manager) register(sess *CallSession, sipCallID string) {
	m.mu.Lock()
	m.callsByID[sess.ID] = sess
	if sipCallID != "" {
		m.callsBySIPID[sipCallID] = sess
	}
	m.mu.Unlock()
}

func (m *Manager) unregister(sess *CallSession) {
	m.mu.Lock()
	delete(m.callsByID, sess.ID)
	for k, v := range m.callsBySIPID {
		if v == sess {
			delete(m.callsBySIPID, k)
		}
	}
	m.mu.Unlock()
}

func (m *Manager) IsBoxInUse(boxID int64) bool {
	if boxID <= 0 {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, cs := range m.callsByID {
		if cs != nil && cs.BoxID == boxID {
			return true
		}
	}
	return false
}

func (c *CallSession) AddICECandidate(candidate webrtc.ICECandidateInit) error {
	if c.pc == nil {
		return fmt.Errorf("peer connection not ready")
	}
	return c.pc.AddICECandidate(candidate)
}

func (c *CallSession) SendDTMF(ctx context.Context, digits string) error {
	return c.manager.sip.SendDTMF(ctx, c.dialog, digits)
}

func (c *CallSession) Hangup(reason string) {
	c.close(true, "ended", reason)
}

func (c *CallSession) close(sendBye bool, status, reason string) {
	c.closeOnce.Do(func() {
		c.markRemoteReady()
		if sendBye && c.dialog != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			_ = c.manager.sip.Hangup(ctx, c.dialog)
			cancel()
		}
		if c.dialog != nil {
			c.dialog.Close()
		}
		if c.rtpConn != nil {
			_ = c.rtpConn.Close()
		}
		if c.pc != nil {
			_ = c.pc.Close()
		}
		c.manager.unregister(c)
		if c.callLogID > 0 {
			_ = c.manager.store.EndCallLog(context.Background(), c.callLogID, status, reason)
		}
		if c.callbacks.OnHangup != nil {
			c.callbacks.OnHangup(reason)
		}
	})
}

func (c *CallSession) forwardWebRTCToSIP(track *webrtc.TrackRemote) {
	<-c.remoteReady
	for {
		packet, _, err := track.ReadRTP()
		if err != nil {
			return
		}
		packet.PayloadType = c.sipPT
		raw, err := packet.Marshal()
		if err != nil {
			continue
		}
		target := c.getRemoteRTP()
		if target == nil {
			continue
		}
		if _, err := c.rtpConn.WriteToUDP(raw, target); err != nil {
			return
		}
	}
}

func (c *CallSession) forwardSIPToWebRTC() {
	buf := make([]byte, 2000)
	for {
		n, src, err := c.rtpConn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		if c.getRemoteRTP() == nil {
			c.setRemoteRTP(src)
		}
		var pkt rtp.Packet
		if err := pkt.Unmarshal(buf[:n]); err != nil {
			continue
		}
		pkt.PayloadType = c.webRTCPT
		if err := c.localTrack.WriteRTP(&pkt); err != nil {
			return
		}
	}
}

func (c *CallSession) setRemoteRTP(addr *net.UDPAddr) {
	c.remoteMu.Lock()
	c.remoteRTP = addr
	c.remoteMu.Unlock()
}

func (c *CallSession) markRemoteReady() {
	c.readyOnce.Do(func() {
		close(c.remoteReady)
	})
}

func (c *CallSession) getRemoteRTP() *net.UDPAddr {
	c.remoteMu.RLock()
	defer c.remoteMu.RUnlock()
	if c.remoteRTP == nil {
		return nil
	}
	copyAddr := *c.remoteRTP
	return &copyAddr
}

func buildSIPSDPOffer(ip string, port int) string {
	sid := time.Now().UnixNano()
	return fmt.Sprintf("v=0\r\no=- %d %d IN IP4 %s\r\ns=callfxo\r\nc=IN IP4 %s\r\nt=0 0\r\nm=audio %d RTP/AVP 0 101\r\na=rtpmap:0 PCMU/8000\r\na=rtpmap:101 telephone-event/8000\r\na=fmtp:101 0-16\r\na=ptime:20\r\na=sendrecv\r\n", sid, sid, ip, ip, port)
}

func parseSIPAudioTarget(sdpAnswer []byte) (*net.UDPAddr, uint8, error) {
	var s psdp.SessionDescription
	if err := s.Unmarshal(sdpAnswer); err != nil {
		return nil, 0, fmt.Errorf("unmarshal sdp: %w", err)
	}

	host := ""
	if s.ConnectionInformation != nil {
		host = sanitizeSDPAddress(s.ConnectionInformation.Address.Address)
	}
	for _, md := range s.MediaDescriptions {
		if md.MediaName.Media != "audio" {
			continue
		}
		if md.ConnectionInformation != nil {
			host = sanitizeSDPAddress(md.ConnectionInformation.Address.Address)
		}
		port := int(md.MediaName.Port.Value)
		if host == "" || port == 0 {
			continue
		}
		pt, ok := findPCMUPayload(md)
		if !ok {
			return nil, 0, fmt.Errorf("remote audio codec is not PCMU; transcoder required")
		}
		addr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(host, strconv.Itoa(port)))
		if err != nil {
			return nil, 0, fmt.Errorf("resolve remote rtp: %w", err)
		}
		return addr, pt, nil
	}
	return nil, 0, fmt.Errorf("audio media target not found")
}

func parseWebRTCAudioPCMUPayload(sdpText string) (uint8, error) {
	var s psdp.SessionDescription
	if err := s.Unmarshal([]byte(sdpText)); err != nil {
		return 0, fmt.Errorf("unmarshal webrtc sdp: %w", err)
	}
	for _, md := range s.MediaDescriptions {
		if md.MediaName.Media != "audio" {
			continue
		}
		pt, ok := findPCMUPayload(md)
		if ok {
			return pt, nil
		}
	}
	return 0, fmt.Errorf("PCMU payload not found in webrtc answer")
}

func findPCMUPayload(md *psdp.MediaDescription) (uint8, bool) {
	if md == nil {
		return 0, false
	}
	formatSet := make(map[int]struct{}, len(md.MediaName.Formats))
	for _, f := range md.MediaName.Formats {
		n, err := strconv.Atoi(strings.TrimSpace(f))
		if err != nil || n < 0 || n > 255 {
			continue
		}
		formatSet[n] = struct{}{}
		if n == 0 {
			return 0, true
		}
	}

	for _, a := range md.Attributes {
		if !strings.EqualFold(a.Key, "rtpmap") || strings.TrimSpace(a.Value) == "" {
			continue
		}
		parts := strings.Fields(a.Value)
		if len(parts) < 2 {
			continue
		}
		ptNum, err := strconv.Atoi(parts[0])
		if err != nil || ptNum < 0 || ptNum > 255 {
			continue
		}
		codec := strings.ToUpper(strings.SplitN(parts[1], "/", 2)[0])
		if codec != "PCMU" {
			continue
		}
		if _, ok := formatSet[ptNum]; ok {
			return uint8(ptNum), true
		}
	}
	return 0, false
}

func sanitizeSDPAddress(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}
	if idx := strings.Index(addr, "/"); idx > 0 {
		return addr[:idx]
	}
	return addr
}

func drainRTCP(sender *webrtc.RTPSender) {
	buf := make([]byte, 1500)
	for {
		if _, _, err := sender.Read(buf); err != nil {
			return
		}
	}
}

func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("call-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
