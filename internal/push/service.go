package push

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/pccr10001/callfxo/internal/config"
)

type PublicConfig struct {
	Enabled           bool   `json:"enabled"`
	ProjectID         string `json:"project_id"`
	WebAppID          string `json:"web_app_id"`
	WebAPIKey         string `json:"web_api_key"`
	AndroidAppID      string `json:"android_app_id"`
	AndroidAPIKey     string `json:"android_api_key"`
	MessagingSenderID string `json:"messaging_sender_id"`
	AuthDomain        string `json:"auth_domain"`
	StorageBucket     string `json:"storage_bucket"`
	MeasurementID     string `json:"measurement_id"`
	VAPIDKey          string `json:"vapid_key"`
}

type Service struct {
	cfg        config.FCMConfig
	httpClient *http.Client

	mu       sync.Mutex
	tokenSrc oauth2.TokenSource
}

func New(cfg config.FCMConfig) *Service {
	return &Service{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 12 * time.Second},
	}
}

func (s *Service) PublicConfig() PublicConfig {
	return PublicConfig{
		Enabled:           s.ClientEnabled(),
		ProjectID:         strings.TrimSpace(s.cfg.ProjectID),
		WebAppID:          strings.TrimSpace(s.cfg.WebAppID),
		WebAPIKey:         strings.TrimSpace(s.cfg.WebAPIKey),
		AndroidAppID:      strings.TrimSpace(s.cfg.AndroidAppID),
		AndroidAPIKey:     strings.TrimSpace(s.cfg.AndroidAPIKey),
		MessagingSenderID: strings.TrimSpace(s.cfg.MessagingSenderID),
		AuthDomain:        strings.TrimSpace(s.cfg.AuthDomain),
		StorageBucket:     strings.TrimSpace(s.cfg.StorageBucket),
		MeasurementID:     strings.TrimSpace(s.cfg.MeasurementID),
		VAPIDKey:          strings.TrimSpace(s.cfg.VAPIDKey),
	}
}

func (s *Service) ClientEnabled() bool {
	return s != nil &&
		s.cfg.Enabled &&
		strings.TrimSpace(s.cfg.ProjectID) != "" &&
		(strings.TrimSpace(s.cfg.WebAppID) != "" || strings.TrimSpace(s.cfg.AndroidAppID) != "") &&
		(strings.TrimSpace(s.cfg.WebAPIKey) != "" || strings.TrimSpace(s.cfg.AndroidAPIKey) != "") &&
		strings.TrimSpace(s.cfg.MessagingSenderID) != ""
}

func (s *Service) CanSend() bool {
	return s.ClientEnabled() && strings.TrimSpace(s.cfg.ServiceAccountJSON) != ""
}

func (s *Service) SendData(ctx context.Context, registrationToken string, data map[string]string) error {
	if !s.CanSend() {
		return nil
	}
	registrationToken = strings.TrimSpace(registrationToken)
	if registrationToken == "" {
		return nil
	}
	tokenSrc, err := s.ensureTokenSource()
	if err != nil {
		return err
	}
	accessToken, err := tokenSrc.Token()
	if err != nil {
		return fmt.Errorf("get fcm access token: %w", err)
	}

	payload := map[string]any{
		"message": map[string]any{
			"token": registrationToken,
			"data":  data,
			"android": map[string]any{
				"priority": "HIGH",
			},
			"webpush": map[string]any{
				"headers": map[string]string{
					"Urgency": "high",
				},
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal fcm payload: %w", err)
	}

	url := fmt.Sprintf("https://fcm.googleapis.com/v1/projects/%s/messages:send", strings.TrimSpace(s.cfg.ProjectID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("create fcm request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	res, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send fcm request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("fcm send failed status=%d", res.StatusCode)
	}
	return nil
}

func (s *Service) ensureTokenSource() (oauth2.TokenSource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tokenSrc != nil {
		return s.tokenSrc, nil
	}

	credPath := strings.TrimSpace(s.cfg.ServiceAccountJSON)
	if credPath == "" {
		return nil, fmt.Errorf("fcm.service_account_json is empty")
	}
	credJSON, err := os.ReadFile(credPath)
	if err != nil {
		return nil, fmt.Errorf("read fcm service account: %w", err)
	}
	creds, err := google.CredentialsFromJSON(context.Background(), credJSON, "https://www.googleapis.com/auth/firebase.messaging")
	if err != nil {
		return nil, fmt.Errorf("parse fcm service account: %w", err)
	}
	s.tokenSrc = creds.TokenSource
	return s.tokenSrc, nil
}
