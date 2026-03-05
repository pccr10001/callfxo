package sipx

import (
	"context"
	"crypto/md5"
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
	"github.com/icholy/digest"

	"github.com/pccr10001/callfxo/internal/config"
	"github.com/pccr10001/callfxo/internal/store"
)

type Service struct {
	cfg   config.SIPConfig
	store *store.Store
	log   *slog.Logger

	ua      *sipgo.UserAgent
	server  *sipgo.Server
	client  *sipgo.Client
	dialogs *sipgo.DialogClientCache

	nonceTTL time.Duration
	nonceMu  sync.RWMutex
	nonces   map[string]time.Time

	byeMu       sync.RWMutex
	onRemoteBYE func(callID string)
}

func New(cfg config.SIPConfig, st *store.Store, log *slog.Logger) (*Service, error) {
	if log == nil {
		log = slog.Default()
	}

	nonceTTL := time.Duration(cfg.NonceTTLHours) * time.Hour
	if nonceTTL <= 0 {
		nonceTTL = time.Hour
	}

	ua, err := sipgo.NewUA(sipgo.WithUserAgent("callfxo"))
	if err != nil {
		return nil, fmt.Errorf("create sip ua: %w", err)
	}

	client, err := sipgo.NewClient(ua, sipgo.WithClientConnectionAddr(net.JoinHostPort(cfg.AdvertisedIP, "0")))
	if err != nil {
		_ = ua.Close()
		return nil, fmt.Errorf("create sip client: %w", err)
	}

	srv, err := sipgo.NewServer(ua)
	if err != nil {
		_ = client.Close()
		_ = ua.Close()
		return nil, fmt.Errorf("create sip server: %w", err)
	}

	contactPort, err := portFromListen(cfg.Listen)
	if err != nil {
		_ = srv.Close()
		_ = client.Close()
		_ = ua.Close()
		return nil, err
	}

	uriParams := sip.NewParams()
	uriParams.Add("transport", cfg.Transport)
	contactHDR := sip.ContactHeader{
		Address: sip.Uri{
			Scheme:    "sip",
			User:      cfg.ContactUser,
			Host:      cfg.AdvertisedIP,
			Port:      contactPort,
			UriParams: uriParams,
		},
	}

	s := &Service{
		cfg:      cfg,
		store:    st,
		log:      log,
		ua:       ua,
		server:   srv,
		client:   client,
		dialogs:  sipgo.NewDialogClientCache(client, contactHDR),
		nonceTTL: nonceTTL,
		nonces:   make(map[string]time.Time),
	}
	s.configureHandlers()
	return s, nil
}

func (s *Service) configureHandlers() {
	s.server.OnRegister(s.onRegister)
	s.server.OnBye(s.onBye)
	s.server.OnInvite(func(req *sip.Request, tx sip.ServerTransaction) {
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusNotImplemented, "Outgoing call only", nil))
	})
	s.server.OnNoRoute(func(req *sip.Request, tx sip.ServerTransaction) {
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusMethodNotAllowed, "Method not allowed", nil))
	})
}

func (s *Service) Run(ctx context.Context) error {
	go s.cleanupNoncesLoop(ctx)
	s.log.Info("SIP service listening", "transport", s.cfg.Transport, "addr", s.cfg.Listen)
	if err := s.server.ListenAndServe(ctx, s.cfg.Transport, s.cfg.Listen); err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	}
	return nil
}

func (s *Service) Close() {
	_ = s.server.Close()
	_ = s.client.Close()
	_ = s.ua.Close()
}

func (s *Service) SetRemoteByeHandler(fn func(callID string)) {
	s.byeMu.Lock()
	s.onRemoteBYE = fn
	s.byeMu.Unlock()
}

func (s *Service) Invite(ctx context.Context, box store.FXOBox, reg store.Registration, number string, sdpOffer []byte) (*sipgo.DialogClientSession, []byte, error) {
	recipient, err := recipientForDial(reg, number)
	if err != nil {
		return nil, nil, err
	}

	contentType := sip.ContentTypeHeader("application/sdp")
	sess, err := s.dialogs.Invite(ctx, recipient, sdpOffer, &contentType)
	if err != nil {
		return nil, nil, fmt.Errorf("send INVITE failed: %w", err)
	}

	if err := sess.WaitAnswer(ctx, sipgo.AnswerOptions{
		Username: box.SIPUsername,
		Password: box.SIPPassword,
	}); err != nil {
		sess.Close()
		return nil, nil, fmt.Errorf("wait INVITE answer failed: %w", err)
	}

	if sess.InviteResponse == nil || sess.InviteResponse.StatusCode < 200 || sess.InviteResponse.StatusCode >= 300 {
		status := 0
		if sess.InviteResponse != nil {
			status = sess.InviteResponse.StatusCode
		}
		sess.Close()
		return nil, nil, fmt.Errorf("invite rejected status=%d", status)
	}

	if err := sess.Ack(ctx); err != nil {
		sess.Close()
		return nil, nil, fmt.Errorf("send ACK failed: %w", err)
	}

	answer := sess.InviteResponse.Body()
	if len(answer) == 0 {
		s.log.Warn("INVITE accepted without SDP", "box_id", box.ID)
	}
	return sess, answer, nil
}

func (s *Service) SendDTMF(ctx context.Context, sess *sipgo.DialogClientSession, digits string) error {
	if sess == nil {
		return fmt.Errorf("empty dialog session")
	}
	if digits == "" {
		return nil
	}

	recipient := sip.Uri{}
	if sess.InviteResponse != nil && sess.InviteResponse.Contact() != nil {
		recipient = sess.InviteResponse.Contact().Address
	} else {
		recipient = sess.InviteRequest.Recipient
	}

	for _, r := range digits {
		if !isDTMFDigit(r) {
			return fmt.Errorf("invalid dtmf digit: %q", string(r))
		}
		req := sip.NewRequest(sip.INFO, recipient)
		ct := sip.ContentTypeHeader("application/dtmf-relay")
		req.AppendHeader(&ct)
		req.SetBody([]byte(fmt.Sprintf("Signal=%c\r\nDuration=160\r\n", r)))

		res, err := sess.Do(ctx, req)
		if err != nil {
			return fmt.Errorf("send DTMF INFO failed: %w", err)
		}
		if res.StatusCode >= 300 {
			return fmt.Errorf("dtmf rejected status=%d", res.StatusCode)
		}
		time.Sleep(120 * time.Millisecond)
	}
	return nil
}

func (s *Service) Hangup(ctx context.Context, sess *sipgo.DialogClientSession) error {
	if sess == nil {
		return nil
	}
	err := sess.Bye(ctx)
	sess.Close()
	if err != nil {
		return fmt.Errorf("send BYE failed: %w", err)
	}
	return nil
}

func (s *Service) onRegister(req *sip.Request, tx sip.ServerTransaction) {
	authHeader := req.GetHeader("Authorization")
	if authHeader == nil {
		s.replyAuthChallenge(req, tx)
		return
	}

	cred, err := digest.ParseCredentials(authHeader.Value())
	if err != nil {
		s.log.Warn("REGISTER auth parse failed", "error", err)
		s.replyAuthChallenge(req, tx)
		return
	}

	if strings.TrimSpace(cred.Username) == "" {
		if to := req.To(); to != nil {
			cred.Username = to.Address.User
		}
	}
	if cred.Username == "" {
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusForbidden, "Missing username", nil))
		return
	}

	if !s.validateNonce(cred.Nonce) {
		s.log.Info("REGISTER rejected by nonce", "username", cred.Username)
		s.replyAuthChallenge(req, tx)
		return
	}

	box, err := s.store.GetFXOBoxBySIPUsername(context.Background(), cred.Username)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusForbidden, "Unknown FXO account", nil))
			return
		}
		s.log.Error("REGISTER db lookup failed", "error", err)
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusInternalServerError, "Server error", nil))
		return
	}

	if !verifyDigestResponse(cred, box.SIPPassword, string(req.Method)) {
		s.log.Info("REGISTER auth mismatch", "username", cred.Username)
		s.replyAuthChallenge(req, tx)
		return
	}

	expires := parseRegisterExpires(req)
	if expires <= 0 {
		if err := s.store.DeleteRegistration(context.Background(), box.ID); err != nil {
			s.log.Warn("delete registration failed", "box_id", box.ID, "error", err)
		}
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil))
		s.log.Info("FXO unregistered", "box_id", box.ID, "username", box.SIPUsername)
		return
	}

	contact := req.Contact()
	if contact == nil || contact.Address.Wildcard {
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusBadRequest, "Missing contact", nil))
		return
	}

	callID := ""
	if cid := req.CallID(); cid != nil {
		callID = string(*cid)
	}
	userAgent := ""
	if ua := req.GetHeader("User-Agent"); ua != nil {
		userAgent = ua.Value()
	}

	now := time.Now()
	reg := store.Registration{
		FXOBoxID:   box.ID,
		ContactURI: contact.Address.String(),
		SourceAddr: req.Source(),
		Transport:  req.Transport(),
		CallID:     callID,
		UserAgent:  userAgent,
		ExpiresAt:  now.Add(time.Duration(expires) * time.Second),
		UpdatedAt:  now,
	}
	if err := s.store.UpsertRegistration(context.Background(), reg); err != nil {
		s.log.Error("upsert registration failed", "box_id", box.ID, "error", err)
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusInternalServerError, "Server error", nil))
		return
	}

	_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil))
	s.log.Info("FXO registered", "box_id", box.ID, "username", box.SIPUsername, "source", req.Source())
}

func (s *Service) onBye(req *sip.Request, tx sip.ServerTransaction) {
	err := s.dialogs.ReadBye(req, tx)
	if err != nil {
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusCallTransactionDoesNotExists, "Dialog not found", nil))
		return
	}

	callID := ""
	if cid := req.CallID(); cid != nil {
		callID = string(*cid)
	}

	s.byeMu.RLock()
	fn := s.onRemoteBYE
	s.byeMu.RUnlock()
	if fn != nil && callID != "" {
		fn(callID)
	}
}

func (s *Service) replyAuthChallenge(req *sip.Request, tx sip.ServerTransaction) {
	nonce := newToken(16)
	opaque := newToken(8)
	s.storeNonce(nonce)

	www := fmt.Sprintf(`Digest realm="%s", nonce="%s", opaque="%s", algorithm=MD5`, s.cfg.Realm, nonce, opaque)
	res := sip.NewResponseFromRequest(req, sip.StatusUnauthorized, "Unauthorized", nil)
	res.AppendHeader(sip.NewHeader("WWW-Authenticate", www))
	_ = tx.Respond(res)
}

func (s *Service) storeNonce(nonce string) {
	s.nonceMu.Lock()
	s.nonces[nonce] = time.Now().Add(s.nonceTTL)
	s.nonceMu.Unlock()
}

func (s *Service) validateNonce(nonce string) bool {
	if nonce == "" {
		return false
	}
	s.nonceMu.RLock()
	exp, ok := s.nonces[nonce]
	s.nonceMu.RUnlock()
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		s.nonceMu.Lock()
		delete(s.nonces, nonce)
		s.nonceMu.Unlock()
		return false
	}
	return true
}

func (s *Service) cleanupNoncesLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			now := time.Now()
			s.nonceMu.Lock()
			for n, exp := range s.nonces {
				if now.After(exp) {
					delete(s.nonces, n)
				}
			}
			s.nonceMu.Unlock()
		case <-ctx.Done():
			return
		}
	}
}

func verifyDigestResponse(cred *digest.Credentials, password, method string) bool {
	if cred == nil {
		return false
	}
	ha1 := md5Hex(cred.Username + ":" + cred.Realm + ":" + password)
	ha2 := md5Hex(method + ":" + cred.URI)

	var kd string
	if cred.QOP == "" {
		kd = ha1 + ":" + cred.Nonce + ":" + ha2
	} else {
		nc := fmt.Sprintf("%08x", cred.Nc)
		kd = ha1 + ":" + cred.Nonce + ":" + nc + ":" + cred.Cnonce + ":" + cred.QOP + ":" + ha2
	}
	expected := md5Hex(kd)
	return strings.EqualFold(expected, cred.Response)
}

func md5Hex(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

func parseRegisterExpires(req *sip.Request) int {
	if c := req.Contact(); c != nil {
		if exp, ok := c.Params.Get("expires"); ok {
			if n, err := strconv.Atoi(strings.TrimSpace(exp)); err == nil {
				return n
			}
		}
	}
	if h := req.GetHeader("Expires"); h != nil {
		if n, err := strconv.Atoi(strings.TrimSpace(h.Value())); err == nil {
			return n
		}
	}
	return 3600
}

func recipientForDial(reg store.Registration, number string) (sip.Uri, error) {
	var uri sip.Uri
	if err := sip.ParseUri(reg.ContactURI, &uri); err != nil {
		return sip.Uri{}, fmt.Errorf("parse contact uri: %w", err)
	}
	uri.User = number
	if reg.Transport != "" {
		if uri.UriParams == nil {
			uri.UriParams = sip.NewParams()
		}
		uri.UriParams.Add("transport", reg.Transport)
	}
	if uri.Host == "" {
		host, port, err := net.SplitHostPort(reg.SourceAddr)
		if err != nil {
			return sip.Uri{}, fmt.Errorf("missing recipient host")
		}
		uri.Host = host
		p, _ := strconv.Atoi(port)
		uri.Port = p
	}
	return uri, nil
}

func isDTMFDigit(r rune) bool {
	return (r >= '0' && r <= '9') || r == '*' || r == '#' || r == 'A' || r == 'B' || r == 'C' || r == 'D'
}

func newToken(n int) string {
	b := make([]byte, n)
	_, _ = time.Now().UTC().MarshalBinary()
	for i := range b {
		b[i] = byte(time.Now().UnixNano() >> (i % 8))
	}
	return hex.EncodeToString(b)
}

func portFromListen(addr string) (int, error) {
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return 0, fmt.Errorf("invalid sip.listen format, need host:port: %w", err)
	}
	p, err := strconv.Atoi(portStr)
	if err != nil {
		return 0, fmt.Errorf("invalid sip.listen port: %w", err)
	}
	return p, nil
}
