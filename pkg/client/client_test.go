package client_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/emdzej/swsrs/internal/relay"
	"github.com/emdzej/swsrs/internal/session"
	"github.com/emdzej/swsrs/pkg/client"
)

// startRelay stands up a relay-only HTTP server (no OIDC, no admin) and
// returns its base URL plus a session created directly in the store.
func startRelay(t *testing.T) (string, *session.Session, func()) {
	t.Helper()
	store := session.NewStore(time.Minute)
	mux := http.NewServeMux()
	(&relay.Handler{
		Store:           store,
		Logger:          slog.New(slog.NewTextHandler(io.Discard, nil)),
		PeerWaitTimeout: 5 * time.Second,
	}).Register(mux)

	srv := httptest.NewServer(mux)
	sess := store.Create()
	return srv.URL, sess, srv.Close
}

func TestDialAcceptRoundTrip(t *testing.T) {
	url, sess, stop := startRelay(t)
	defer stop()

	initTok, respTok := sess.Tokens()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	initiatorCh := make(chan *client.Conn, 1)
	go func() {
		c, err := client.Dial(ctx, client.DialOptions{
			RelayURL:  url,
			SessionID: sess.ID,
			Token:     initTok,
		})
		if err != nil {
			t.Errorf("dial: %v", err)
			initiatorCh <- nil
			return
		}
		initiatorCh <- c
	}()

	responder, err := client.Accept(ctx, client.DialOptions{
		RelayURL:  url,
		SessionID: sess.ID,
		Token:     respTok,
	})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	defer responder.Close()

	initiator := <-initiatorCh
	if initiator == nil {
		t.Fatal("initiator failed to dial")
	}
	defer initiator.Close()

	// initiator -> responder
	want := "hello from initiator"
	if _, err := initiator.Write([]byte(want)); err != nil {
		t.Fatalf("initiator write: %v", err)
	}
	buf := make([]byte, 1024)
	n, err := responder.Read(buf)
	if err != nil {
		t.Fatalf("responder read: %v", err)
	}
	if got := string(buf[:n]); got != want {
		t.Fatalf("responder got %q, want %q", got, want)
	}

	// responder -> initiator
	want2 := "hello back"
	if _, err := responder.Write([]byte(want2)); err != nil {
		t.Fatalf("responder write: %v", err)
	}
	n, err = initiator.Read(buf)
	if err != nil {
		t.Fatalf("initiator read: %v", err)
	}
	if got := string(buf[:n]); got != want2 {
		t.Fatalf("initiator got %q, want %q", got, want2)
	}
}

func TestUnauthorizedToken(t *testing.T) {
	url, sess, stop := startRelay(t)
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := client.Dial(ctx, client.DialOptions{
		RelayURL:  url,
		SessionID: sess.ID,
		Token:     "not-the-real-token",
	})
	if err == nil {
		t.Fatal("expected error for bad token, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected 401 error, got: %v", err)
	}
}

func TestSendRecvPreservesBoundaries(t *testing.T) {
	url, sess, stop := startRelay(t)
	defer stop()
	initTok, respTok := sess.Tokens()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		c, err := client.Dial(ctx, client.DialOptions{RelayURL: url, SessionID: sess.ID, Token: initTok})
		if err != nil {
			t.Errorf("dial: %v", err)
			return
		}
		defer c.Close()
		_ = c.Send(ctx, []byte("frame-one"))
		_ = c.Send(ctx, []byte("frame-two"))
		time.Sleep(100 * time.Millisecond)
	}()

	r, err := client.Accept(ctx, client.DialOptions{RelayURL: url, SessionID: sess.ID, Token: respTok})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	defer r.Close()

	got1, err := r.Recv(ctx)
	if err != nil {
		t.Fatalf("recv 1: %v", err)
	}
	if string(got1) != "frame-one" {
		t.Fatalf("frame 1 = %q", got1)
	}
	got2, err := r.Recv(ctx)
	if err != nil {
		t.Fatalf("recv 2: %v", err)
	}
	if string(got2) != "frame-two" {
		t.Fatalf("frame 2 = %q", got2)
	}
}

func pairConns(t *testing.T, keepalive time.Duration) (*client.Conn, *client.Conn) {
	t.Helper()
	url, sess, stop := startRelay(t)
	t.Cleanup(stop)
	initTok, respTok := sess.Tokens()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	a, err := client.Dial(ctx, client.DialOptions{RelayURL: url, SessionID: sess.ID, Token: initTok, Keepalive: keepalive})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	b, err := client.Accept(ctx, client.DialOptions{RelayURL: url, SessionID: sess.ID, Token: respTok, Keepalive: keepalive})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	t.Cleanup(func() { _ = a.Close(); _ = b.Close() })
	return a, b
}

func TestReadDeadlinePassedLeavesConnUsable(t *testing.T) {
	a, b := pairConns(t, -1)
	_ = b.SetReadDeadline(time.Now().Add(-time.Second))
	var nerr net.Error
	if _, err := b.Read(make([]byte, 8)); !errors.As(err, &nerr) || !nerr.Timeout() {
		t.Fatalf("read past deadline: %v, want timeout", err)
	}
	_ = b.SetReadDeadline(time.Time{})
	if _, err := a.Write([]byte("still-open")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 32)
	n, err := b.Read(buf)
	if err != nil || string(buf[:n]) != "still-open" {
		t.Fatalf("read after clearing deadline: %q %v", buf[:n], err)
	}
}

func TestReadDeadlineInterruptsBlockedRead(t *testing.T) {
	_, b := pairConns(t, -1)
	errCh := make(chan error, 1)
	go func() {
		_, err := b.Read(make([]byte, 8))
		errCh <- err
	}()
	time.Sleep(100 * time.Millisecond)
	_ = b.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	select {
	case err := <-errCh:
		if !errors.Is(err, os.ErrDeadlineExceeded) {
			t.Fatalf("got %v, want os.ErrDeadlineExceeded", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("blocked Read not interrupted by SetReadDeadline")
	}
}

func TestReadStreamsLargeMessage(t *testing.T) {
	a, b := pairConns(t, -1)
	msg := bytes.Repeat([]byte("0123456789"), 400_000)
	go func() { _, _ = a.Write(msg) }()
	got := make([]byte, len(msg))
	if _, err := io.ReadFull(b, got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, msg) {
		t.Fatal("payload mismatch")
	}
}

func TestKeepaliveSurvivesPeerWait(t *testing.T) {
	url, sess, stop := startRelay(t)
	defer stop()
	initTok, respTok := sess.Tokens()
	ctx := context.Background()
	a, err := client.Dial(ctx, client.DialOptions{RelayURL: url, SessionID: sess.ID, Token: initTok, Keepalive: 200 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	got := make(chan string, 1)
	go func() {
		buf := make([]byte, 16)
		n, _ := a.Read(buf)
		got <- string(buf[:n])
	}()
	time.Sleep(time.Second) // several keepalive intervals with no peer
	b, err := client.Accept(ctx, client.DialOptions{RelayURL: url, SessionID: sess.ID, Token: respTok, Keepalive: -1})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if _, err := b.Write([]byte("hi")); err != nil {
		t.Fatal(err)
	}
	select {
	case s := <-got:
		if s != "hi" {
			t.Fatalf("got %q", s)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no data after peer wait")
	}
}
