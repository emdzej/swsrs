package client_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
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
