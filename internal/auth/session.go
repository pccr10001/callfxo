package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const ContextSessionKey = "auth_session"

type Session struct {
	Token       string    `json:"-"`
	UserID      int64     `json:"user_id"`
	Username    string    `json:"username"`
	Role        string    `json:"role"`
	DeviceToken string    `json:"device_token"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type Manager struct {
	secret    []byte
	accessTTL time.Duration
}

type accessClaims struct {
	UserID      int64  `json:"uid"`
	Username    string `json:"usr"`
	Role        string `json:"rol"`
	DeviceToken string `json:"dev"`
	ExpiresAt   int64  `json:"exp"`
	Nonce       string `json:"n"`
}

func NewManager(secret string, accessTTL time.Duration) *Manager {
	if accessTTL <= 0 {
		accessTTL = time.Hour
	}
	return &Manager{
		secret:    []byte(secret),
		accessTTL: accessTTL,
	}
}

func (m *Manager) AccessTTL() time.Duration {
	return m.accessTTL
}

func (m *Manager) StartCleanup(_ <-chan struct{}) {}

func (m *Manager) Create(userID int64, username, role, deviceToken string) (Session, error) {
	claims := accessClaims{
		UserID:      userID,
		Username:    strings.TrimSpace(username),
		Role:        strings.ToLower(strings.TrimSpace(role)),
		DeviceToken: strings.TrimSpace(deviceToken),
		ExpiresAt:   time.Now().Add(m.accessTTL).Unix(),
	}

	nonce, err := randomToken(12)
	if err != nil {
		return Session{}, err
	}
	claims.Nonce = nonce

	payload, err := json.Marshal(claims)
	if err != nil {
		return Session{}, fmt.Errorf("marshal access claims: %w", err)
	}

	mac := hmac.New(sha256.New, m.secret)
	mac.Write(payload)
	signature := mac.Sum(nil)
	token := base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(signature)
	return sessionFromClaims(token, claims), nil
}

func (m *Manager) Validate(token string) (Session, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return Session{}, false
	}

	payloadPart, sigPart, ok := strings.Cut(token, ".")
	if !ok || payloadPart == "" || sigPart == "" {
		return Session{}, false
	}

	payload, err := base64.RawURLEncoding.DecodeString(payloadPart)
	if err != nil {
		return Session{}, false
	}
	sig, err := base64.RawURLEncoding.DecodeString(sigPart)
	if err != nil {
		return Session{}, false
	}

	mac := hmac.New(sha256.New, m.secret)
	mac.Write(payload)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return Session{}, false
	}

	var claims accessClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return Session{}, false
	}
	if claims.UserID <= 0 || strings.TrimSpace(claims.DeviceToken) == "" || claims.ExpiresAt <= 0 {
		return Session{}, false
	}
	if time.Now().After(time.Unix(claims.ExpiresAt, 0)) {
		return Session{}, false
	}

	return sessionFromClaims(token, claims), true
}

func (m *Manager) Delete(string) {}

func (m *Manager) DeleteByUser(int64) int { return 0 }

func RequireAuth(mgr *Manager, cookieName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := ExtractToken(c, cookieName)
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

func ExtractToken(c *gin.Context, cookieName string) string {
	authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
	if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		return strings.TrimSpace(authHeader[7:])
	}
	token, err := c.Cookie(cookieName)
	if err == nil {
		return strings.TrimSpace(token)
	}
	return ""
}

func RefreshCookieName(accessCookieName string) string {
	name := strings.TrimSpace(accessCookieName)
	if name == "" {
		return "callfxo_refresh"
	}
	return name + "_refresh"
}

func sessionFromClaims(token string, claims accessClaims) Session {
	return Session{
		Token:       token,
		UserID:      claims.UserID,
		Username:    strings.TrimSpace(claims.Username),
		Role:        strings.ToLower(strings.TrimSpace(claims.Role)),
		DeviceToken: strings.TrimSpace(claims.DeviceToken),
		ExpiresAt:   time.Unix(claims.ExpiresAt, 0),
	}
}

func randomToken(length int) (string, error) {
	if length <= 0 {
		length = 16
	}
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
