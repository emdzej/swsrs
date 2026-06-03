package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
)

// tcp-dial connects to the relay, dials a local TCP target, and bridges
// the two. Pairs with tcp-listen on the other side of the session.
func runTCPDial(args []string) int {
	fs := flag.NewFlagSet("tcp-dial", flag.ExitOnError)
	var pf peerFlags
	pf.attach(fs)
	target := fs.String("target", "", "local TCP target to dial, e.g. 127.0.0.1:22")
	_ = fs.Parse(args)

	if err := pf.validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if *target == "" {
		fmt.Fprintln(os.Stderr, "missing --target")
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	relayConn, err := pf.connect(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "relay connect:", err)
		return 1
	}
	defer relayConn.Close()

	tcpConn, err := net.Dial("tcp", *target)
	if err != nil {
		fmt.Fprintln(os.Stderr, "tcp dial:", err)
		return 1
	}
	defer tcpConn.Close()

	return pipe(ctx, tcpConn, relayConn)
}
