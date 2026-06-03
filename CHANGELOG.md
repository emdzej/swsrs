# Changelog

All notable changes to this project are documented here. Format follows
[Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/) and the
project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

The Go binary (`swsrs`) and the TypeScript SDK (`@emdzej/swsrs-client`) are
versioned independently. Entries below note which artifact each change
applies to.

## [Unreleased]

_Nothing yet._

## [0.1.0] — 2026-06-03

Initial release. swsrs ships as a single Go binary, a Docker image, a Go SDK,
and a TypeScript SDK. Production-shaped from day one (OIDC, scopes, TLS), but
not yet API-stable — expect breaking changes before 1.0.

### Added — Relay server (`swsrs`)

- `serve` subcommand running the relay on a single HTTP port.
- Two-plane auth model:
  - **Admin plane** (`/admin/sessions`) — OIDC JWT verification with JWKS
    auto-discovery from `SWSRS_OIDC_ISSUER`, audience check against
    `SWSRS_OIDC_AUDIENCE`, scope-gated routes
    (`swsrs:session:{create,read,delete}`). Supports both `scope`
    (RFC 8693) and `scp` (Azure-style) claim shapes.
  - **Data plane** (`/relay/{id}`) — opaque 128-bit per-slot tokens minted
    by the admin API. Constant-time comparison. Token via `Authorization:
    Bearer` or `?token=` query (browser fallback).
- Session lifecycle: two-slot rendezvous (initiator + responder),
  configurable TTL, peer-wait timeout, periodic reaper of expired
  sessions, graceful disconnect propagation between peers.
- TLS modes: off (external termination) or BYO via `--tls-cert` /
  `--tls-key`.
- `--no-auth` flag for local development — disables OIDC entirely and
  logs a loud warning.
- CLI adapters built on the Go SDK:
  - `swsrs create` — admin client; outputs JSON or eval-able env
  - `swsrs tcp-listen` — accept local TCP, tunnel through the relay
  - `swsrs tcp-dial` — receive relayed connection, dial local TCP target
  - `swsrs raw` — bridge stdin/stdout to a session (scripting + debug)
  - `swsrs version` — print build metadata
- Structured JSON logging via `log/slog`, graceful shutdown on
  SIGINT/SIGTERM, `/healthz` endpoint.

### Added — Go SDK (`github.com/emdzej/swsrs/pkg/client`)

- `client.Dial` / `client.Accept` returning a `*client.Conn` that
  implements `net.Conn`. Plugs into `grpc.WithContextDialer`,
  `http.Transport.DialContext`, `crypto/tls.Client`, etc.
- Same `Conn` exposes `Send` / `Recv` for frame-preserving (datagram-style)
  traffic on the same underlying WebSocket.
- `client.Admin` with `CreateSession`, `GetSession`, `ListSessions`,
  `DeleteSession`. Per-request `TokenSource` callback for refreshable
  OIDC tokens.
- WS keepalive pings (30 s default, configurable, can be disabled).
- URL normalization — accepts `http(s)://` and `ws(s)://`, upgrades scheme
  for the WebSocket dial.

### Added — TypeScript SDK (`@emdzej/swsrs-client`)

- `AdminClient` (fetch-based) with `createSession`, `getSession`,
  `listSessions`, `deleteSession`, async `TokenProvider` for refreshable
  tokens, full `AbortSignal` support.
- `dial` / `accept` returning a `PeerConnection` wrapping a native
  `WebSocket`, with `opened` / `closed` promises.
- Zero runtime dependencies. Works in browsers and Node 22+ via native
  `fetch` and `WebSocket`. ESM-only, with full TypeScript declarations
  and source maps.
- Token-as-query-param strategy (browsers can't set headers on WS
  upgrades); server accepts both forms.

### Added — Examples

- `examples/chat` — minimal two-party text chat over a relay session,
  Node + TypeScript, `commander`-based CLI. Demonstrates end-to-end use
  of the TS SDK from session creation through bidirectional messaging.

### Added — Distribution & release

- Multi-stage `Dockerfile` (distroless/static, non-root, ~7 MB on ARM64).
- `docker-compose.yml` for local runs with env-driven config.
- GoReleaser cross-builds: `linux/{amd64,arm64}`, `darwin/{amd64,arm64}`,
  `windows/amd64`. Stripped, trimpath, version metadata injected via
  ldflags. `tar.gz` (Unix) / `zip` (Windows) archives plus
  `SHA256SUMS`.
- GitHub Actions:
  - `ci.yml` — Go build/vet/test/gofmt + TS workspace build/typecheck +
    end-to-end smoke test on every push and PR.
  - `docker.yml` — multi-arch image push to `ghcr.io/emdzej/swsrs` on
    push to `main` and on plain-semver tags.
  - `release.yml` — GoReleaser cross-build attached to the GitHub Release
    on plain-semver tags. `workflow_dispatch` supports snapshot dry runs.
  - `npm-publish.yml` — npm Trusted Publishing (OIDC, no `NPM_TOKEN`)
    triggered by `npm-<X.Y.Z>` release tags. Skips cleanly if the
    version is already published.
- `scripts/smoke-chat.sh` — self-contained end-to-end test runnable
  locally and in CI.

### Known limitations

- **No transparent reconnect** within the peer-wait grace window — the
  drop surfaces as a read/write error and callers must redial with the
  same token.
- **No server-side WS pings** — the Go SDK pings; browsers can't (no
  `WebSocket.ping()` API). Long-lived browser ↔ Go sessions rely on the
  Go side to keep the connection live.
- **Mixed views** — using `Read`/`Write` and `Send`/`Recv` together on
  the same `Conn` is not supported.
- **Single port** for admin + data plane. A flood of relay traffic could
  starve admin requests; split into separate listeners if needed.

[Unreleased]: https://github.com/emdzej/swsrs/compare/0.1.0...HEAD
[0.1.0]: https://github.com/emdzej/swsrs/releases/tag/0.1.0
