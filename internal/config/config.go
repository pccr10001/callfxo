package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	HTTP      HTTPConfig      `yaml:"http"`
	SIP       SIPConfig       `yaml:"sip"`
	Media     MediaConfig     `yaml:"media"`
	Database  DatabaseConfig  `yaml:"database"`
	Auth      AuthConfig      `yaml:"auth"`
	FCM       FCMConfig       `yaml:"fcm"`
	Bootstrap BootstrapConfig `yaml:"bootstrap"`
}

type HTTPConfig struct {
	Listen string `yaml:"listen"`
}

type SIPConfig struct {
	Transport     string `yaml:"transport"`
	Listen        string `yaml:"listen"`
	Realm         string `yaml:"realm"`
	Domain        string `yaml:"domain"`
	AdvertisedIP  string `yaml:"advertised_ip"`
	ContactUser   string `yaml:"contact_user"`
	NonceTTLHours int    `yaml:"nonce_ttl_hours"`
}

type MediaConfig struct {
	RTPBindIP   string   `yaml:"rtp_bind_ip"`
	PublicIP    string   `yaml:"public_ip"`
	ICESTUNURLs []string `yaml:"ice_stun_urls"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type AuthConfig struct {
	CookieName       string `yaml:"cookie_name"`
	SessionSecret    string `yaml:"session_secret"`
	AccessTTLMinutes int    `yaml:"access_ttl_minutes"`
	RefreshTTLHours  int    `yaml:"refresh_ttl_hours"`
	LegacySessionTTL int    `yaml:"session_ttl_hours"`
}

type FCMConfig struct {
	Enabled            bool   `yaml:"enabled"`
	ProjectID          string `yaml:"project_id"`
	WebAppID           string `yaml:"web_app_id"`
	WebAPIKey          string `yaml:"web_api_key"`
	AndroidAppID       string `yaml:"android_app_id"`
	AndroidAPIKey      string `yaml:"android_api_key"`
	MessagingSenderID  string `yaml:"messaging_sender_id"`
	AuthDomain         string `yaml:"auth_domain"`
	StorageBucket      string `yaml:"storage_bucket"`
	MeasurementID      string `yaml:"measurement_id"`
	VAPIDKey           string `yaml:"vapid_key"`
	ServiceAccountJSON string `yaml:"service_account_json"`
}

type BootstrapConfig struct {
	AdminUsername string `yaml:"admin_username"`
	AdminPassword string `yaml:"admin_password"`
}

func Default() Config {
	return Config{
		HTTP: HTTPConfig{Listen: ":8080"},
		SIP: SIPConfig{
			Transport:     "udp",
			Listen:        "0.0.0.0:5060",
			Realm:         "callfxo",
			Domain:        "callfxo.local",
			AdvertisedIP:  "127.0.0.1",
			ContactUser:   "callfxo",
			NonceTTLHours: 1,
		},
		Media: MediaConfig{
			RTPBindIP:   "0.0.0.0",
			PublicIP:    "127.0.0.1",
			ICESTUNURLs: []string{"stun:stun.l.google.com:19302"},
		},
		Database: DatabaseConfig{Path: "./callfxo.db"},
		Auth: AuthConfig{
			CookieName:       "callfxo_access",
			SessionSecret:    "change-me-very-long-random-string",
			AccessTTLMinutes: 60,
			RefreshTTLHours:  24 * 30,
			LegacySessionTTL: 24,
		},
		FCM: FCMConfig{
			Enabled:            false,
			ProjectID:          "",
			WebAppID:           "",
			WebAPIKey:          "",
			AndroidAppID:       "",
			AndroidAPIKey:      "",
			MessagingSenderID:  "",
			AuthDomain:         "",
			StorageBucket:      "",
			MeasurementID:      "",
			VAPIDKey:           "",
			ServiceAccountJSON: "",
		},
		Bootstrap: BootstrapConfig{
			AdminUsername: "admin",
			AdminPassword: "admin123",
		},
	}
}

func Ensure(path string) (Config, error) {
	if _, err := os.Stat(path); err == nil {
		return Load(path)
	} else if !os.IsNotExist(err) {
		return Config{}, fmt.Errorf("stat config: %w", err)
	}

	cfg := Default()
	if err := Save(path, cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Save(path string, cfg Config) error {
	b, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	cfg := Default()
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config yaml: %w", err)
	}
	if cfg.Auth.AccessTTLMinutes <= 0 {
		if cfg.Auth.LegacySessionTTL > 0 {
			cfg.Auth.AccessTTLMinutes = cfg.Auth.LegacySessionTTL * 60
		} else {
			cfg.Auth.AccessTTLMinutes = Default().Auth.AccessTTLMinutes
		}
	}
	if cfg.Auth.RefreshTTLHours <= 0 {
		cfg.Auth.RefreshTTLHours = Default().Auth.RefreshTTLHours
	}
	return cfg, nil
}
