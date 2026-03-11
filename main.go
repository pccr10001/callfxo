package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/pccr10001/callfxo/internal/auth"
	"github.com/pccr10001/callfxo/internal/call"
	"github.com/pccr10001/callfxo/internal/config"
	"github.com/pccr10001/callfxo/internal/push"
	"github.com/pccr10001/callfxo/internal/sipx"
	"github.com/pccr10001/callfxo/internal/store"
	"github.com/pccr10001/callfxo/internal/web"
)

func main() {
	cfgPath := flag.String("config", "config.yaml", "config yaml path")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Ensure(*cfgPath)
	if err != nil {
		logger.Error("load config failed", "error", err)
		os.Exit(1)
	}

	freshDB, err := isFreshDB(cfg.Database.Path)
	if err != nil {
		logger.Error("inspect database path failed", "error", err)
		os.Exit(1)
	}

	st, err := store.Open(cfg.Database.Path)
	if err != nil {
		logger.Error("open database failed", "error", err)
		os.Exit(1)
	}
	defer st.Close()

	secret, generatedSecret, err := ensureSessionSecret(st)
	if err != nil {
		logger.Error("prepare auth session secret failed", "error", err)
		os.Exit(1)
	}
	if generatedSecret {
		logger.Info("generated session secret", "setting", settingAuthSessionSecretKey)
	}

	adminPwd, generatedAdminPwd, err := bootstrapAdmin(st, cfg, freshDB, logger)
	if err != nil {
		logger.Error("bootstrap admin failed", "error", err)
		os.Exit(1)
	}
	if generatedAdminPwd {
		adminUser := strings.TrimSpace(cfg.Bootstrap.AdminUsername)
		if adminUser == "" {
			adminUser = "admin"
		}
		logger.Info("generated bootstrap admin password", "username", adminUser, "password", adminPwd)
	}

	authMgr := auth.NewManager(secret, time.Duration(cfg.Auth.AccessTTLMinutes)*time.Minute)
	pushSvc := push.New(cfg.FCM)
	sipSvc, err := sipx.New(cfg.SIP, st, logger)
	if err != nil {
		logger.Error("create sip service failed", "error", err)
		os.Exit(1)
	}
	defer sipSvc.Close()

	callMgr, err := call.NewManager(cfg.Media, cfg.SIP, st, sipSvc, logger)
	if err != nil {
		logger.Error("create call manager failed", "error", err)
		os.Exit(1)
	}

	webServer := web.New(cfg, st, authMgr, callMgr, pushSvc, logger)
	callMgr.SetNotifier(webServer)
	router := webServer.Router()
	httpSrv := &http.Server{Addr: cfg.HTTP.Listen, Handler: router}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go authMgr.StartCleanup(ctx.Done())
	go cleanupRegistrationsLoop(ctx, st, logger)
	webServer.StartBackground(ctx)

	go func() {
		if err := sipSvc.Run(ctx); err != nil {
			logger.Error("sip listener stopped", "error", err)
			stop()
		}
	}()

	go func() {
		logger.Info("HTTP server listening", "addr", cfg.HTTP.Listen)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server error", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		logger.Warn("http shutdown error", "error", err)
	}
	logger.Info("shutdown complete")
}

const settingAuthSessionSecretKey = "auth.session_secret"

func isFreshDB(path string) (bool, error) {
	if strings.TrimSpace(path) == "" {
		return false, fmt.Errorf("database path is empty")
	}
	info, err := os.Stat(path)
	if err == nil {
		if info.IsDir() {
			return false, fmt.Errorf("database path is a directory: %s", path)
		}
		return false, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	return false, err
}

func ensureSessionSecret(st *store.Store) (string, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	value, err := st.GetSetting(ctx, settingAuthSessionSecretKey)
	if err == nil && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value), false, nil
	}
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return "", false, err
	}

	secret, err := randomToken(48)
	if err != nil {
		return "", false, err
	}
	if err := st.UpsertSetting(ctx, settingAuthSessionSecretKey, secret); err != nil {
		return "", false, err
	}
	return secret, true, nil
}

func bootstrapAdmin(st *store.Store, cfg config.Config, freshDB bool, log *slog.Logger) (string, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	username := strings.TrimSpace(cfg.Bootstrap.AdminUsername)
	if username == "" {
		username = "admin"
	}

	uc, err := st.GetUserCredentialByUsername(ctx, username)
	if err == nil {
		if uc.Role != store.RoleAdmin {
			if err := st.SetUserRoleByUsername(ctx, username, store.RoleAdmin); err != nil {
				return "", false, err
			}
			log.Info("bootstrap admin promoted", "username", username)
		}
		return "", false, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return "", false, err
	}

	adminCount, err := st.CountAdmins(ctx)
	if err != nil {
		return "", false, err
	}
	if adminCount > 0 {
		return "", false, nil
	}

	adminPwd := strings.TrimSpace(cfg.Bootstrap.AdminPassword)
	generated := false
	if freshDB || adminPwd == "" {
		adminPwd, err = randomToken(16)
		if err != nil {
			return "", false, err
		}
		generated = true
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(adminPwd), bcrypt.DefaultCost)
	if err != nil {
		return "", false, err
	}
	_, err = st.CreateUser(ctx, username, string(hash), store.RoleAdmin)
	if err != nil {
		return "", false, err
	}
	log.Info("bootstrap admin created", "username", username)
	return adminPwd, generated, nil
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

func cleanupRegistrationsLoop(ctx context.Context, st *store.Store, log *slog.Logger) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := st.CleanupExpiredRegistrations(context.Background()); err != nil {
				log.Warn("cleanup registrations failed", "error", err)
			}
		case <-ctx.Done():
			return
		}
	}
}
