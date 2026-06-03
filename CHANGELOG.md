# Changelog

All notable changes to this project are documented here. Format follows
[Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/) and the
project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

The Go binary (`swsrs`) and the TypeScript SDK (`@emdzej/swsrs-client`) are
versioned independently. Entries below note which artifact each change
applies to.

## [Unreleased]

### Changed — Release tag convention

- Git tags for the binary, Docker image, and Go module are now
  **`vX.Y.Z`** (was bare `X.Y.Z`). This matches Go module
  conventions — `go get github.com/emdzej/swsrs/pkg/client@vX.Y.Z`
  now resolves cleanly. Docker image tags remain bare (`1.2.3`); only
  the *git* tag carries the `v`.
- npm package release tags are now **`npm-vX.Y.Z`** (was `npm-X.Y.Z`),
  keeping the distinction from Go/Docker release tags explicit.

### Added — Client auth flow (so clients don't need IdP coordinates)

- **Server** — `GET /.well-known/swsrs-config` returns the relay's issuer,
  audience, scopes, IdP endpoints (`authorization_endpoint`,
  `token_endpoint`, `device_authorization_endpoint`) and an optional
  shared `client_id_hint`. Public; gated to 404 when the server runs
  `--no-auth`.
- **Server** — new `--oidc-client-id` flag / `SWSRS_OIDC_CLIENT_ID` env
  for surfacing the shared OAuth client_id via discovery.
- **Go SDK** — new `pkg/client/auth` subpackage:
  - `auth.Discover(ctx, relayURL)` → parsed `*Config`, with
    `ErrAuthDisabled` sentinel for `--no-auth` servers.
  - `cfg.DeviceLogin(ctx, opts)` runs RFC 8628 device flow via
    `golang.org/x/oauth2`; `OnPrompt` callback for caller-side rendering.
  - `TokenStore` interface + `FileTokenStore` (atomic write, mode 0600,
    OS-appropriate default path).
  - `AdminTokenSource(cfg, store)` produces a refresh-aware
    `client.TokenSource` for `client.Admin.Token`. Errors when refresh
    fails or no token is cached — never silently re-prompts.
- **CLI** — new `swsrs auth` subcommand drives discovery + device flow,
  saves credentials. `--logout` clears the cached file. `swsrs create`
  now auto-loads from the credentials cache when `--oidc-token` is omitted.
- **TS SDK** — new `discoverConfig`, `deviceLogin`, `MemoryTokenStore`,
  and `TokenStore`/`RelayConfig`/`TokenResponse`/`DevicePrompt`/
  `DeviceLoginOptions` exports on the main entry. `AuthDisabledError`
  sentinel for `--no-auth` servers. New subpath export
  `@emdzej/swsrs-client/node` provides `FileTokenStore` and
  `defaultCredentialsPath()` using `node:fs`.

### Added — Testing

- **Vitest** in `clients/typescript/`. 24 unit tests covering
  `discoverConfig` (happy / 404 / 5xx / trailing-slash), `deviceLogin`
  (all RFC 8628 outcomes including `authorization_pending`, `slow_down`,
  `access_denied`, `expired_token`, abort), `AdminClient`
  (auth header, token rotation, error mapping, URL encoding),
  `MemoryTokenStore`, and `FileTokenStore` (round-trip, mode 0600,
  parent-dir creation, idempotent clear). Wired into CI.

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

- Multi-stage `Dockerfile` (distroless/static, non-root, about 7 MB on ARM64).
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

[Unreleased]: https://github.com/emdzej/swsrs/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/emdzej/swsrs/releases/tag/v0.1.0
