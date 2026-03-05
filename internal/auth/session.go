package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const ContextSessionKey = "auth_session"

type Session struct {
	Token     string    `json:"-"`
	UserID    int64     `json:"user_id"`
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Manager struct {
	secret []byte
	ttl    time.Duration

	mu       sync.RWMutex
	sessions map[string]Session
}

func NewManager(secret string, ttl time.Duration) *Manager {
	return &Manager{
		secret:   []byte(secret),
		ttl:      ttl,
		sessions: make(map[string]Session),
	}
}

func (m *Manager) StartCleanup(stop <-chan struct{}) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.cleanupExpired()
		case <-stop:
			return
		}
	}
}

func (m *Manager) Create(userID int64, username, role string) (Session, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return Session{}, fmt.Errorf("generate session bytes: %w", err)
	}

	mac := hmac.New(sha256.New, m.secret)
	mac.Write(raw)
	tok := base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	s := Session{
		Token:     tok,
		UserID:    userID,
		Username:  username,
		Role:      strings.ToLower(strings.TrimSpace(role)),
		ExpiresAt: time.Now().Add(m.ttl),
	}

	m.mu.Lock()
	m.sessions[tok] = s
	m.mu.Unlock()
	return s, nil
}

func (m *Manager) Validate(token string) (Session, bool) {
	if token == "" {
		return Session{}, false
	}
	m.mu.RLock()
	s, ok := m.sessions[token]
	m.mu.RUnlock()
	if !ok {
		return Session{}, false
	}
	if time.Now().After(s.ExpiresAt) {
		m.Delete(token)
		return Session{}, false
	}
	return s, true
}

func (m *Manager) Delete(token string) {
	m.mu.Lock()
	delete(m.sessions, token)
	m.mu.Unlock()
}

func (m *Manager) DeleteByUser(userID int64) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	removed := 0
	for tok, s := range m.sessions {
		if s.UserID == userID {
			delete(m.sessions, tok)
			removed++
		}
	}
	return removed
}

func (m *Manager) cleanupExpired() {
	now := time.Now()
	m.mu.Lock()
	for k, s := range m.sessions {
		if now.After(s.ExpiresAt) {
			delete(m.sessions, k)
		}
	}
	m.mu.Unlock()
}

func RequireAuth(mgr *Manager, cookieName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie(cookieName)
		if err != nil {
			c.AbortWithStatusJSON(401, gin.H{"error": "unauthorized"})
			return
		}
		s, ok := mgr.Validate(token)
		if !ok {
			c.AbortWithStatusJSON(401, gin.H{"error": "unauthorized"})
			return
		}
		c.Set(ContextSessionKey, s)
		c.Next()
	}
}

func RequireRole(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		r = strings.ToLower(strings.TrimSpace(r))
		if r != "" {
			allowed[r] = struct{}{}
		}
	}
	return func(c *gin.Context) {
		s, ok := SessionFromContext(c)
		if !ok {
			c.AbortWithStatusJSON(401, gin.H{"error": "unauthorized"})
			return
		}
		if len(allowed) == 0 {
			c.Next()
			return
		}
		if _, ok := allowed[strings.ToLower(strings.TrimSpace(s.Role))]; !ok {
			c.AbortWithStatusJSON(403, gin.H{"error": "forbidden"})
			return
		}
		c.Next()
	}
}

func SessionFromContext(c *gin.Context) (Session, bool) {
	v, ok := c.Get(ContextSessionKey)
	if !ok {
		return Session{}, false
	}
	s, ok := v.(Session)
	return s, ok
}
