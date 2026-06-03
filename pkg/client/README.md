# swsrs Go client

Go SDK for the [swsrs](https://github.com/emdzej/swsrs) relay. Two APIs:

- **Admin** — create / list / delete sessions (OIDC bearer)
- **Peer** — `Dial` / `Accept` returning a `net.Conn` over the relay
  WebSocket; same connection also exposes frame-preserving `Send` / `Recv`

## Install

```bash
go get github.com/emdzej/swsrs/pkg/client
```

Requires Go 1.22+.

## Admin client

```go
import (
    "context"
    "github.com/emdzej/swsrs/pkg/client"
)

admin := &client.Admin{
    BaseURL: "https://relay.example.com",
    Token:   client.StaticToken(oidcToken),  // or your refresh-aware TokenSource
}

sess, err := admin.CreateSession(ctx)
// sess.ID, sess.InitiatorToken, sess.ResponderToken, sess.ExpiresAt

status, err := admin.GetSession(ctx, sess.ID)
sessions, err := admin.ListSessions(ctx)
err = admin.DeleteSession(ctx, sess.ID)
```

`TokenSource` is invoked per request, so production callers can return a
refreshed token without rebuilding the client:

```go
admin := &client.Admin{
    BaseURL: "https://relay.example.com",
    Token: func(ctx context.Context) (string, error) {
        return tokenProvider.Refresh(ctx) // your impl
    },
}
```

## Peer client

`Dial` and `Accept` are wire-identical — the names express caller intent.
Both return a `*client.Conn` that satisfies `net.Conn`.

```go
conn, err := client.Dial(ctx, client.DialOptions{
    RelayURL:  "wss://relay.example.com",
    SessionID: sess.ID,
    Token:     sess.InitiatorToken,
})
defer conn.Close()

// It's a net.Conn — drop into anything that takes one.
io.Copy(localSocket, conn)
io.Copy(conn, localSocket)
```

### Use with gRPC

```go
import "google.golang.org/grpc"

relayConn, _ := client.Dial(ctx, opts)
gc, _ := grpc.DialContext(ctx, "relay",
    grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
        return relayConn, nil
    }),
    grpc.WithTransportCredentials(insecure.NewCredentials()),
)
```

### Use with `net/http`

```go
relayConn, _ := client.Dial(ctx, opts)
tr := &http.Transport{
    DialContext: func(context.Context, string, string) (net.Conn, error) {
        return relayConn, nil
    },
}
hc := &http.Client{Transport: tr}
hc.Get("http://probe/diagnostics")
```

### Frame-preserving view

For datagram-style traffic, use the same connection's `Send` / `Recv`:

```go
conn.Send(ctx, []byte("frame-one"))
conn.Send(ctx, []byte("frame-two"))

msg, _ := conn.Recv(ctx)  // returns exactly one WS message
```

Don't mix `Read`/`Write` with `Send`/`Recv` on the same connection —
buffering across views is undefined.

## Options

```go
client.DialOptions{
    RelayURL:         "wss://relay.example.com", // http(s):// is auto-upgraded
    SessionID:        "...",
    Token:            "...",
    HTTPClient:       customClient,    // optional, for proxies / custom roots
    Keepalive:        30*time.Second,  // 0 = default, negative = disabled
    HandshakeTimeout: 10*time.Second,  // 0 = default
}
```

## Known limitations

- **No transparent reconnect** within the peer-wait grace window. A dropped WS
  surfaces as a read/write error; caller redials with the same token.
- **Mixed views** — using `Read`/`Write` together with `Send`/`Recv` on the
  same `Conn` is not supported.

## License

MIT.
