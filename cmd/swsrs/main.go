// Command swsrs is the simple websocket relay service.
//
// Subcommands:
//
//	serve         run the relay server
//	create        create a session (admin client)
//	tcp-listen    accept local TCP connections and tunnel them through the relay
//	tcp-dial      receive relayed connections and dial a local TCP target
//	raw           bridge stdin/stdout to the relay (debug + scripting)
package main

import (
	"fmt"
	"os"
)

// Populated by goreleaser via -ldflags="-X main.version=... ...". Defaults
// to "dev" for local `go build`.
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

const usage = `swsrs - simple websocket relay service

Usage:
  swsrs <command> [flags]

Commands:
  serve         run the relay server
  auth          log in to the relay's IdP via OAuth device flow
  create        create a session (uses the admin API)
  tcp-listen    accept local TCP connections and tunnel them through the relay
  tcp-dial      receive a relayed connection and dial a local TCP target
  raw           bridge stdin/stdout to a relay session
  version       print build information

Run 'swsrs <command> --help' for command-specific flags.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	switch cmd {
	case "serve":
		os.Exit(runServe(args))
	case "auth":
		os.Exit(runAuth(args))
	case "create":
		os.Exit(runCreate(args))
	case "tcp-listen":
		os.Exit(runTCPListen(args))
	case "tcp-dial":
		os.Exit(runTCPDial(args))
	case "raw":
		os.Exit(runRaw(args))
	case "version", "--version", "-v":
		fmt.Printf("swsrs %s\ncommit:  %s\nbuilt:   %s\n", version, commit, date)
		os.Exit(0)
	case "-h", "--help", "help":
		fmt.Print(usage)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n%s", cmd, usage)
		os.Exit(2)
	}
}
