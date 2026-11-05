package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/emdzej/swsrs/internal/session"
)

func call(t *testing.T, srv *httptest.Server, method, path, body string) (int, map[string]any) {
	t.Helper()
	var rd *strings.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	var req *http.Request
	if rd != nil {
		req, _ = http.NewRequest(method, srv.URL+path, rd)
	} else {
		req, _ = http.NewRequest(method, srv.URL+path, nil)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	out := map[string]any{}
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

// A session's own options at creation (no body: the server's), within the
// server's limits; a deleted one stays readable, with why it ended.
func TestCreateOptions(t *testing.T) {
	store := session.NewStore(time.Hour)
	store.Retention = time.Minute
	mux := http.NewServeMux()
	(&API{Store: store, MaxPeerWait: 2 * time.Minute}).Register(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	code, plain := call(t, srv, "POST", "/admin/sessions", "")
	if code != 201 || plain["label"] != nil || plain["initiator_token"] == nil {
		t.Fatalf("no body: %d %v", code, plain)
	}
	code, out := call(t, srv, "POST", "/admin/sessions", `{"ttl_seconds": 600, "peer_wait_seconds": 60, "label": "alias:nip:42", "single_attach": true}`)
	if code != 201 || out["label"] != "alias:nip:42" || out["single_attach"] != true {
		t.Fatalf("options: %d %v", code, out)
	}
	created, _ := time.Parse(time.RFC3339Nano, out["created_at"].(string))
	expires, _ := time.Parse(time.RFC3339Nano, out["expires_at"].(string))
	if d := expires.Sub(created); d < 599*time.Second || d > 601*time.Second {
		t.Fatalf("ttl: %v", d)
	}
	id := out["id"].(string)
	if _, st := call(t, srv, "GET", "/admin/sessions/"+id, ""); st["peer_wait_seconds"] != float64(60) || st["state"] != "pending" {
		t.Fatalf("status: %v", st)
	}
	for _, bad := range []string{`{"ttl_seconds": 7200}`, `{"peer_wait_seconds": 600}`, `{"ttl_seconds": -1}`, `{"label": "` + strings.Repeat("x", 300) + `"}`, `{"unknown": 1}`, `not json`} {
		if code, out := call(t, srv, "POST", "/admin/sessions", bad); code != 400 || out["error"] == nil {
			t.Errorf("%.40s: %d %v", bad, code, out)
		}
	}
	if code, _ := call(t, srv, "DELETE", "/admin/sessions/"+id, ""); code != 204 {
		t.Fatalf("delete: %d", code)
	}
	if code, st := call(t, srv, "GET", "/admin/sessions/"+id, ""); code != 200 || st["state"] != "closed" || st["close_reason"] != "deleted" {
		t.Fatalf("after delete: %d %v", code, st)
	}
}
