package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

func runRaw(args []string) int {
	fs := flag.NewFlagSet("raw", flag.ExitOnError)
	var pf peerFlags
	pf.attach(fs)
	_ = fs.Parse(args)

	if err := pf.validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	conn, err := pf.connect(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "connect:", err)
		return 1
	}
	defer conn.Close()

	errCh := make(chan error, 2)
	go func() { _, err := io.Copy(conn, os.Stdin); errCh <- err }()
	go func() { _, err := io.Copy(os.Stdout, conn); errCh <- err }()

	select {
	case <-ctx.Done():
	case <-errCh:
	}
	return 0
}
