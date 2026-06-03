package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/emdzej/swsrs/pkg/client"
)

func runCreate(args []string) int {
	fs := flag.NewFlagSet("create", flag.ExitOnError)
	adminURL := fs.String("admin-url", envOr("SWSRS_ADMIN_URL", ""), "admin API base URL, e.g. https://relay.example.com")
	token := fs.String("oidc-token", os.Getenv("SWSRS_OIDC_TOKEN"), "OIDC bearer token (read SWSRS_OIDC_TOKEN if not set)")
	output := fs.String("output", "json", "json | env (env emits SWSRS_SESSION/INITIATOR_TOKEN/RESPONDER_TOKEN for eval)")
	_ = fs.Parse(args)

	if *adminURL == "" || *token == "" {
		fmt.Fprintln(os.Stderr, "missing required: --admin-url and --oidc-token (or env)")
		return 2
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	admin := &client.Admin{BaseURL: *adminURL, Token: client.StaticToken(*token)}
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
