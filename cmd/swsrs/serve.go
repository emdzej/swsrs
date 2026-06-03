package main

import (
	"context"
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

	"github.com/emdzej/swsrs/internal/admin"
	"github.com/emdzej/swsrs/internal/auth"
	"github.com/emdzej/swsrs/internal/discovery"
	"github.com/emdzej/swsrs/internal/relay"
	"github.com/emdzej/swsrs/internal/session"
)

type serveConfig struct {
	Addr            string
	OIDCIssuer      string
	OIDCAudience    string
	OIDCClientID    string
	SessionTTL      time.Duration
	PeerWaitTimeout time.Duration
	ReapInterval    time.Duration
	PublicBaseURL   string
	AllowedOrigins  []string
	TLSCert         string
	TLSKey          string
	NoAuth          bool
}

func runServe(args []string) int {
	cfg := serveConfig{
		Addr:            envOr("SWSRS_ADDR", ":8080"),
		OIDCIssuer:      os.Getenv("SWSRS_OIDC_ISSUER"),
		OIDCAudience:    os.Getenv("SWSRS_OIDC_AUDIENCE"),
		OIDCClientID:    os.Getenv("SWSRS_OIDC_CLIENT_ID"),
		SessionTTL:      envDuration("SWSRS_SESSION_TTL", 1*time.Hour),
		PeerWaitTimeout: envDuration("SWSRS_PEER_WAIT", 2*time.Minute),
		ReapInterval:    envDuration("SWSRS_REAP_INTERVAL", 30*time.Second),
		PublicBaseURL:   os.Getenv("SWSRS_PUBLIC_BASE_URL"),
		TLSCert:         os.Getenv("SWSRS_TLS_CERT"),
		TLSKey:          os.Getenv("SWSRS_TLS_KEY"),
		NoAuth:          os.Getenv("SWSRS_NO_AUTH") == "1" || strings.EqualFold(os.Getenv("SWSRS_NO_AUTH"), "true"),
	}
	if v := os.Getenv("SWSRS_ALLOWED_ORIGINS"); v != "" {
		cfg.AllowedOrigins = strings.Split(v, ",")
	}

	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	fs.StringVar(&cfg.Addr, "addr", cfg.Addr, "listen address")
	fs.StringVar(&cfg.OIDCIssuer, "oidc-issuer", cfg.OIDCIssuer, "OIDC issuer URL (autodiscovery)")
	fs.StringVar(&cfg.OIDCAudience, "oidc-audience", cfg.OIDCAudience, "expected audience (client_id)")
	fs.StringVar(&cfg.OIDCClientID, "oidc-client-id", cfg.OIDCClientID, "shared OAuth client_id surfaced via /.well-known/swsrs-config (optional)")
	fs.DurationVar(&cfg.SessionTTL, "session-ttl", cfg.SessionTTL, "max session lifetime")
	fs.DurationVar(&cfg.PeerWaitTimeout, "peer-wait", cfg.PeerWaitTimeout, "how long to wait for the other peer")
	fs.StringVar(&cfg.PublicBaseURL, "public-base-url", cfg.PublicBaseURL, "public ws(s):// URL for connect links in admin responses")
	fs.StringVar(&cfg.TLSCert, "tls-cert", cfg.TLSCert, "path to PEM cert (with --tls-key enables TLS; omit both to run plain HTTP behind external termination)")
	fs.StringVar(&cfg.TLSKey, "tls-key", cfg.TLSKey, "path to PEM key")
	fs.BoolVar(&cfg.NoAuth, "no-auth", cfg.NoAuth, "DEV ONLY: disable OIDC verification on the admin API; do NOT enable in production")
	_ = fs.Parse(args)

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if !cfg.NoAuth && cfg.OIDCIssuer == "" {
		logger.Error("SWSRS_OIDC_ISSUER (or --oidc-issuer) is required; pass --no-auth for local dev only")
		return 2
	}
	if (cfg.TLSCert == "") != (cfg.TLSKey == "") {
		logger.Error("--tls-cert and --tls-key must be set together (or both omitted for plain HTTP)")
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var verifier *auth.Verifier
	if cfg.NoAuth {
		logger.Warn("AUTH DISABLED — admin API is open. Do not use in production.")
	} else {
		v, err := auth.NewVerifier(ctx, cfg.OIDCIssuer, cfg.OIDCAudience)
		if err != nil {
			logger.Error("oidc init failed", "err", err)
			return 1
		}
		verifier = v
	}

	store := session.NewStore(cfg.SessionTTL)
	go store.RunReaper(ctx, cfg.ReapInterval)

	mux := http.NewServeMux()
	(&admin.API{
		Store:         store,
		Verifier:      verifier,
		PublicBaseURL: cfg.PublicBaseURL,
	}).Register(mux)
	(&relay.Handler{
		Store:           store,
		Logger:          logger,
		PeerWaitTimeout: cfg.PeerWaitTimeout,
		AllowedOrigins:  cfg.AllowedOrigins,
	}).Register(mux)
	mux.Handle("GET /.well-known/swsrs-config", discovery.Handler(
		verifier,
		[]string{admin.ScopeCreate, admin.ScopeRead, admin.ScopeDelete},
		cfg.OIDCClientID,
	))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	tlsEnabled := cfg.TLSCert != ""
	go func() {
		logger.Info("listening", "addr", cfg.Addr, "tls", tlsEnabled, "issuer", cfg.OIDCIssuer)
		var err error
		if tlsEnabled {
			err = srv.ListenAndServeTLS(cfg.TLSCert, cfg.TLSKey)
		} else {
			err = srv.ListenAndServe()
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server crashed", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")
	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
	return 0
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envDuration(k string, def time.Duration) time.Duration {
	if v := os.Getenv(k); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
		fmt.Fprintf(os.Stderr, "ignoring invalid duration in %s\n", k)
	}
	return def
}
