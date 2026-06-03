package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/emdzej/swsrs/pkg/client"
)

// peerFlags collects the common flags for any subcommand that opens a
// peer connection to the relay.
type peerFlags struct {
	URL     string
	Session string
	Token   string
	Role    string
}

func (p *peerFlags) attach(fs *flag.FlagSet) {
	fs.StringVar(&p.URL, "url", envOr("SWSRS_URL", ""), "relay base URL, e.g. wss://relay.example.com")
	fs.StringVar(&p.Session, "session", os.Getenv("SWSRS_SESSION"), "session id")
	fs.StringVar(&p.Token, "token", os.Getenv("SWSRS_TOKEN"), "session token")
	fs.StringVar(&p.Role, "role", "responder", "initiator | responder (controls which slot we claim; same wire effect)")
}

func (p *peerFlags) validate() error {
	missing := []string{}
	if p.URL == "" {
		missing = append(missing, "--url")
	}
	if p.Session == "" {
		missing = append(missing, "--session")
	}
	if p.Token == "" {
		missing = append(missing, "--token")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required flags: %s", strings.Join(missing, ", "))
	}
	if p.Role != "initiator" && p.Role != "responder" {
		return fmt.Errorf("--role must be 'initiator' or 'responder'")
	}
	return nil
}

func (p *peerFlags) connect(ctx context.Context) (*client.Conn, error) {
	opts := client.DialOptions{
		RelayURL:  p.URL,
		SessionID: p.Session,
		Token:     p.Token,
	}
	if p.Role == "initiator" {
		return client.Dial(ctx, opts)
	}
	return client.Accept(ctx, opts)
}
