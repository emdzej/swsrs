# Architecture

swsrs is a relay, not a VPN. Its single job: get bytes from one
authenticated peer to another, regardless of what those bytes mean.

## Surfaces

Two distinct HTTP surfaces, deliberately different:

| Surface | Path | Auth | Purpose |
|---|---|---|---|
| Admin API | `/admin/sessions` | OIDC JWT (JWKS auto-discovery, scope-gated) | Create / list / delete sessions |
| Data plane | `/relay/{id}` | Opaque per-slot token (minted by admin) | Two peers connect; opaque bytes flow between them |
| Discovery | `/.well-known/swsrs-config` | Public | Lets clients run their own OAuth flow without hard-coding the IdP |
| Health | `/healthz` | Public | Liveness probe |

This split is load-bearing. See [Authentication](/guide/auth) for the why.

## Session lifecycle

A **session** is a two-slot rendezvous:

```
pending  ──peer attaches──▶  half_open  ──counterpart attaches──▶  open
   │                              │                                 │
   │                              │                                 │
   └──TTL expires──▶  closed  ◀──peer disconnects + no reconnect────┘
```

- States: `pending` → `half_open` → `open` → `closed`.
- Each session has an `initiator` and a `responder` slot. Names express
  caller intent; the wire is symmetric.
- TTL (default 1h) bounds session lifetime regardless of activity.
- `peer_wait` (default 2m) bounds how long the first-arriving peer waits
  for its counterpart.
- A periodic reaper sweeps expired sessions.

The slot model means **one TCP/UDP connection per session**. For N
concurrent tunnels, mint N sessions. Multiplexing is a client-side
concern — it would mean inspecting frames, which the server doesn't do.

## Protocol-agnostic relay

The server treats every session as a single stream of opaque WebSocket
binary frames. It does not parse, inspect, or modify payloads:

- Adding TCP semantics? Client-side (`swsrs tcp-listen` / `tcp-dial`).
- Adding UDP semantics? Client-side (one WS frame == one datagram).
- Adding gRPC, SSH, HTTP, TLS? The Go SDK gives you a `net.Conn`; drop
  it into any library that takes one.

This is why the binary stays so small: the relay's threat model is
trivial (it sees ciphertext-equivalent blobs), and adding new protocols
never requires a server release.

## Backpressure

The forwarding loop is synchronous per direction. A slow peer creates
backpressure on the fast peer (the relay won't buffer unbounded). If a
peer's WebSocket write blocks, both sides eventually close. No
in-memory queue per-session means no OOM surprises on a `t4g.nano`.

## What's NOT built in

Worth being explicit about scope:

- **No transparent reconnect** within the peer-wait grace window — the
  drop surfaces as a read/write error and callers redial with the same
  token.
- **No server-side WS pings** — the Go SDK pings; browsers can't.
- **No transparent multi-stream multiplexing** — one TCP/UDP per session.
- **No per-user identity on the data plane** — slot tokens are opaque,
  not JWTs. If you need an audit trail of who connected, your control
  plane records it when it hands out tokens.
- **Single port** for admin + data plane. A surge of relay traffic could
  starve admin requests; if that's a real concern, run two listeners
  behind a reverse proxy.

These are deliberate; they keep the surface auditable and the binary
tiny. Add layers above the relay where they belong.
