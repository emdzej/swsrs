package session

import (
	"testing"
	"time"
)

func TestAuthorizeToken(t *testing.T) {
	s := New(time.Minute)
	it, rt := s.Tokens()
	if it == rt || len(it) != 22 {
		t.Fatalf("unexpected tokens %q %q", it, rt)
	}
	if r, err := s.AuthorizeToken(it); err != nil || r != RoleInitiator {
		t.Fatalf("initiator: %v %v", r, err)
	}
	if r, err := s.AuthorizeToken(rt); err != nil || r != RoleResponder {
		t.Fatalf("responder: %v %v", r, err)
	}
	if _, err := s.AuthorizeToken(""); err != ErrUnauthorized {
		t.Fatalf("empty token: %v", err)
	}
}

func TestStateTransitions(t *testing.T) {
	s := New(time.Minute)
	step := func(want string) {
		t.Helper()
		if got := s.Status().State; got != want {
			t.Fatalf("state = %s, want %s", got, want)
		}
	}
	step("pending")
	if err := s.Attach(RoleInitiator, nil); err != nil {
		t.Fatal(err)
	}
	step("half_open")
	if err := s.Attach(RoleInitiator, nil); err != ErrSlotTaken {
		t.Fatalf("double attach: %v", err)
	}
	if err := s.Attach(RoleResponder, nil); err != nil {
		t.Fatal(err)
	}
	step("open")
	s.Detach(RoleInitiator)
	step("half_open")
	s.Detach(RoleResponder)
	step("pending")

	if err := s.Attach(RoleInitiator, nil); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s.Detach(RoleInitiator)
	step("closed")
	if err := s.Attach(RoleInitiator, nil); err != ErrClosed {
		t.Fatalf("attach after close: %v", err)
	}
}

func TestPeerChangedFires(t *testing.T) {
	s := New(time.Minute)
	_, changed := s.Peer(RoleInitiator)
	if err := s.Attach(RoleResponder, nil); err != nil {
		t.Fatal(err)
	}
	select {
	case <-changed:
	default:
		t.Fatal("changed channel not closed on attach")
	}
}

func TestAttachExpired(t *testing.T) {
	s := New(-time.Second)
	if err := s.Attach(RoleInitiator, nil); err != ErrExpired {
		t.Fatalf("got %v", err)
	}
}

func TestStoreReapAndList(t *testing.T) {
	st := NewStore(time.Minute)
	a := st.Create()
	time.Sleep(time.Millisecond)
	b := st.Create()
	list := st.List()
	if len(list) != 2 || list[0] != a || list[1] != b {
		t.Fatal("List not ordered by creation time")
	}
	if n := st.Reap(time.Now()); n != 0 {
		t.Fatalf("reaped %d live sessions", n)
	}
	if n := st.Reap(time.Now().Add(2 * time.Minute)); n != 2 {
		t.Fatalf("reaped %d, want 2", n)
	}
	select {
	case <-a.Closed():
	default:
		t.Fatal("reaped session not closed")
	}
	if _, ok := st.Get(a.ID); ok {
		t.Fatal("reaped session still in store")
	}
}
