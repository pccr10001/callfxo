package web

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
	"golang.org/x/crypto/bcrypt"

	"github.com/pccr10001/callfxo/internal/auth"
	"github.com/pccr10001/callfxo/internal/call"
	"github.com/pccr10001/callfxo/internal/config"
	"github.com/pccr10001/callfxo/internal/push"
	"github.com/pccr10001/callfxo/internal/store"
)

type Server struct {
	cfg     config.Config
	store   *store.Store
	authMgr *auth.Manager
	calls   *call.Manager
	push    *push.Service
	log     *slog.Logger

	upgrader websocket.Upgrader

	wsMu      sync.RWMutex
	wsClients map[*wsClient]struct{}

	boxStateMu sync.Mutex
	boxState   map[int64]boxRuntimeState
}

type wsMessage struct {
	Type      string                   `json:"type"`
	BoxID     int64                    `json:"box_id,omitempty"`
	InviteID  string                   `json:"invite_id,omitempty"`
	Number    string                   `json:"number,omitempty"`
	SDP       string                   `json:"sdp,omitempty"`
	Digits    string                   `json:"digits,omitempty"`
	Candidate *webrtc.ICECandidateInit `json:"candidate,omitempty"`
}

type wsClient struct {
	conn        *websocket.Conn
	userID      int64
	deviceToken string
	mu          sync.Mutex
}

type boxRuntimeState struct {
	Online bool
	InUse  bool
}

func (c *wsClient) Send(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.conn.SetWriteDeadline(time.Now().Add(8 * time.Second))
	return c.conn.WriteJSON(v)
}

func (c *wsClient) Ping() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteControl(websocket.PingMessage, []byte("keepalive"), time.Now().Add(8*time.Second))
}

func New(cfg config.Config, st *store.Store, authMgr *auth.Manager, calls *call.Manager, pushSvc *push.Service, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{
		cfg:     cfg,
		store:   st,
		authMgr: authMgr,
		calls:   calls,
		push:    pushSvc,
		log:     log,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				origin := strings.TrimSpace(r.Header.Get("Origin"))
				if origin == "" {
					return true
				}
				u, err := url.Parse(origin)
				if err != nil {
					return false
				}
				return strings.EqualFold(u.Host, r.Host)
			},
		},
		wsClients: make(map[*wsClient]struct{}),
		boxState:  make(map[int64]boxRuntimeState),
	}
}

func (s *Server) Router() *gin.Engine {
	r := gin.Default()
	r.Static("/assets", "./web")
	r.GET("/", func(c *gin.Context) {
		c.File("./web/index.html")
	})
	r.GET("/firebase-messaging-sw.js", s.handleFirebaseMessagingServiceWorker)

	r.POST("/api/login", s.handleLogin)
	r.POST("/api/refresh", s.handleRefresh)
	r.POST("/api/logout", s.handleLogout)
	r.GET("/api/me", s.handleMe)

	authed := r.Group("/api")
	authed.Use(auth.RequireAuth(s.authMgr, s.cfg.Auth.CookieName))
	{
		authed.GET("/fxo", s.handleListFXO)
		authed.PUT("/fxo/:id/notify", s.handleSetFXONotifyPreference)
		authed.GET("/calls", s.handleListCalls)
		authed.GET("/contacts", s.handleListContacts)
		authed.POST("/contacts", s.handleCreateContact)
		authed.DELETE("/contacts/:id", s.handleDeleteContact)
		authed.PUT("/password", s.handleChangeOwnPassword)
		authed.GET("/push/config", s.handlePushConfig)
		authed.POST("/device/push", s.handleRegisterPushToken)
		authed.GET("/incoming", s.handleListIncomingCalls)

		admin := authed.Group("")
		admin.Use(auth.RequireRole(store.RoleAdmin))
		admin.GET("/users", s.handleListUsers)
		admin.POST("/users", s.handleCreateUser)
		admin.DELETE("/users/:id", s.handleDeleteUser)
		admin.PUT("/users/:id/password", s.handleAdminChangeUserPassword)
		admin.POST("/fxo", s.handleCreateFXO)
		admin.PUT("/fxo/:id", s.handleUpdateFXO)
		admin.DELETE("/fxo/:id", s.handleDeleteFXO)
		admin.GET("/fxo-permissions", s.handleListFXOPermissions)
		admin.PUT("/fxo-permissions", s.handleSetFXOPermission)
	}

	r.GET("/ws/signaling", auth.RequireAuth(s.authMgr, s.cfg.Auth.CookieName), s.handleWS)
	return r
}

func (s *Server) StartBackground(ctx context.Context) {
	go s.boxStatusLoop(ctx)
}

func (s *Server) NotifyIncomingEvent(ctx context.Context, event call.IncomingEvent) {
	s.notifyIncomingWS(event)
	go s.notifyIncomingPush(ctx, event)
}

func (s *Server) addWSClient(c *wsClient) {
	s.wsMu.Lock()
	s.wsClients[c] = struct{}{}
	s.wsMu.Unlock()
}

func (s *Server) removeWSClient(c *wsClient) {
	s.wsMu.Lock()
	delete(s.wsClients, c)
	s.wsMu.Unlock()
}

func (s *Server) listWSClients() []*wsClient {
	s.wsMu.RLock()
	defer s.wsMu.RUnlock()
	out := make([]*wsClient, 0, len(s.wsClients))
	for c := range s.wsClients {
		out = append(out, c)
	}
	return out
}

func (s *Server) hasWSClientForDevice(deviceToken string) bool {
	deviceToken = strings.TrimSpace(deviceToken)
	if deviceToken == "" {
		return false
	}
	s.wsMu.RLock()
	defer s.wsMu.RUnlock()
	for c := range s.wsClients {
		if c.deviceToken == deviceToken {
			return true
		}
	}
	return false
}

func (s *Server) sendBoxSnapshot(c *wsClient) {
	items, err := s.store.ListFXOBoxesWithStatus(context.Background())
	if err != nil {
		s.log.Warn("send box snapshot failed", "error", err)
		return
	}
	type boxStatus struct {
		BoxID  int64 `json:"box_id"`
		Online bool  `json:"online"`
		InUse  bool  `json:"in_use"`
	}
	list := make([]boxStatus, 0, len(items))
	for _, b := range items {
		list = append(list, boxStatus{BoxID: b.ID, Online: b.Online, InUse: s.calls.IsBoxInUse(b.ID)})
	}
	_ = c.Send(gin.H{"type": "boxes_snapshot", "items": list})
}

func (s *Server) sendPendingIncoming(c *wsClient) {
	for _, item := range s.calls.ListPendingIncoming(c.userID) {
		_ = c.Send(gin.H{
			"type":          "incoming_call",
			"invite_id":     item.ID,
			"box_id":        item.BoxID,
			"box_name":      item.BoxName,
			"caller_id":     item.CallerID,
			"remote_number": item.RemoteNumber,
			"state":         item.State,
			"expires_at":    item.ExpiresAt,
		})
	}
}

func (s *Server) notifyIncomingWS(event call.IncomingEvent) {
	userSet := make(map[int64]struct{}, len(event.UserIDs))
	for _, id := range event.UserIDs {
		userSet[id] = struct{}{}
	}
	msgType := mapIncomingMessageType(event.Type)
	for _, client := range s.listWSClients() {
		if _, ok := userSet[client.userID]; !ok {
			continue
		}
		payload := gin.H{
			"type":          msgType,
			"invite_id":     event.Call.ID,
			"box_id":        event.Call.BoxID,
			"box_name":      event.Call.BoxName,
			"caller_id":     event.Call.CallerID,
			"remote_number": event.Call.RemoteNumber,
			"reason":        event.Reason,
			"state":         event.Call.State,
		}
		if event.Type == call.IncomingEventAnswered {
			payload["answered_by_user_id"] = event.Call.AnsweredByUserID
			payload["answered_by_device_token"] = event.Call.AnsweredByDeviceToken
		}
		if err := client.Send(payload); err != nil {
			s.removeWSClient(client)
		}
	}
}

func (s *Server) notifyIncomingPush(ctx context.Context, event call.IncomingEvent) {
	if s.push == nil || !s.push.CanSend() || len(event.UserIDs) == 0 {
		return
	}
	devices, err := s.store.ListUserDevicesByUserIDs(ctx, event.UserIDs)
	if err != nil {
		s.log.Warn("list devices for push failed", "error", err)
		return
	}
	pushEvent := mapIncomingPushEvent(event.Type)
	for _, dev := range devices {
		if strings.TrimSpace(dev.PushToken) == "" {
			continue
		}
		if event.Type == call.IncomingEventAnswered && dev.DeviceToken == event.Call.AnsweredByDeviceToken {
			continue
		}
		if s.hasWSClientForDevice(dev.DeviceToken) && strings.EqualFold(dev.ClientType, "web") {
			continue
		}
		data := map[string]string{
			"event":                 pushEvent,
			"invite_id":             event.Call.ID,
			"box_id":                strconv.FormatInt(event.Call.BoxID, 10),
			"box_name":              event.Call.BoxName,
			"caller_id":             event.Call.CallerID,
			"remote_number":         event.Call.RemoteNumber,
			"reason":                event.Reason,
			"answered_by_user_id":   strconv.FormatInt(event.Call.AnsweredByUserID, 10),
			"answered_by_device_id": event.Call.AnsweredByDeviceToken,
		}
		sendCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := s.push.SendData(sendCtx, dev.PushToken, data)
		cancel()
		if err != nil {
			s.log.Warn("push delivery failed", "device", dev.DeviceToken, "error", err)
		}
	}
}

func (s *Server) boxStatusLoop(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	first := true
	for {
		select {
		case <-ticker.C:
			items, err := s.store.ListFXOBoxesWithStatus(context.Background())
			if err != nil {
				s.log.Warn("box status poll failed", "error", err)
				continue
			}
			current := make(map[int64]boxRuntimeState, len(items))
			for _, b := range items {
				current[b.ID] = boxRuntimeState{Online: b.Online, InUse: s.calls.IsBoxInUse(b.ID)}
			}
			s.boxStateMu.Lock()
			if first {
				s.boxState = current
				first = false
				s.boxStateMu.Unlock()
				continue
			}
			changes := make([]struct {
				id     int64
				online bool
				inUse  bool
			}, 0)
			for id, now := range current {
				prev, ok := s.boxState[id]
				if !ok || prev.Online != now.Online || prev.InUse != now.InUse {
					changes = append(changes, struct {
						id     int64
						online bool
						inUse  bool
					}{id: id, online: now.Online, inUse: now.InUse})
				}
			}
			for id := range s.boxState {
				if _, ok := current[id]; !ok {
					changes = append(changes, struct {
						id     int64
						online bool
						inUse  bool
					}{id: id, online: false, inUse: false})
				}
			}
			s.boxState = current
			s.boxStateMu.Unlock()
			for _, ch := range changes {
				payload := gin.H{"type": "box_status", "box_id": ch.id, "online": ch.online, "in_use": ch.inUse}
				for _, client := range s.listWSClients() {
					if err := client.Send(payload); err != nil {
						s.removeWSClient(client)
					}
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

func (s *Server) handleLogin(c *gin.Context) {
	var req struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		DeviceToken string `json:"device_token"`
		ClientType  string `json:"client_type"`
		DeviceName  string `json:"device_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid payload"})
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		c.JSON(400, gin.H{"error": "username/password required"})
		return
	}

	uc, err := s.store.GetUserCredentialByUsername(c.Request.Context(), req.Username)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(401, gin.H{"error": "invalid credentials"})
			return
		}
		s.log.Error("login lookup failed", "error", err)
		c.JSON(500, gin.H{"error": "internal error"})
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(uc.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(401, gin.H{"error": "invalid credentials"})
		return
	}

	sess, refreshToken, deviceToken, refreshExpiresAt, err := s.issueDeviceSession(c.Request.Context(), uc, req.DeviceToken, req.ClientType, req.DeviceName)
	if err != nil {
		s.log.Error("issue device session failed", "error", err)
		c.JSON(500, gin.H{"error": "internal error"})
		return
	}
	setTokenCookie(c, s.cfg.Auth.CookieName, sess.Token, sess.ExpiresAt)
	setTokenCookie(c, auth.RefreshCookieName(s.cfg.Auth.CookieName), refreshToken, refreshExpiresAt)
	c.JSON(200, gin.H{
		"user":               gin.H{"id": uc.ID, "username": uc.Username, "role": uc.Role},
		"access_token":       sess.Token,
		"refresh_token":      refreshToken,
		"access_expires_at":  sess.ExpiresAt,
		"refresh_expires_at": refreshExpiresAt,
		"device_token":       deviceToken,
		"push_config":        s.push.PublicConfig(),
	})
}

func (s *Server) handleRefresh(c *gin.Context) {
	var req struct {
		DeviceToken  string `json:"device_token"`
		RefreshToken string `json:"refresh_token"`
		ClientType   string `json:"client_type"`
		DeviceName   string `json:"device_name"`
	}
	_ = c.ShouldBindJSON(&req)

	deviceToken := normalizeDeviceToken(req.DeviceToken)
	if deviceToken == "" {
		c.JSON(400, gin.H{"error": "device_token required"})
		return
	}
	refreshToken := strings.TrimSpace(req.RefreshToken)
	if refreshToken == "" {
		cookieToken, err := c.Cookie(auth.RefreshCookieName(s.cfg.Auth.CookieName))
		if err == nil {
			refreshToken = strings.TrimSpace(cookieToken)
		}
	}
	if refreshToken == "" {
		clearTokenCookies(c, s.cfg.Auth.CookieName)
		c.JSON(401, gin.H{"error": "refresh token required"})
		return
	}

	dev, err := s.store.GetUserDeviceByToken(c.Request.Context(), deviceToken)
	if err != nil {
		clearTokenCookies(c, s.cfg.Auth.CookieName)
		c.JSON(401, gin.H{"error": "invalid refresh session"})
		return
	}
	if dev.RefreshTokenHash == "" || dev.RefreshExpiresAt.Before(time.Now()) || !strings.EqualFold(dev.RefreshTokenHash, hashToken(refreshToken)) {
		clearTokenCookies(c, s.cfg.Auth.CookieName)
		c.JSON(401, gin.H{"error": "invalid refresh session"})
		return
	}
	uc, err := s.store.GetUserCredentialByID(c.Request.Context(), dev.UserID)
	if err != nil {
		clearTokenCookies(c, s.cfg.Auth.CookieName)
		c.JSON(401, gin.H{"error": "invalid refresh session"})
		return
	}

	sess, newRefreshToken, _, refreshExpiresAt, err := s.issueDeviceSession(c.Request.Context(), uc, dev.DeviceToken, firstNonBlank(req.ClientType, dev.ClientType), firstNonBlank(req.DeviceName, dev.DeviceName))
	if err != nil {
		s.log.Error("refresh session failed", "error", err)
		c.JSON(500, gin.H{"error": "internal error"})
		return
	}
	setTokenCookie(c, s.cfg.Auth.CookieName, sess.Token, sess.ExpiresAt)
	setTokenCookie(c, auth.RefreshCookieName(s.cfg.Auth.CookieName), newRefreshToken, refreshExpiresAt)
	c.JSON(200, gin.H{
		"user":               gin.H{"id": uc.ID, "username": uc.Username, "role": uc.Role},
		"access_token":       sess.Token,
		"refresh_token":      newRefreshToken,
		"access_expires_at":  sess.ExpiresAt,
		"refresh_expires_at": refreshExpiresAt,
		"device_token":       dev.DeviceToken,
		"push_config":        s.push.PublicConfig(),
	})
}

func (s *Server) handleLogout(c *gin.Context) {
	if sess, ok := s.sessionFromRequest(c); ok {
		_ = s.store.ClearUserDeviceAuth(c.Request.Context(), sess.UserID, sess.DeviceToken)
	}
	clearTokenCookies(c, s.cfg.Auth.CookieName)
	c.JSON(200, gin.H{"ok": true})
}

func (s *Server) handleMe(c *gin.Context) {
	sess, ok := s.sessionFromRequest(c)
	if !ok {
		c.JSON(200, gin.H{"authenticated": false})
		return
	}
	c.JSON(200, gin.H{
		"authenticated": true,
		"user":          gin.H{"id": sess.UserID, "username": sess.Username, "role": sess.Role},
		"device_token":  sess.DeviceToken,
	})
}

func (s *Server) handlePushConfig(c *gin.Context) {
	c.JSON(200, gin.H{"item": s.push.PublicConfig()})
}

func (s *Server) handleRegisterPushToken(c *gin.Context) {
	authSess, ok := auth.SessionFromContext(c)
	if !ok {
		c.JSON(401, gin.H{"error": "unauthorized"})
		return
	}
	var req struct {
		PushPlatform string `json:"push_platform"`
		PushToken    string `json:"push_token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid payload"})
		return
	}
	if err := s.store.UpdateUserDevicePush(c.Request.Context(), authSess.UserID, authSess.DeviceToken, req.PushPlatform, req.PushToken); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(404, gin.H{"error": "device not found"})
			return
		}
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func (s *Server) handleListIncomingCalls(c *gin.Context) {
	authSess, ok := auth.SessionFromContext(c)
	if !ok {
		c.JSON(401, gin.H{"error": "unauthorized"})
		return
	}
	c.JSON(200, gin.H{"items": s.calls.ListPendingIncoming(authSess.UserID)})
}

func (s *Server) handleListUsers(c *gin.Context) {
	users, err := s.store.ListUsers(c.Request.Context())
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"items": users})
}

func (s *Server) handleCreateUser(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid payload"})
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		c.JSON(400, gin.H{"error": "username/password required"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(500, gin.H{"error": "password hash error"})
		return
	}
	role := strings.ToLower(strings.TrimSpace(req.Role))
	if role == "" {
		role = store.RoleUser
	}
	if role != store.RoleUser && role != store.RoleAdmin {
		c.JSON(400, gin.H{"error": "role must be admin or user"})
		return
	}
	user, err := s.store.CreateUser(c.Request.Context(), req.Username, string(hash), role)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.JSON(201, gin.H{"item": user})
}

func (s *Server) handleDeleteUser(c *gin.Context) {
	id, err := parseIDParam(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid id"})
		return
	}
	if err := s.store.DeleteUser(c.Request.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(404, gin.H{"error": "user not found"})
			return
		}
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	_ = s.store.ClearUserDeviceAuthByUserID(c.Request.Context(), id)
	c.JSON(200, gin.H{"ok": true})
}

func (s *Server) handleChangeOwnPassword(c *gin.Context) {
	authSess, ok := auth.SessionFromContext(c)
	if !ok {
		c.JSON(401, gin.H{"error": "unauthorized"})
		return
	}
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid payload"})
		return
	}
	if req.OldPassword == "" || req.NewPassword == "" {
		c.JSON(400, gin.H{"error": "old_password/new_password required"})
		return
	}
	uc, err := s.store.GetUserCredentialByID(c.Request.Context(), authSess.UserID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(404, gin.H{"error": "user not found"})
			return
		}
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(uc.PasswordHash), []byte(req.OldPassword)); err != nil {
		c.JSON(401, gin.H{"error": "invalid old password"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(500, gin.H{"error": "password hash error"})
		return
	}
	if err := s.store.UpdateUserPasswordHashByID(c.Request.Context(), authSess.UserID, string(hash)); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	_ = s.store.ClearUserDeviceAuthByUserID(c.Request.Context(), authSess.UserID)
	clearTokenCookies(c, s.cfg.Auth.CookieName)
	c.JSON(200, gin.H{"ok": true, "relogin": true})
}

func (s *Server) handleAdminChangeUserPassword(c *gin.Context) {
	id, err := parseIDParam(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid id"})
		return
	}
	var req struct {
		NewPassword string `json:"new_password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid payload"})
		return
	}
	if req.NewPassword == "" {
		c.JSON(400, gin.H{"error": "new_password required"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(500, gin.H{"error": "password hash error"})
		return
	}
	if err := s.store.UpdateUserPasswordHashByID(c.Request.Context(), id, string(hash)); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(404, gin.H{"error": "user not found"})
			return
		}
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	_ = s.store.ClearUserDeviceAuthByUserID(c.Request.Context(), id)
	c.JSON(200, gin.H{"ok": true})
}

func (s *Server) handleListFXO(c *gin.Context) {
	authSess, ok := auth.SessionFromContext(c)
	if !ok {
		c.JSON(401, gin.H{"error": "unauthorized"})
		return
	}
	items, err := s.store.ListFXOBoxesWithStatusForUser(c.Request.Context(), authSess.UserID, strings.EqualFold(authSess.Role, store.RoleAdmin))
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	for i := range items {
		items[i].SIPPassword = ""
		items[i].InUse = s.calls.IsBoxInUse(items[i].ID)
	}
	c.JSON(200, gin.H{"items": items})
}

func (s *Server) handleListCalls(c *gin.Context) {
	authSess, ok := auth.SessionFromContext(c)
	if !ok {
		c.JSON(401, gin.H{"error": "unauthorized"})
		return
	}
	page := 1
	pageSize := 10
	if raw := strings.TrimSpace(c.Query("page")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v <= 0 {
			c.JSON(400, gin.H{"error": "invalid page"})
			return
		}
		page = v
	}
	if raw := strings.TrimSpace(c.Query("page_size")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v <= 0 {
			c.JSON(400, gin.H{"error": "invalid page_size"})
			return
		}
		pageSize = v
	}
	if pageSize > 100 {
		pageSize = 100
	}
	items, total, err := s.store.ListCallLogsByUser(c.Request.Context(), authSess.UserID, page, pageSize)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	totalPages := 0
	if total > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}
	c.JSON(200, gin.H{
		"items":       items,
		"page":        page,
		"page_size":   pageSize,
		"total":       total,
		"total_pages": totalPages,
	})
}

func (s *Server) handleListContacts(c *gin.Context) {
	authSess, ok := auth.SessionFromContext(c)
	if !ok {
		c.JSON(401, gin.H{"error": "unauthorized"})
		return
	}
	limit := 500
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v <= 0 {
			c.JSON(400, gin.H{"error": "invalid limit"})
			return
		}
		limit = v
	}
	q := strings.TrimSpace(c.Query("q"))
	items, err := s.store.ListContacts(c.Request.Context(), authSess.UserID, q, limit)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"items": items})
}

func (s *Server) handleCreateContact(c *gin.Context) {
	authSess, ok := auth.SessionFromContext(c)
	if !ok {
		c.JSON(401, gin.H{"error": "unauthorized"})
		return
	}
	var req struct {
		Name   string `json:"name"`
		Number string `json:"number"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid payload"})
		return
	}
	name := strings.TrimSpace(req.Name)
	number := strings.TrimSpace(req.Number)
	if name == "" || number == "" {
		c.JSON(400, gin.H{"error": "name/number required"})
		return
	}
	item, err := s.store.CreateContact(c.Request.Context(), authSess.UserID, name, number)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(201, gin.H{"item": item})
}

func (s *Server) handleDeleteContact(c *gin.Context) {
	authSess, ok := auth.SessionFromContext(c)
	if !ok {
		c.JSON(401, gin.H{"error": "unauthorized"})
		return
	}
	id, err := parseIDParam(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid id"})
		return
	}
	if err := s.store.DeleteContact(c.Request.Context(), authSess.UserID, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(404, gin.H{"error": "contact not found"})
			return
		}
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func (s *Server) handleCreateFXO(c *gin.Context) {
	var req struct {
		Name        string `json:"name"`
		SIPUsername string `json:"sip_username"`
		SIPPassword string `json:"sip_password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid payload"})
		return
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.SIPUsername) == "" || strings.TrimSpace(req.SIPPassword) == "" {
		c.JSON(400, gin.H{"error": "name/sip_username/sip_password required"})
		return
	}
	box, err := s.store.CreateFXOBox(c.Request.Context(), strings.TrimSpace(req.Name), strings.TrimSpace(req.SIPUsername), req.SIPPassword)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	box.SIPPassword = ""
	c.JSON(201, gin.H{"item": box})
}

func (s *Server) handleUpdateFXO(c *gin.Context) {
	id, err := parseIDParam(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid id"})
		return
	}
	var req struct {
		Name        string `json:"name"`
		SIPUsername string `json:"sip_username"`
		SIPPassword string `json:"sip_password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid payload"})
		return
	}
	oldBox, err := s.store.GetFXOBoxByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(404, gin.H{"error": "fxo box not found"})
			return
		}
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = oldBox.Name
	}
	user := strings.TrimSpace(req.SIPUsername)
	if user == "" {
		user = oldBox.SIPUsername
	}
	pass := req.SIPPassword
	if strings.TrimSpace(pass) == "" {
		pass = oldBox.SIPPassword
	}
	box, err := s.store.UpdateFXOBox(c.Request.Context(), id, name, user, pass)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(404, gin.H{"error": "fxo box not found"})
			return
		}
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	box.SIPPassword = ""
	c.JSON(200, gin.H{"item": box})
}

func (s *Server) handleDeleteFXO(c *gin.Context) {
	id, err := parseIDParam(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid id"})
		return
	}
	if err := s.store.DeleteFXOBox(c.Request.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(404, gin.H{"error": "fxo box not found"})
			return
		}
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func (s *Server) handleListFXOPermissions(c *gin.Context) {
	allUsers, err := s.store.ListUsers(c.Request.Context())
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	users := make([]store.User, 0, len(allUsers))
	for _, u := range allUsers {
		if !strings.EqualFold(u.Role, store.RoleAdmin) {
			users = append(users, u)
		}
	}
	boxes, err := s.store.ListFXOBoxesWithStatus(c.Request.Context())
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	items, err := s.store.ListUserFXOPermissions(c.Request.Context())
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	for i := range boxes {
		boxes[i].SIPPassword = ""
	}
	c.JSON(200, gin.H{"users": users, "boxes": boxes, "items": items})
}

func (s *Server) handleSetFXONotifyPreference(c *gin.Context) {
	authSess, ok := auth.SessionFromContext(c)
	if !ok {
		c.JSON(401, gin.H{"error": "unauthorized"})
		return
	}
	id, err := parseIDParam(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid id"})
		return
	}
	var req struct {
		Notify bool `json:"notify"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid payload"})
		return
	}
	if err := s.store.SetUserNotifyPreference(c.Request.Context(), authSess.UserID, id, req.Notify); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func (s *Server) handleSetFXOPermission(c *gin.Context) {
	var req struct {
		UserID     int64 `json:"user_id"`
		BoxID      int64 `json:"box_id"`
		CanDial    bool  `json:"can_dial"`
		CanReceive bool  `json:"can_receive"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid payload"})
		return
	}
	if req.UserID <= 0 || req.BoxID <= 0 {
		c.JSON(400, gin.H{"error": "user_id/box_id required"})
		return
	}
	if _, err := s.store.GetUserCredentialByID(c.Request.Context(), req.UserID); err != nil {
		c.JSON(404, gin.H{"error": "user not found"})
		return
	}
	if _, err := s.store.GetFXOBoxByID(c.Request.Context(), req.BoxID); err != nil {
		c.JSON(404, gin.H{"error": "fxo box not found"})
		return
	}
	if err := s.store.SetUserFXOPermission(c.Request.Context(), req.UserID, req.BoxID, req.CanDial, req.CanReceive); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func (s *Server) handleWS(c *gin.Context) {
	authSess, ok := auth.SessionFromContext(c)
	if !ok {
		c.JSON(401, gin.H{"error": "unauthorized"})
		return
	}
	conn, err := s.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		s.log.Warn("websocket upgrade failed", "error", err)
		return
	}
	defer conn.Close()

	client := &wsClient{conn: conn, userID: authSess.UserID, deviceToken: authSess.DeviceToken}
	s.addWSClient(client)
	defer s.removeWSClient(client)
	s.sendBoxSnapshot(client)
	s.sendPendingIncoming(client)

	const (
		wsPongWait   = 180 * time.Second
		wsPingPeriod = 45 * time.Second
	)
	_ = conn.SetReadDeadline(time.Now().Add(wsPongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(wsPongWait))
	})

	stopPing := make(chan struct{})
	defer close(stopPing)
	go func() {
		ticker := time.NewTicker(wsPingPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := client.Ping(); err != nil {
					_ = conn.Close()
					return
				}
			case <-stopPing:
				return
			}
		}
	}()

	var current *call.CallSession
	var curMu sync.Mutex
	setCurrent := func(cs *call.CallSession) {
		curMu.Lock()
		current = cs
		curMu.Unlock()
	}
	getCurrent := func() *call.CallSession {
		curMu.Lock()
		defer curMu.Unlock()
		return current
	}
	sendErr := func(msg string) {
		_ = client.Send(gin.H{"type": "error", "error": msg})
	}
	if cs := s.calls.GetUserCall(authSess.UserID); cs != nil {
		setCurrent(cs)
		_ = client.Send(gin.H{"type": "state", "state": "call_recovered"})
	}

	for {
		var msg wsMessage
		if err := conn.ReadJSON(&msg); err != nil {
			break
		}
		switch msg.Type {
		case "dial":
			if getCurrent() != nil || s.calls.GetUserCall(authSess.UserID) != nil {
				sendErr("a call is already active")
				continue
			}
			if msg.BoxID == 0 || strings.TrimSpace(msg.Number) == "" || strings.TrimSpace(msg.SDP) == "" {
				sendErr("box_id/number/sdp are required")
				continue
			}
			allowed, err := s.store.UserCanDialFXO(c.Request.Context(), authSess.UserID, msg.BoxID, strings.EqualFold(authSess.Role, store.RoleAdmin))
			if err != nil {
				sendErr(err.Error())
				continue
			}
			if !allowed {
				sendErr("fxo box not permitted")
				continue
			}
			_ = client.Send(gin.H{"type": "state", "state": "dialing"})
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			sess, answer, err := s.calls.StartCall(ctx, authSess.UserID, authSess.DeviceToken, msg.BoxID, msg.Number, msg.SDP, signalCallbacksForClient(client, setCurrent))
			cancel()
			if err != nil {
				sendErr(err.Error())
				_ = client.Send(gin.H{"type": "state", "state": "idle"})
				continue
			}
			setCurrent(sess)
			_ = client.Send(gin.H{"type": "answer", "sdp": answer, "mode": "outgoing"})

		case "incoming_accept":
			if strings.TrimSpace(msg.InviteID) == "" || strings.TrimSpace(msg.SDP) == "" {
				sendErr("invite_id/sdp are required")
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			sess, answer, err := s.calls.AcceptIncoming(ctx, authSess.UserID, authSess.DeviceToken, msg.InviteID, msg.SDP, signalCallbacksForClient(client, setCurrent))
			cancel()
			if err != nil {
				sendErr(err.Error())
				continue
			}
			setCurrent(sess)
			_ = client.Send(gin.H{"type": "answer", "sdp": answer, "mode": "incoming", "invite_id": msg.InviteID})

		case "incoming_reject":
			if strings.TrimSpace(msg.InviteID) == "" {
				sendErr("invite_id required")
				continue
			}
			if err := s.calls.RejectIncoming(context.Background(), authSess.UserID, msg.InviteID, "declined"); err != nil {
				sendErr(err.Error())
			}

		case "candidate":
			cs := getCurrent()
			if cs == nil || msg.Candidate == nil {
				continue
			}
			if err := cs.AddICECandidate(*msg.Candidate); err != nil {
				sendErr(err.Error())
			}

		case "dtmf":
			cs := getCurrent()
			if cs == nil {
				sendErr("no active call")
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			err := cs.SendDTMF(ctx, strings.TrimSpace(msg.Digits))
			cancel()
			if err != nil {
				sendErr(err.Error())
			}

		case "hangup":
			if cs := getCurrent(); cs != nil {
				setCurrent(nil)
				cs.Hangup("user hangup")
			}

		case "ping":
			_ = client.Send(gin.H{"type": "pong"})

		default:
			sendErr("unsupported websocket message type")
		}
	}
}

func signalCallbacksForClient(client *wsClient, setCurrent func(*call.CallSession)) call.SignalCallbacks {
	return call.SignalCallbacks{
		OnICECandidate: func(candidate webrtc.ICECandidateInit) {
			_ = client.Send(gin.H{"type": "candidate", "candidate": candidate})
		},
		OnState: func(state, detail string) {
			_ = client.Send(gin.H{"type": "state", "state": state, "detail": detail})
		},
		OnHangup: func(reason string) {
			setCurrent(nil)
			_ = client.Send(gin.H{"type": "hangup", "reason": reason})
		},
	}
}

func (s *Server) handleFirebaseMessagingServiceWorker(c *gin.Context) {
	cfg := s.push.PublicConfig()
	c.Header("Content-Type", "application/javascript; charset=utf-8")
	if !cfg.Enabled {
		c.String(200, "self.addEventListener('install',()=>self.skipWaiting());self.addEventListener('activate',()=>self.clients.claim());")
		return
	}
	cfgJSON, _ := json.Marshal(cfg)
	script := fmt.Sprintf(`importScripts('https://www.gstatic.com/firebasejs/10.13.2/firebase-app-compat.js');
importScripts('https://www.gstatic.com/firebasejs/10.13.2/firebase-messaging-compat.js');
self.addEventListener('install',()=>self.skipWaiting());
self.addEventListener('activate',()=>self.clients.claim());
const firebaseConfig=%s;
firebase.initializeApp({
  apiKey: firebaseConfig.api_key,
  appId: firebaseConfig.app_id,
  projectId: firebaseConfig.project_id,
  messagingSenderId: firebaseConfig.messaging_sender_id,
  authDomain: firebaseConfig.auth_domain || undefined,
  storageBucket: firebaseConfig.storage_bucket || undefined,
  measurementId: firebaseConfig.measurement_id || undefined,
});
const messaging=firebase.messaging();
messaging.onBackgroundMessage((payload)=>{
  const data=(payload&&payload.data)||{};
  const title=data.caller_id||data.remote_number||'Incoming call';
  const body=data.box_name ? ('FXO: '+data.box_name) : 'Tap to open CallFXO';
  self.registration.showNotification(title,{body,data});
});
self.addEventListener('notificationclick',(event)=>{
  event.notification.close();
  const data=event.notification.data||{};
  const url='/?incoming=' + encodeURIComponent(data.invite_id || '');
  event.waitUntil(clients.matchAll({type:'window',includeUncontrolled:true}).then((items)=>{
    for (const client of items) {
      if ('focus' in client) {
        client.navigate(url);
        return client.focus();
      }
    }
    return clients.openWindow(url);
  }));
});`, string(cfgJSON))
	c.String(200, script)
}

func (s *Server) issueDeviceSession(ctx context.Context, uc store.UserCredential, rawDeviceToken, rawClientType, rawDeviceName string) (auth.Session, string, string, time.Time, error) {
	deviceToken := normalizeDeviceToken(rawDeviceToken)
	if deviceToken == "" {
		var err error
		deviceToken, err = randomToken(24)
		if err != nil {
			return auth.Session{}, "", "", time.Time{}, err
		}
	}
	clientType := normalizeClientType(rawClientType)
	deviceName := strings.TrimSpace(rawDeviceName)
	if deviceName == "" {
		deviceName = clientType
	}
	refreshToken, err := randomToken(32)
	if err != nil {
		return auth.Session{}, "", "", time.Time{}, err
	}
	sess, err := s.authMgr.Create(uc.ID, uc.Username, uc.Role, deviceToken)
	if err != nil {
		return auth.Session{}, "", "", time.Time{}, err
	}
	refreshExpiresAt := time.Now().Add(time.Duration(s.cfg.Auth.RefreshTTLHours) * time.Hour)
	dev := store.UserDevice{
		DeviceToken:      deviceToken,
		UserID:           uc.ID,
		ClientType:       clientType,
		DeviceName:       deviceName,
		RefreshTokenHash: hashToken(refreshToken),
		RefreshExpiresAt: refreshExpiresAt,
	}
	if err := s.store.UpsertUserDevice(ctx, dev); err != nil {
		return auth.Session{}, "", "", time.Time{}, err
	}
	return sess, refreshToken, deviceToken, refreshExpiresAt, nil
}

func (s *Server) sessionFromRequest(c *gin.Context) (auth.Session, bool) {
	token := auth.ExtractToken(c, s.cfg.Auth.CookieName)
	return s.authMgr.Validate(token)
}

func parseIDParam(raw string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid id")
	}
	return id, nil
}

func setTokenCookie(c *gin.Context, name, token string, expiresAt time.Time) {
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 0 {
		maxAge = 0
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   isSecureRequest(c),
		MaxAge:   maxAge,
		Expires:  expiresAt,
	})
}

func clearTokenCookies(c *gin.Context, accessCookie string) {
	clearCookie(c, accessCookie)
	clearCookie(c, auth.RefreshCookieName(accessCookie))
}

func clearCookie(c *gin.Context, name string) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   isSecureRequest(c),
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
}

func isSecureRequest(c *gin.Context) bool {
	if c != nil && c.Request != nil && c.Request.TLS != nil {
		return true
	}
	proto := strings.ToLower(strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")))
	return proto == "https"
}

func hashToken(v string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(v)))
	return hex.EncodeToString(sum[:])
}

func normalizeDeviceToken(v string) string {
	return strings.TrimSpace(v)
}

func normalizeClientType(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "android":
		return "android"
	default:
		return "web"
	}
}

func randomToken(length int) (string, error) {
	if length <= 0 {
		length = 16
	}
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func mapIncomingMessageType(eventType string) string {
	switch eventType {
	case call.IncomingEventAnswered:
		return "incoming_answered"
	case call.IncomingEventStopped:
		return "incoming_stop"
	default:
		return "incoming_call"
	}
}

func mapIncomingPushEvent(eventType string) string {
	switch eventType {
	case call.IncomingEventAnswered:
		return "incoming_answered"
	case call.IncomingEventStopped:
		return "incoming_stop"
	default:
		return "incoming_call"
	}
}

func firstNonBlank(values ...string) string {
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
