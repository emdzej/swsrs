package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/emdzej/swsrs/pkg/client/auth"
)

func runAuth(args []string) int {
	fs := flag.NewFlagSet("auth", flag.ExitOnError)
	relay := fs.String("relay", envOr("SWSRS_URL", ""), "relay base URL, e.g. https://relay.example.com")
	clientID := fs.String("client-id", os.Getenv("SWSRS_OIDC_CLIENT_ID"), "OAuth client_id to use (overrides client_id_hint from discovery)")
	credsPath := fs.String("credentials", "", "path to write the credentials file (default: OS user-config dir)")
	logout := fs.Bool("logout", false, "remove the cached credentials file and exit")
	_ = fs.Parse(args)

	store := &auth.FileTokenStore{Path: *credsPath}

	if *logout {
		if err := store.Clear(context.Background()); err != nil {
			fmt.Fprintln(os.Stderr, "logout:", err)
			return 1
		}
		fmt.Fprintln(os.Stderr, "cleared cached credentials.")
		return 0
	}

	if *relay == "" {
		fmt.Fprintln(os.Stderr, "missing --relay (or SWSRS_URL)")
		return 2
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	fmt.Fprintf(os.Stderr, "discovering OIDC config from %s ...\n", *relay)
	cfg, err := auth.Discover(ctx, *relay)
	if err != nil {
		if errors.Is(err, auth.ErrAuthDisabled) {
			fmt.Fprintln(os.Stderr, "relay is running with --no-auth — no token needed.")
			return 0
		}
		fmt.Fprintln(os.Stderr, "discover:", err)
		return 1
	}

	tok, err := cfg.DeviceLogin(ctx, auth.DeviceLoginOptions{
		ClientID: *clientID,
		OnPrompt: func(p auth.DevicePrompt) {
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "  1. open this URL on any device:")
			if p.VerificationURIComplete != "" {
				fmt.Fprintf(os.Stderr, "       %s\n", p.VerificationURIComplete)
				fmt.Fprintln(os.Stderr, "     (or, if that URL doesn't pre-fill the code:)")
				fmt.Fprintf(os.Stderr, "       %s\n", p.VerificationURI)
			} else {
				fmt.Fprintf(os.Stderr, "       %s\n", p.VerificationURI)
			}
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintf(os.Stderr, "  2. enter code: %s\n", p.UserCode)
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintf(os.Stderr, "  (waiting; code expires at %s)\n", p.ExpiresAt.Format(time.RFC3339))
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "device login:", err)
		return 1
	}

	if err := store.Save(ctx, tok); err != nil {
		fmt.Fprintln(os.Stderr, "save credentials:", err)
		return 1
	}
	final, _ := store.Load(ctx) //nolint:errcheck // best-effort, only for output
	p, _ := auth.DefaultFilePath()
	if final != nil {
		expIn := time.Until(final.Expiry).Round(time.Second)
		fmt.Fprintf(os.Stderr, "✓ saved token to %s (access token expires in %s)\n",
			strings.TrimSpace(displayPath(*credsPath, p)), expIn,
		)
	} else {
		fmt.Fprintf(os.Stderr, "✓ saved token to %s\n", displayPath(*credsPath, p))
	}
	return 0
}

func displayPath(explicit, def string) string {
	if explicit != "" {
		return explicit
	}
	return def
}
