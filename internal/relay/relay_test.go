package relay

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/emdzej/swsrs/internal/session"
)

type testRelay struct {
	url   string
	store *session.Store
	sess  *session.Session
}

func startRelay(t *testing.T, peerWait time.Duration) *testRelay {
	t.Helper()
	store := session.NewStore(time.Minute)
	mux := http.NewServeMux()
	(&Handler{
		Store:           store,
		Logger:          slog.New(slog.NewTextHandler(io.Discard, nil)),
		PeerWaitTimeout: peerWait,
		MaxFrameSize:    -1,
	}).Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &testRelay{url: "ws" + strings.TrimPrefix(srv.URL, "http"), store: store, sess: store.Create()}
}

func (tr *testRelay) dial(t *testing.T, token string) *websocket.Conn {
	t.Helper()
	c, err := tr.tryDial(token)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	c.SetReadLimit(-1)
	t.Cleanup(func() { _ = c.CloseNow() })
	return c
}

func (tr *testRelay) tryDial(token string) (*websocket.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, tr.url+"/relay/"+tr.sess.ID, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": []string{"Bearer " + token}},
	})
	return c, err
}

func (tr *testRelay) pair(t *testing.T) (initiator, responder *websocket.Conn) {
	t.Helper()
	it, rt := tr.sess.Tokens()
	initiator = tr.dial(t, it)
	responder = tr.dial(t, rt)
	waitFor(t, func() bool { return tr.sess.Status().State == "open" })
	return initiator, responder
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met within 5s")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func readCtx(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func expectClose(t *testing.T, c *websocket.Conn, want websocket.StatusCode) {
	t.Helper()
	_, _, err := c.Read(readCtx(t))
	if got := websocket.CloseStatus(err); got != want {
		t.Fatalf("close status = %v (err %v), want %v", got, err, want)
	}
}

func TestRelayPreservesMessages(t *testing.T) {
	tr := startRelay(t, 5*time.Second)
	a, b := tr.pair(t)

	big := bytes.Repeat([]byte("x"), 8<<20)
	msgs := [][]byte{[]byte("hello"), big, {}}
	go func() {
		for _, m := range msgs {
			_ = a.Write(context.Background(), websocket.MessageBinary, m)
		}
	}()
	for i, want := range msgs {
		typ, got, err := b.Read(readCtx(t))
		if err != nil {
			t.Fatalf("read %d: %v", i, err)
		}
		if typ != websocket.MessageBinary || !bytes.Equal(got, want) {
			t.Fatalf("message %d: got %d bytes, want %d", i, len(got), len(want))
		}
	}
	if st := tr.sess.Status(); st.BytesIn != uint64(5+len(big)) {
		t.Fatalf("bytes_in = %d", st.BytesIn)
	}
}

func TestDeleteDisconnectsPeers(t *testing.T) {
	tr := startRelay(t, 5*time.Second)
	a, b := tr.pair(t)
	tr.store.Delete(tr.sess.ID)
	expectClose(t, a, websocket.StatusGoingAway)
	expectClose(t, b, websocket.StatusGoingAway)
}

func TestReapDisconnectsPeers(t *testing.T) {
	tr := startRelay(t, 5*time.Second)
	a, b := tr.pair(t)
	tr.store.Reap(tr.sess.ExpiresAt.Add(time.Second))
	expectClose(t, a, websocket.StatusGoingAway)
	expectClose(t, b, websocket.StatusGoingAway)
}

func TestPeerDisconnectClosesCounterpart(t *testing.T) {
	tr := startRelay(t, 5*time.Second)
	a, b := tr.pair(t)
	_ = a.Close(websocket.StatusNormalClosure, "")
	expectClose(t, b, websocket.StatusNormalClosure)
}

func TestPingAnsweredWhileWaiting(t *testing.T) {
	tr := startRelay(t, 5*time.Second)
	it, _ := tr.sess.Tokens()
	a := tr.dial(t, it)
	a.CloseRead(context.Background()) // client must read for pongs to arrive
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := a.Ping(ctx); err != nil {
		t.Fatalf("ping while waiting for peer: %v", err)
	}
}

func TestCloseWhileWaitingFreesSlot(t *testing.T) {
	tr := startRelay(t, 5*time.Second)
	it, _ := tr.sess.Tokens()
	a := tr.dial(t, it)
	start := time.Now()
	_ = a.Close(websocket.StatusNormalClosure, "")
	if d := time.Since(start); d > time.Second {
		t.Fatalf("close took %v", d)
	}
	waitFor(t, func() bool { return tr.sess.SlotFree(session.RoleInitiator) })
	tr.dial(t, it)
}

func TestSlotTakenIsConflict(t *testing.T) {
	tr := startRelay(t, 5*time.Second)
	it, _ := tr.sess.Tokens()
	tr.dial(t, it)
	waitFor(t, func() bool { return !tr.sess.SlotFree(session.RoleInitiator) })
	_, err := tr.tryDial(it)
	if err == nil || !strings.Contains(err.Error(), "409") {
		t.Fatalf("second dial on same slot: %v, want 409", err)
	}
}

func TestPeerWaitTimeout(t *testing.T) {
	tr := startRelay(t, 200*time.Millisecond)
	it, _ := tr.sess.Tokens()
	a := tr.dial(t, it)
	expectClose(t, a, websocket.StatusGoingAway)
}

func TestPeerWaitTimeoutWithEarlyData(t *testing.T) {
	tr := startRelay(t, 200*time.Millisecond)
	it, _ := tr.sess.Tokens()
	a := tr.dial(t, it)
	if err := a.Write(readCtx(t), websocket.MessageBinary, []byte("early")); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, _, err := a.Read(readCtx(t))
	if err == nil {
		t.Fatal("expected close")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("timeout close took %v", d)
	}
}

func TestEarlyDataDeliveredOnAttach(t *testing.T) {
	tr := startRelay(t, 5*time.Second)
	it, rt := tr.sess.Tokens()
	a := tr.dial(t, it)
	if err := a.Write(readCtx(t), websocket.MessageBinary, []byte("early")); err != nil {
		t.Fatal(err)
	}
	b := tr.dial(t, rt)
	_, got, err := b.Read(readCtx(t))
	if err != nil || string(got) != "early" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestUnknownSessionAndBadToken(t *testing.T) {
	tr := startRelay(t, time.Second)
	if _, err := tr.tryDial("nope"); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("bad token: %v", err)
	}
	tr.store.Delete(tr.sess.ID)
	it, _ := tr.sess.Tokens()
	_, err := tr.tryDial(it)
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("deleted session: %v", err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("dial hung")
	}
}
