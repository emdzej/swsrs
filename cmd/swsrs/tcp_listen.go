package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"syscall"
)

// tcp-listen opens a local TCP listener; the first accepted connection is
// tunneled through the relay session and the listener closes. One TCP
// connection per session — for multiple concurrent tunnels, create multiple
// sessions.
func runTCPListen(args []string) int {
	fs := flag.NewFlagSet("tcp-listen", flag.ExitOnError)
	var pf peerFlags
	pf.attach(fs)
	listen := fs.String("listen", "127.0.0.1:0", "local TCP address to listen on")
	_ = fs.Parse(args)

	if err := pf.validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		fmt.Fprintln(os.Stderr, "listen:", err)
		return 1
	}
	defer ln.Close()
	fmt.Fprintf(os.Stderr, "tcp-listen: waiting for TCP on %s\n", ln.Addr())

	// Accept exactly one connection.
	acceptCh := make(chan net.Conn, 1)
	errCh := make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			errCh <- err
			return
		}
		acceptCh <- c
	}()

	var tcpConn net.Conn
	select {
	case <-ctx.Done():
		return 0
	case err := <-errCh:
		fmt.Fprintln(os.Stderr, "accept:", err)
		return 1
	case tcpConn = <-acceptCh:
	}
	_ = ln.Close()
	defer tcpConn.Close()

	relayConn, err := pf.connect(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "relay connect:", err)
		return 1
	}
	defer relayConn.Close()

	return pipe(ctx, tcpConn, relayConn)
}

// pipe runs bidirectional io.Copy between a and b until either side errors,
// returning a process exit code.
func pipe(ctx context.Context, a, b io.ReadWriteCloser) int {
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(a, b); done <- struct{}{} }()
	go func() { _, _ = io.Copy(b, a); done <- struct{}{} }()
	select {
	case <-ctx.Done():
	case <-done:
	}
	return 0
}
