package web

import (
	"context"
	"errors"
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
	"github.com/pccr10001/callfxo/internal/store"
)

type Server struct {
	cfg     config.Config
	store   *store.Store
	authMgr *auth.Manager
	calls   *call.Manager
	log     *slog.Logger

	upgrader websocket.Upgrader

	wsMu      sync.RWMutex
	wsClients map[*wsClient]struct{}

	boxStateMu sync.Mutex
	boxOnline  map[int64]bool

	callMu    sync.RWMutex
	userCalls map[int64]*call.CallSession
}

type wsMessage struct {
	Type      string                   `json:"type"`
	BoxID     int64                    `json:"box_id,omitempty"`
	Number    string                   `json:"number,omitempty"`
	SDP       string                   `json:"sdp,omitempty"`
	Digits    string                   `json:"digits,omitempty"`
	Candidate *webrtc.ICECandidateInit `json:"candidate,omitempty"`
}

type wsClient struct {
	conn *websocket.Conn
	mu   sync.Mutex
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

func (s *Server) broadcastWS(v any) {
	s.wsMu.RLock()
	clients := make([]*wsClient, 0, len(s.wsClients))
	for c := range s.wsClients {
		clients = append(clients, c)
	}
	s.wsMu.RUnlock()

	for _, c := range clients {
		if err := c.Send(v); err != nil {
			s.removeWSClient(c)
		}
	}
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
	}
	list := make([]boxStatus, 0, len(items))
	for _, b := range items {
		list = append(list, boxStatus{BoxID: b.ID, Online: b.Online})
	}
	_ = c.Send(gin.H{"type": "boxes_snapshot", "items": list})
}

func (s *Server) setUserCall(userID int64, cs *call.CallSession) {
	s.callMu.Lock()
	if cs == nil {
		delete(s.userCalls, userID)
	} else {
		s.userCalls[userID] = cs
	}
	s.callMu.Unlock()
}

func (s *Server) getUserCall(userID int64) *call.CallSession {
	s.callMu.RLock()
	cs := s.userCalls[userID]
	s.callMu.RUnlock()
	return cs
}

func New(cfg config.Config, st *store.Store, authMgr *auth.Manager, calls *call.Manager, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{
		cfg:     cfg,
		store:   st,
		authMgr: authMgr,
		calls:   calls,
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
		boxOnline: make(map[int64]bool),
		userCalls: make(map[int64]*call.CallSession),
	}
}

func (s *Server) Router() *gin.Engine {
	r := gin.Default()
	r.Static("/assets", "./web")
	r.GET("/", func(c *gin.Context) {
		c.File("./web/index.html")
	})

	r.POST("/api/login", s.handleLogin)
	r.POST("/api/logout", s.handleLogout)
	r.GET("/api/me", s.handleMe)

	authed := r.Group("/api")
	authed.Use(auth.RequireAuth(s.authMgr, s.cfg.Auth.CookieName))
	{
		authed.GET("/fxo", s.handleListFXO)
		authed.GET("/calls", s.handleListCalls)
		authed.GET("/contacts", s.handleListContacts)
		authed.POST("/contacts", s.handleCreateContact)
		authed.DELETE("/contacts/:id", s.handleDeleteContact)
		authed.PUT("/password", s.handleChangeOwnPassword)
		admin := authed.Group("")
		admin.Use(auth.RequireRole(store.RoleAdmin))
		admin.GET("/users", s.handleListUsers)
		admin.POST("/users", s.handleCreateUser)
		admin.DELETE("/users/:id", s.handleDeleteUser)
		admin.PUT("/users/:id/password", s.handleAdminChangeUserPassword)

		admin.POST("/fxo", s.handleCreateFXO)
		admin.PUT("/fxo/:id", s.handleUpdateFXO)
		admin.DELETE("/fxo/:id", s.handleDeleteFXO)
	}

	r.GET("/ws/signaling", auth.RequireAuth(s.authMgr, s.cfg.Auth.CookieName), s.handleWS)
	return r
}

func (s *Server) StartBackground(ctx context.Context) {
	go s.boxStatusLoop(ctx)
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

			current := make(map[int64]bool, len(items))
			for _, b := range items {
				current[b.ID] = b.Online
			}

			s.boxStateMu.Lock()
			if first {
				s.boxOnline = current
				first = false
				s.boxStateMu.Unlock()
				continue
			}

			changes := make([]struct {
				id     int64
				online bool
			}, 0)
			for id, online := range current {
				prev, ok := s.boxOnline[id]
				if !ok || prev != online {
					changes = append(changes, struct {
						id     int64
						online bool
					}{id: id, online: online})
				}
			}
			for id := range s.boxOnline {
				if _, ok := current[id]; !ok {
					changes = append(changes, struct {
						id     int64
						online bool
					}{id: id, online: false})
				}
			}
			s.boxOnline = current
			s.boxStateMu.Unlock()

			for _, ch := range changes {
				s.broadcastWS(gin.H{
					"type":   "box_status",
					"box_id": ch.id,
					"online": ch.online,
				})
			}

		case <-ctx.Done():
			return
		}
	}
}

func (s *Server) handleLogin(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
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

	sess, err := s.authMgr.Create(uc.ID, uc.Username, uc.Role)
	if err != nil {
		s.log.Error("create session failed", "error", err)
		c.JSON(500, gin.H{"error": "internal error"})
		return
	}

	setSessionCookie(c, s.cfg.Auth.CookieName, sess.Token, sess.ExpiresAt)
	c.JSON(200, gin.H{"user": gin.H{"id": uc.ID, "username": uc.Username, "role": uc.Role}})
}

func (s *Server) handleLogout(c *gin.Context) {
	token, err := c.Cookie(s.cfg.Auth.CookieName)
	if err == nil {
		s.authMgr.Delete(token)
	}
	clearSessionCookie(c, s.cfg.Auth.CookieName)
	c.JSON(200, gin.H{"ok": true})
}

func (s *Server) handleMe(c *gin.Context) {
	token, err := c.Cookie(s.cfg.Auth.CookieName)
	if err != nil {
		c.JSON(200, gin.H{"authenticated": false})
		return
	}
	sess, ok := s.authMgr.Validate(token)
	if !ok {
		c.JSON(200, gin.H{"authenticated": false})
		return
	}
	c.JSON(200, gin.H{"authenticated": true, "user": gin.H{"id": sess.UserID, "username": sess.Username, "role": sess.Role}})
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
	s.authMgr.DeleteByUser(id)
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
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(404, gin.H{"error": "user not found"})
			return
		}
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	s.authMgr.DeleteByUser(authSess.UserID)
	clearSessionCookie(c, s.cfg.Auth.CookieName)
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
	s.authMgr.DeleteByUser(id)
	c.JSON(200, gin.H{"ok": true})
}

func (s *Server) handleListFXO(c *gin.Context) {
	items, err := s.store.ListFXOBoxesWithStatus(c.Request.Context())
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	for i := range items {
		items[i].SIPPassword = ""
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

func (s *Server) handleWS(c *gin.Context) {
	authSess, ok := auth.SessionFromContext(c)
	if !ok {
		c.JSON(401, gin.H{"error": "unauthorized"})
		return
	}
	userID := authSess.UserID

	conn, err := s.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		s.log.Warn("websocket upgrade failed", "error", err)
		return
	}
	defer conn.Close()

	client := &wsClient{conn: conn}
	s.addWSClient(client)
	defer s.removeWSClient(client)
	s.sendBoxSnapshot(client)

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

	sendErr := func(msg string) {
		_ = client.Send(gin.H{"type": "error", "error": msg})
	}

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
	if cs := s.getUserCall(userID); cs != nil {
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
			if getCurrent() != nil || s.getUserCall(userID) != nil {
				sendErr("a call is already active")
				continue
			}
			if msg.BoxID == 0 || strings.TrimSpace(msg.Number) == "" || strings.TrimSpace(msg.SDP) == "" {
				sendErr("box_id/number/sdp are required")
				continue
			}
			_ = client.Send(gin.H{"type": "state", "state": "dialing"})
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			sess, answer, err := s.calls.StartCall(ctx, userID, msg.BoxID, msg.Number, msg.SDP, call.SignalCallbacks{
				OnICECandidate: func(candidate webrtc.ICECandidateInit) {
					_ = client.Send(gin.H{"type": "candidate", "candidate": candidate})
				},
				OnState: func(state, detail string) {
					_ = client.Send(gin.H{"type": "state", "state": state, "detail": detail})
				},
				OnHangup: func(reason string) {
					s.setUserCall(userID, nil)
					setCurrent(nil)
					_ = client.Send(gin.H{"type": "hangup", "reason": reason})
				},
			})
			cancel()
			if err != nil {
				sendErr(err.Error())
				_ = client.Send(gin.H{"type": "state", "state": "idle"})
				continue
			}
			setCurrent(sess)
			s.setUserCall(userID, sess)
			_ = client.Send(gin.H{"type": "answer", "sdp": answer})

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
			cs := getCurrent()
			if cs != nil {
				setCurrent(nil)
				s.setUserCall(userID, nil)
				cs.Hangup("user hangup")
			}

		case "ping":
			_ = client.Send(gin.H{"type": "pong"})

		default:
			sendErr("unsupported websocket message type")
		}
	}
}

func parseIDParam(raw string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid id")
	}
	return id, nil
}

func setSessionCookie(c *gin.Context, name, token string, expiresAt time.Time) {
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 0 {
		maxAge = 0
	}
	secure := isSecureRequest(c)
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   secure,
		MaxAge:   maxAge,
		Expires:  expiresAt,
	})
}

func clearSessionCookie(c *gin.Context, name string) {
	secure := isSecureRequest(c)
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   secure,
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
