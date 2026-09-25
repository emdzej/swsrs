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
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/emdzej/swsrs/internal/admin"
	"github.com/emdzej/swsrs/internal/auth"
	"github.com/emdzej/swsrs/internal/cors"
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
	MaxFrameSize    int64
}

func runServe(args []string) int {
	var envErrs []error
	cfg := serveConfig{
		Addr:            envOr("SWSRS_ADDR", ":8080"),
		OIDCIssuer:      os.Getenv("SWSRS_OIDC_ISSUER"),
		OIDCAudience:    os.Getenv("SWSRS_OIDC_AUDIENCE"),
		OIDCClientID:    os.Getenv("SWSRS_OIDC_CLIENT_ID"),
		SessionTTL:      envDuration("SWSRS_SESSION_TTL", 1*time.Hour, &envErrs),
		PeerWaitTimeout: envDuration("SWSRS_PEER_WAIT", 2*time.Minute, &envErrs),
		ReapInterval:    envDuration("SWSRS_REAP_INTERVAL", 30*time.Second, &envErrs),
		PublicBaseURL:   os.Getenv("SWSRS_PUBLIC_BASE_URL"),
		TLSCert:         os.Getenv("SWSRS_TLS_CERT"),
		TLSKey:          os.Getenv("SWSRS_TLS_KEY"),
		NoAuth:          os.Getenv("SWSRS_NO_AUTH") == "1" || strings.EqualFold(os.Getenv("SWSRS_NO_AUTH"), "true"),
		MaxFrameSize:    envInt64("SWSRS_MAX_FRAME_SIZE", -1, &envErrs),
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
	fs.DurationVar(&cfg.ReapInterval, "reap-interval", cfg.ReapInterval, "how often expired sessions are swept")
	fs.StringVar(&cfg.PublicBaseURL, "public-base-url", cfg.PublicBaseURL, "public ws(s):// URL for connect links in admin responses")
	fs.StringVar(&cfg.TLSCert, "tls-cert", cfg.TLSCert, "path to PEM cert (with --tls-key enables TLS; omit both to run plain HTTP behind external termination)")
	fs.StringVar(&cfg.TLSKey, "tls-key", cfg.TLSKey, "path to PEM key")
	fs.BoolVar(&cfg.NoAuth, "no-auth", cfg.NoAuth, "DEV ONLY: disable OIDC verification on the admin API; do NOT enable in production")
	fs.Int64Var(&cfg.MaxFrameSize, "max-frame-size", cfg.MaxFrameSize, "max WS frame size in bytes accepted on the data plane; -1 = unlimited (recommended for protocol-agnostic relay use)")
	_ = fs.Parse(args)

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// A typo in a limit must not silently fall back to a default.
	if err := errors.Join(envErrs...); err != nil {
		logger.Error("invalid configuration", "err", err)
		return 2
	}
	if cfg.SessionTTL <= 0 || cfg.PeerWaitTimeout <= 0 || cfg.ReapInterval <= 0 {
		logger.Error("--session-ttl, --peer-wait and --reap-interval must be positive")
		return 2
	}
	cfg.PublicBaseURL = strings.TrimRight(cfg.PublicBaseURL, "/")
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
	relayHandler := &relay.Handler{
		Store:           store,
		Logger:          logger,
		PeerWaitTimeout: cfg.PeerWaitTimeout,
		AllowedOrigins:  cfg.AllowedOrigins,
		MaxFrameSize:    cfg.MaxFrameSize,
	}
	relayHandler.Register(mux)
	mux.Handle("GET /.well-known/swsrs-config", discovery.Handler(
		verifier,
		[]string{admin.ScopeCreate, admin.ScopeRead, admin.ScopeDelete},
		cfg.OIDCClientID,
	))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	corsMW := &cors.Middleware{AllowedOriginPatterns: cfg.AllowedOrigins}

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           corsMW.Wrap(mux),
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
	// Shutdown doesn't track hijacked WebSocket connections: close every
	// session so peers get a close frame, then wait for handlers to finish.
	store.CloseAll()
	relayHandler.Wait(shutCtx)
	return 0
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envDuration(k string, def time.Duration, errs *[]error) time.Duration {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s: %w", k, err))
		return def
	}
	return d
}

func envInt64(k string, def int64, errs *[]error) int64 {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s: %w", k, err))
		return def
	}
	return n
}
