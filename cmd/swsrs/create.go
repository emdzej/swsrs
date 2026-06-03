package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/emdzej/swsrs/pkg/client"
	"github.com/emdzej/swsrs/pkg/client/auth"
)

func runCreate(args []string) int {
	fs := flag.NewFlagSet("create", flag.ExitOnError)
	adminURL := fs.String("admin-url", envOr("SWSRS_ADMIN_URL", ""), "admin API base URL, e.g. https://relay.example.com")
	token := fs.String("oidc-token", os.Getenv("SWSRS_OIDC_TOKEN"), "OIDC bearer token (overrides cached credentials)")
	credsPath := fs.String("credentials", "", "path to cached credentials file (default: OS user-config dir)")
	output := fs.String("output", "json", "json | env (env emits SWSRS_SESSION/INITIATOR_TOKEN/RESPONDER_TOKEN for eval)")
	_ = fs.Parse(args)

	if *adminURL == "" {
		fmt.Fprintln(os.Stderr, "missing --admin-url (or SWSRS_ADMIN_URL)")
		return 2
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	tokSource, err := resolveTokenSource(ctx, *adminURL, *token, *credsPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

	admin := &client.Admin{BaseURL: *adminURL, Token: tokSource}
	sess, err := admin.CreateSession(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "create session:", err)
		return 1
	}

	switch *output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(sess)
	case "env":
		fmt.Printf("SWSRS_SESSION=%s\n", sess.ID)
		fmt.Printf("SWSRS_INITIATOR_TOKEN=%s\n", sess.InitiatorToken)
		fmt.Printf("SWSRS_RESPONDER_TOKEN=%s\n", sess.ResponderToken)
	default:
		fmt.Fprintln(os.Stderr, "unknown --output:", *output)
		return 2
	}
	return 0
}

// resolveTokenSource picks where the OIDC bearer comes from, in order:
//  1. explicit flag / env (--oidc-token / SWSRS_OIDC_TOKEN)
//  2. the cached credentials file (auth.FileTokenStore) — discovers the
//     relay's OIDC config first, then loads + refreshes against it
//  3. if discovery returns ErrAuthDisabled (server in --no-auth mode),
//     returns an empty-token source
func resolveTokenSource(ctx context.Context, relayURL, explicit, credsPath string) (client.TokenSource, error) {
	if explicit != "" {
		return client.StaticToken(explicit), nil
	}
	cfg, err := auth.Discover(ctx, relayURL)
	if err != nil {
		if errors.Is(err, auth.ErrAuthDisabled) {
			return client.StaticToken(""), nil
		}
		return nil, fmt.Errorf("auth discovery: %w (pass --oidc-token to bypass)", err)
	}
	store := &auth.FileTokenStore{Path: credsPath}
	return auth.AdminTokenSource(cfg, store), nil
}
