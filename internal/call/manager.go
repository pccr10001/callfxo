package call

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
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
	"github.com/emiago/sipgo/sip"
	"github.com/pion/rtp"
	psdp "github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v3"

	"github.com/pccr10001/callfxo/internal/config"
	"github.com/pccr10001/callfxo/internal/sipx"
	"github.com/pccr10001/callfxo/internal/store"
)

const (
	IncomingEventRinging  = "ringing"
	IncomingEventStopped  = "stopped"
	IncomingEventAnswered = "answered"
)

type SignalCallbacks struct {
	OnICECandidate func(c webrtc.ICECandidateInit)
	OnState        func(state string, detail string)
	OnHangup       func(reason string)
}

type IncomingCall struct {
	ID                    string    `json:"id"`
	SIPCallID             string    `json:"sip_call_id"`
	BoxID                 int64     `json:"box_id"`
	BoxName               string    `json:"box_name"`
	CallerID              string    `json:"caller_id"`
	RemoteNumber          string    `json:"remote_number"`
	State                 string    `json:"state"`
	CreatedAt             time.Time `json:"created_at"`
	ExpiresAt             time.Time `json:"expires_at"`
	AnsweredByUserID      int64     `json:"answered_by_user_id,omitempty"`
	AnsweredByDeviceToken string    `json:"answered_by_device_token,omitempty"`
}

type IncomingEvent struct {
	Type    string       `json:"type"`
	Call    IncomingCall `json:"call"`
	UserIDs []int64      `json:"user_ids"`
	Reason  string       `json:"reason,omitempty"`
}

type Notifier interface {
	NotifyIncomingEvent(ctx context.Context, event IncomingEvent)
}

type Manager struct {
	cfg    config.MediaConfig
	sipCfg config.SIPConfig
	store  *store.Store
	sip    *sipx.Service
	api    *webrtc.API
	log    *slog.Logger

	mu           sync.RWMutex
	notifier     Notifier
	callsByID    map[string]*CallSession
	callsBySIPID map[string]*CallSession
	callsByUser  map[int64]*CallSession
	incomingByID map[string]*InboundInvite
}

type CallSession struct {
	ID          string
	Direction   string
	UserID      int64
	DeviceToken string
	BoxID       int64
	Number      string
	CallerID    string

	dialogClient *sipgo.DialogClientSession
	dialogServer *sipgo.DialogServerSession
	pc           *webrtc.PeerConnection

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

type InboundInvite struct {
	meta      IncomingCall
	users     map[int64]struct{}
	dialog    *sipgo.DialogServerSession
	remoteRTP *net.UDPAddr
	sipPT     uint8
	timer     *time.Timer
	createdBy *Manager
	mu        sync.Mutex
	ended     bool
	connected bool
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
		callsByUser:  make(map[int64]*CallSession),
		incomingByID: make(map[string]*InboundInvite),
	}
	sipSvc.SetRemoteByeHandler(mgr.onRemoteBye)
	sipSvc.SetIncomingInviteHandler(mgr.onIncomingInvite)
	return mgr, nil
}

func (m *Manager) SetNotifier(notifier Notifier) {
	m.mu.Lock()
	m.notifier = notifier
	m.mu.Unlock()
}

func (m *Manager) StartCall(ctx context.Context, userID int64, deviceToken string, boxID int64, number string, offerSDP string, cb SignalCallbacks) (*CallSession, string, error) {
	number = strings.TrimSpace(number)
	if number == "" {
		return nil, "", fmt.Errorf("number is required")
	}
	if m.GetUserCall(userID) != nil {
		return nil, "", fmt.Errorf("user already has an active call")
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
	sess, answer, err := m.createWebRTCSide(offerSDP, cb, logID)
	if err != nil {
		return nil, "", err
	}
	sess.Direction = "outgoing"
	sess.UserID = userID
	sess.DeviceToken = strings.TrimSpace(deviceToken)
	sess.BoxID = boxID
	sess.Number = number
	sess.CallerID = number

	publicIP := strings.TrimSpace(m.cfg.PublicIP)
	if publicIP == "" {
		publicIP = strings.TrimSpace(m.sipCfg.AdvertisedIP)
	}
	if publicIP == "" {
		sess.close(false, "failed", "media public ip missing")
		return nil, "", fmt.Errorf("media.public_ip is required")
	}

	localPort := sess.rtpConn.LocalAddr().(*net.UDPAddr).Port
	sdpOffer := buildSIPSDPOffer(publicIP, localPort)
	sipSess, sipAnswer, err := m.sip.Invite(ctx, box, reg, number, []byte(sdpOffer))
	if err != nil {
		sess.close(false, "failed", err.Error())
		return nil, "", err
	}
	sess.dialogClient = sipSess

	remoteRTP, sipPT, err := parseSIPAudioTarget(sipAnswer)
	if err != nil {
		sess.close(true, "failed", "bad remote SDP")
		return nil, "", fmt.Errorf("parse sip SDP answer: %w", err)
	}
	sess.setRemoteRTP(remoteRTP)
	sess.sipPT = sipPT
	sess.markRemoteReady()

	sipCallID := callIDFromResponse(sipSess.InviteResponse)
	m.registerCall(sess, sipCallID)
	go sess.forwardSIPToWebRTC()
	if sess.callbacks.OnState != nil {
		sess.callbacks.OnState("sip_connected", reg.SourceAddr)
	}
	return sess, answer, nil
}

func (m *Manager) GetUserCall(userID int64) *CallSession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.callsByUser[userID]
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

func (m *Manager) ListPendingIncoming(userID int64) []IncomingCall {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]IncomingCall, 0)
	for _, inv := range m.incomingByID {
		if inv == nil || !inv.hasUser(userID) {
			continue
		}
		inv.mu.Lock()
		ended := inv.ended
		meta := inv.meta
		inv.mu.Unlock()
		if ended {
			continue
		}
		out = append(out, meta)
	}
	return out
}

func (m *Manager) AcceptIncoming(ctx context.Context, userID int64, deviceToken, inviteID, offerSDP string, cb SignalCallbacks) (*CallSession, string, error) {
	m.mu.RLock()
	inv := m.incomingByID[strings.TrimSpace(inviteID)]
	m.mu.RUnlock()
	if inv == nil {
		return nil, "", fmt.Errorf("incoming call not found")
	}
	if !inv.hasUser(userID) {
		return nil, "", fmt.Errorf("incoming call not allowed")
	}
	if m.GetUserCall(userID) != nil {
		return nil, "", fmt.Errorf("user already has an active call")
	}

	inv.mu.Lock()
	if inv.ended {
		inv.mu.Unlock()
		return nil, "", fmt.Errorf("incoming call already ended")
	}
	if inv.connected {
		inv.mu.Unlock()
		return nil, "", fmt.Errorf("incoming call already answered")
	}
	inv.meta.State = "connecting"
	inv.meta.AnsweredByUserID = userID
	inv.meta.AnsweredByDeviceToken = strings.TrimSpace(deviceToken)
	if inv.timer != nil {
		inv.timer.Stop()
	}
	inv.mu.Unlock()

	logID, _ := m.store.CreateCallLog(ctx, userID, inv.meta.BoxID, firstNonBlank(inv.meta.CallerID, inv.meta.RemoteNumber), "connecting", "")
	sess, answer, err := m.createWebRTCSide(offerSDP, cb, logID)
	if err != nil {
		_ = inv.rejectWithResponse(sip.StatusInternalServerError, "Accept failed")
		m.finishInbound(inv, IncomingEventStopped, "accept failed")
		return nil, "", err
	}

	publicIP := strings.TrimSpace(m.cfg.PublicIP)
	if publicIP == "" {
		publicIP = strings.TrimSpace(m.sipCfg.AdvertisedIP)
	}
	if publicIP == "" {
		sess.close(false, "failed", "media public ip missing")
		_ = inv.rejectWithResponse(sip.StatusInternalServerError, "Media unavailable")
		m.finishInbound(inv, IncomingEventStopped, "media public ip missing")
		return nil, "", fmt.Errorf("media.public_ip is required")
	}

	sess.Direction = "incoming"
	sess.UserID = userID
	sess.DeviceToken = strings.TrimSpace(deviceToken)
	sess.BoxID = inv.meta.BoxID
	sess.Number = inv.meta.RemoteNumber
	sess.CallerID = inv.meta.CallerID
	sess.dialogServer = inv.dialog
	sess.setRemoteRTP(inv.remoteRTP)
	sess.sipPT = inv.sipPT
	sess.markRemoteReady()

	localPort := sess.rtpConn.LocalAddr().(*net.UDPAddr).Port
	sipAnswer := buildSIPSDPOffer(publicIP, localPort)
	if err := inv.dialog.RespondSDP([]byte(sipAnswer)); err != nil {
		sess.close(false, "failed", "respond sip answer failed")
		m.finishInbound(inv, IncomingEventStopped, "sip answer failed")
		return nil, "", fmt.Errorf("respond incoming invite: %w", err)
	}

	inv.mu.Lock()
	inv.connected = true
	inv.meta.State = "connected"
	meta := inv.meta
	inv.mu.Unlock()

	m.registerCall(sess, meta.SIPCallID)
	m.removeInbound(meta.ID)
	go sess.forwardSIPToWebRTC()
	m.emitIncomingEvent(context.Background(), IncomingEvent{
		Type:    IncomingEventAnswered,
		Call:    meta,
		UserIDs: inv.userIDs(),
		Reason:  "answered",
	})
	return sess, answer, nil
}

func (m *Manager) RejectIncoming(ctx context.Context, userID int64, inviteID, reason string) error {
	m.mu.RLock()
	inv := m.incomingByID[strings.TrimSpace(inviteID)]
	m.mu.RUnlock()
	if inv == nil {
		return store.ErrNotFound
	}
	if !inv.hasUser(userID) {
		return fmt.Errorf("incoming call not allowed")
	}
	if strings.TrimSpace(reason) == "" {
		reason = "declined"
	}
	if err := inv.rejectWithResponse(sip.StatusBusyHere, reason); err != nil {
		return err
	}
	m.finishInbound(inv, IncomingEventStopped, reason)
	return nil
}

func (m *Manager) onIncomingInvite(box store.FXOBox, callerID, remoteNumber string, dlg *sipgo.DialogServerSession) {
	sipCallID := callIDFromRequest(dlg.InviteRequest)
	remoteRTP, sipPT, err := parseSIPAudioTarget(dlg.InviteRequest.Body())
	if err != nil {
		_ = dlg.Respond(sip.StatusNotAcceptableHere, "PCMU required", nil)
		_ = dlg.Close()
		return
	}

	users, err := m.store.ListUsersForIncomingFXO(context.Background(), box.ID)
	if err != nil {
		m.log.Error("list incoming users failed", "box_id", box.ID, "error", err)
		_ = dlg.Respond(sip.StatusInternalServerError, "Server error", nil)
		_ = dlg.Close()
		return
	}
	if len(users) == 0 {
		_ = dlg.Respond(sip.StatusTemporarilyUnavailable, "No receiving devices", nil)
		_ = dlg.Close()
		return
	}
	userIDs := make([]int64, 0, len(users))
	for _, u := range users {
		userIDs = append(userIDs, u.ID)
	}
	devices, err := m.store.ListUserDevicesByUserIDs(context.Background(), userIDs)
	if err != nil {
		m.log.Error("list incoming devices failed", "box_id", box.ID, "error", err)
		_ = dlg.Respond(sip.StatusInternalServerError, "Server error", nil)
		_ = dlg.Close()
		return
	}
	if len(devices) == 0 {
		_ = dlg.Respond(sip.StatusTemporarilyUnavailable, "No reachable devices", nil)
		_ = dlg.Close()
		return
	}

	_ = dlg.Respond(sip.StatusTrying, "Trying", nil)
	_ = dlg.Respond(sip.StatusRinging, "Ringing", nil)

	inv := &InboundInvite{
		meta: IncomingCall{
			ID:           newID(),
			SIPCallID:    sipCallID,
			BoxID:        box.ID,
			BoxName:      box.Name,
			CallerID:     callerID,
			RemoteNumber: remoteNumber,
			State:        "ringing",
			CreatedAt:    time.Now(),
			ExpiresAt:    time.Now().Add(35 * time.Second),
		},
		users:     make(map[int64]struct{}, len(users)),
		dialog:    dlg,
		remoteRTP: remoteRTP,
		sipPT:     sipPT,
		createdBy: m,
	}
	for _, u := range users {
		inv.users[u.ID] = struct{}{}
	}
	inv.timer = time.AfterFunc(35*time.Second, func() {
		_ = inv.rejectWithResponse(sip.StatusTemporarilyUnavailable, "No answer")
		m.finishInbound(inv, IncomingEventStopped, "timeout")
	})

	m.mu.Lock()
	m.incomingByID[inv.meta.ID] = inv
	m.mu.Unlock()
	m.emitIncomingEvent(context.Background(), IncomingEvent{
		Type:    IncomingEventRinging,
		Call:    inv.meta,
		UserIDs: inv.userIDs(),
		Reason:  "incoming",
	})

	go func() {
		<-dlg.Context().Done()
		m.log.Info("dlg.Context().Done() triggered", "callID", sipCallID, "err", dlg.Context().Err())
		inv.mu.Lock()
		ended := inv.ended || inv.connected
		inv.mu.Unlock()
		if ended {
			return
		}
		m.finishInbound(inv, IncomingEventStopped, "remote canceled")
	}()
}

func (m *Manager) onRemoteBye(callID string) {
	m.mu.RLock()
	sess := m.callsBySIPID[callID]
	m.mu.RUnlock()
	if sess != nil {
		sess.close(false, "ended", "remote hangup")
	}
}

func (m *Manager) registerCall(sess *CallSession, sipCallID string) {
	m.mu.Lock()
	m.callsByID[sess.ID] = sess
	if sipCallID != "" {
		m.callsBySIPID[sipCallID] = sess
	}
	if sess.UserID > 0 {
		m.callsByUser[sess.UserID] = sess
	}
	m.mu.Unlock()
}

func (m *Manager) unregisterCall(sess *CallSession) {
	m.mu.Lock()
	delete(m.callsByID, sess.ID)
	if sess.UserID > 0 {
		delete(m.callsByUser, sess.UserID)
	}
	for k, v := range m.callsBySIPID {
		if v == sess {
			delete(m.callsBySIPID, k)
		}
	}
	m.mu.Unlock()
}

func (m *Manager) removeInbound(inviteID string) {
	m.mu.Lock()
	delete(m.incomingByID, inviteID)
	m.mu.Unlock()
}

func (m *Manager) emitIncomingEvent(ctx context.Context, event IncomingEvent) {
	m.mu.RLock()
	notifier := m.notifier
	m.mu.RUnlock()
	if notifier != nil {
		notifier.NotifyIncomingEvent(ctx, event)
	}
}

func (m *Manager) finishInbound(inv *InboundInvite, eventType, reason string) {
	if inv == nil {
		return
	}
	inv.mu.Lock()
	if inv.ended {
		inv.mu.Unlock()
		return
	}
	inv.ended = true
	if inv.timer != nil {
		inv.timer.Stop()
	}
	meta := inv.meta
	meta.State = "ended"
	inv.meta = meta
	inv.mu.Unlock()

	m.removeInbound(meta.ID)
	m.emitIncomingEvent(context.Background(), IncomingEvent{
		Type:    eventType,
		Call:    meta,
		UserIDs: inv.userIDs(),
		Reason:  reason,
	})
	_ = inv.dialog.Close()
}

func (m *Manager) createWebRTCSide(offerSDP string, cb SignalCallbacks, callLogID int64) (*CallSession, string, error) {
	ip := net.ParseIP(m.cfg.RTPBindIP)
	if ip == nil {
		ip = net.IPv4zero
	}
	rtpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: ip, Port: 0})
	if err != nil {
		if callLogID > 0 {
			_ = m.store.EndCallLog(context.Background(), callLogID, "failed", "rtp socket error")
		}
		return nil, "", fmt.Errorf("create rtp socket: %w", err)
	}

	iceServers := make([]webrtc.ICEServer, 0, 1)
	if len(m.cfg.ICESTUNURLs) > 0 {
		iceServers = append(iceServers, webrtc.ICEServer{URLs: trimNonBlankICEURLs(m.cfg.ICESTUNURLs)})
	}
	turnURLs := trimNonBlankICEURLs(m.cfg.ICETURNURLs)
	if len(turnURLs) > 0 {
		username, credential := m.turnCredentials("callfxo-server")
		iceServers = append(iceServers, webrtc.ICEServer{
			URLs:       turnURLs,
			Username:   username,
			Credential: credential,
		})
	}
	pc, err := m.api.NewPeerConnection(webrtc.Configuration{ICEServers: iceServers})
	if err != nil {
		_ = rtpConn.Close()
		if callLogID > 0 {
			_ = m.store.EndCallLog(context.Background(), callLogID, "failed", "webrtc create error")
		}
		return nil, "", fmt.Errorf("create peer connection: %w", err)
	}

	sess := &CallSession{
		ID:          newID(),
		pc:          pc,
		rtpConn:     rtpConn,
		remoteReady: make(chan struct{}),
		callbacks:   cb,
		callLogID:   callLogID,
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
			sess.callbacks.OnState(strings.ToLower(state.String()), "")
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
	ld := pc.LocalDescription()
	if ld == nil {
		sess.close(false, "failed", "local description missing")
		return nil, "", fmt.Errorf("local description not ready")
	}
	return sess, ld.SDP, nil
}

func (c *CallSession) AddICECandidate(candidate webrtc.ICECandidateInit) error {
	if c.pc == nil {
		return fmt.Errorf("peer connection not ready")
	}
	return c.pc.AddICECandidate(candidate)
}

func (c *CallSession) SendDTMF(ctx context.Context, digits string) error {
	if c.dialogClient != nil {
		return c.manager.sip.SendDTMF(ctx, c.dialogClient, digits)
	}
	if c.dialogServer != nil {
		return c.manager.sip.SendServerDTMF(ctx, c.dialogServer, digits)
	}
	return fmt.Errorf("dialog not ready")
}

func (c *CallSession) Hangup(reason string) {
	c.close(true, "ended", reason)
}

func (c *CallSession) close(sendBye bool, status, reason string) {
	c.closeOnce.Do(func() {
		c.markRemoteReady()
		if sendBye {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			switch {
			case c.dialogClient != nil:
				_ = c.manager.sip.Hangup(ctx, c.dialogClient)
			case c.dialogServer != nil:
				_ = c.dialogServer.Bye(ctx)
			}
			cancel()
		}
		if c.dialogClient != nil {
			c.dialogClient.Close()
		}
		if c.dialogServer != nil {
			c.dialogServer.Close()
		}
		if c.rtpConn != nil {
			_ = c.rtpConn.Close()
		}
		if c.pc != nil {
			_ = c.pc.Close()
		}
		c.manager.unregisterCall(c)
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

func (inv *InboundInvite) hasUser(userID int64) bool {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	_, ok := inv.users[userID]
	return ok
}

func (inv *InboundInvite) userIDs() []int64 {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	out := make([]int64, 0, len(inv.users))
	for id := range inv.users {
		out = append(out, id)
	}
	return out
}

func (inv *InboundInvite) rejectWithResponse(statusCode int, reason string) error {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	if inv.ended || inv.connected {
		return nil
	}
	if inv.timer != nil {
		inv.timer.Stop()
	}
	if err := inv.dialog.Respond(statusCode, reason, nil); err != nil {
		return err
	}
	return nil
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

func (m *Manager) turnCredentials(subject string) (string, string) {
	sharedSecret := strings.TrimSpace(m.cfg.ICETURNSharedSecret)
	if sharedSecret != "" {
		ttl := time.Duration(m.cfg.ICETURNCredentialTTLMinute) * time.Minute
		if ttl <= 0 {
			ttl = time.Duration(config.Default().Media.ICETURNCredentialTTLMinute) * time.Minute
		}
		return buildTURNCredentials(sharedSecret, normalizeTurnSubject(subject), ttl)
	}
	return strings.TrimSpace(m.cfg.ICETURNUsername), strings.TrimSpace(m.cfg.ICETURNCredential)
}

func buildTURNCredentials(secret, subject string, ttl time.Duration) (string, string) {
	exp := time.Now().Add(ttl).Unix()
	username := fmt.Sprintf("%d:%s", exp, subject)
	mac := hmac.New(sha1.New, []byte(secret))
	_, _ = mac.Write([]byte(username))
	credential := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return username, credential
}

func normalizeTurnSubject(subject string) string {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return "callfxo"
	}
	return strings.ReplaceAll(subject, ":", "_")
}

func trimNonBlankICEURLs(items []string) []string {
	out := make([]string, 0, len(items))
	for _, raw := range items {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("call-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func callIDFromResponse(res *sip.Response) string {
	if res == nil || res.CallID() == nil {
		return ""
	}
	return string(*res.CallID())
}

func callIDFromRequest(req *sip.Request) string {
	if req == nil || req.CallID() == nil {
		return ""
	}
	return string(*req.CallID())
}

func firstNonBlank(values ...string) string {
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
