# Changelog

All notable changes to this project are documented here. Format follows
[Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/) and the
project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

The Go binary (`swsrs`), the Docker image, and the TypeScript SDK
(`@emdzej/swsrs-client`) MAY be versioned independently, but starting with
0.2.0 they ship together at the same version where possible.

## [Unreleased]

_Nothing yet._

## [0.2.1] — 2026-06-03

### Added — CORS on the HTTP surfaces

- `/admin/*` and `/.well-known/swsrs-config` now emit proper CORS
  headers when the request `Origin` matches `SWSRS_ALLOWED_ORIGINS`.
  Same configuration as the WebSocket data plane — one allowlist for
  both surfaces. Glob host patterns supported (`app.example.com`,
  `*.example.com`, `localhost:*`).
- `OPTIONS` preflight requests short-circuit with `Access-Control-
  Allow-Methods`, `Access-Control-Allow-Headers` (echoed back),
  `Access-Control-Allow-Credentials: true`, and a 10-minute
  `Access-Control-Max-Age`.
- Disallowed origins fall through silently (no CORS headers set,
  browsers block the response). `Vary: Origin` is always emitted to
  prevent cache poisoning across origins.
- Empty `SWSRS_ALLOWED_ORIGINS` keeps the previous behavior (no CORS
  headers; same-origin only).

## [0.2.0] — 2026-06-03

First unified release: server binary, Go SDK, npm package
(`@emdzej/swsrs-client`), and Docker image all ship at 0.2.0. The npm
package previously released a 0.1.0 preview with just the relay client;
0.2.0 adds the OIDC discovery + device-flow auth helpers, aligns release
tag conventions across all artifacts, and is the first release of the
server binary, the Go SDK, and the Docker image.

### Added — Relay server (`swsrs`)

- `serve` subcommand running the relay on a single HTTP port.
- Two-plane auth:
  - **Admin plane** (`/admin/sessions`) — OIDC JWT with JWKS
    auto-discovery from `SWSRS_OIDC_ISSUER`, audience check against
    `SWSRS_OIDC_AUDIENCE`, scope-gated routes
    (`swsrs:session:{create,read,delete}`). Supports both `scope`
    (RFC 8693) and `scp` (Azure-style) claim shapes.
  - **Data plane** (`/relay/{id}`) — opaque 128-bit per-slot tokens
    minted by the admin API. Constant-time comparison. Token via
    `Authorization: Bearer` or `?token=` query (browser fallback).
- Session lifecycle: two-slot rendezvous (initiator + responder),
  configurable TTL, peer-wait timeout, periodic reaper, graceful
  disconnect propagation between peers.
- TLS modes: off (external termination) or BYO via `--tls-cert` /
  `--tls-key`.
- `--no-auth` flag for local development — disables OIDC entirely and
  logs a loud warning.
- `GET /.well-known/swsrs-config` (public) so clients can discover the
  IdP without hard-coded coordinates. Returns issuer, audience,
  scopes, IdP endpoints (`authorization_endpoint`, `token_endpoint`,
  `device_authorization_endpoint`) and the optional shared
  `client_id_hint`. 404s when the server is running `--no-auth`.
- `--oidc-client-id` / `SWSRS_OIDC_CLIENT_ID` configures the shared
  OAuth client_id surfaced via discovery.
- CLI adapters built on the Go SDK:
  - `swsrs auth` — runs OIDC device flow (RFC 8628) against the IdP
    surfaced by discovery; persists token to a credentials file.
    `--logout` clears the cache.
  - `swsrs create` — admin client; auto-loads from credentials cache
    when `--oidc-token` is omitted; outputs JSON or eval-able env.
  - `swsrs tcp-listen` — accept local TCP, tunnel through the relay.
  - `swsrs tcp-dial` — receive relayed connection, dial local TCP.
  - `swsrs raw` — bridge stdin/stdout to a session.
  - `swsrs version` — print build metadata.
- Structured JSON logging via `log/slog`, graceful shutdown on
  SIGINT/SIGTERM, `/healthz` endpoint.

### Added — Go SDK (`github.com/emdzej/swsrs/pkg/client`)

- `client.Dial` / `client.Accept` returning a `*client.Conn` that
  implements `net.Conn`. Plugs into `grpc.WithContextDialer`,
  `http.Transport.DialContext`, `crypto/tls.Client`, etc.
- Same `Conn` exposes `Send` / `Recv` for frame-preserving
  (datagram-style) traffic on the same underlying WebSocket.
- `client.Admin` with `CreateSession`, `GetSession`, `ListSessions`,
  `DeleteSession`. Per-request `TokenSource` callback for refreshable
  OIDC tokens.
- WS keepalive pings (30 s default, configurable, can be disabled).
- URL normalization — accepts `http(s)://` and `ws(s)://`, upgrades
  scheme for the WebSocket dial.
- New `pkg/client/auth` subpackage:
  - `auth.Discover(ctx, relayURL)` → parsed `*Config`, with
    `ErrAuthDisabled` sentinel for `--no-auth` servers.
  - `cfg.DeviceLogin(ctx, opts)` runs RFC 8628 device flow via
    `golang.org/x/oauth2`; `OnPrompt` callback for caller-side
    rendering.
  - `TokenStore` interface + `FileTokenStore` (atomic write, mode
    0600, OS-appropriate default path).
  - `AdminTokenSource(cfg, store)` produces a refresh-aware
    `client.TokenSource`. Errors when refresh fails or no token is
    cached — never silently re-prompts.

### Added — TypeScript SDK (`@emdzej/swsrs-client` 0.2.0)

Builds on the 0.1.0 preview (which shipped the basic SDK surface).
0.2.0 additions:

- `discoverConfig` and `deviceLogin` on the main entry. Hand-rolled
  RFC 8628 polling with proper `authorization_pending` / `slow_down`
  / `access_denied` / `expired_token` handling and `AbortSignal`
  support. Browser CORS caveat documented.
- `MemoryTokenStore`, `AuthDisabledError`, and full type surface
  (`RelayConfig`, `TokenResponse`, `DevicePrompt`, `DeviceLoginOptions`,
  `TokenStore`).
- New subpath export `@emdzej/swsrs-client/node` with `FileTokenStore`
  and `defaultCredentialsPath()` using `node:fs` — atomic write, mode
  0600, OS-appropriate default path.

### Added — Examples

- `examples/chat` — minimal two-party text chat over a relay session,
  Node + TypeScript, `commander`-based CLI. Demonstrates end-to-end
  use of the TS SDK from session creation through bidirectional
  messaging.

### Added — Distribution & release pipeline

- Multi-stage `Dockerfile` (distroless/static, non-root, about 7 MB on
  ARM64).
- `docker-compose.yml` for local runs with env-driven config.
- GoReleaser cross-builds: `linux/{amd64,arm64}`,
  `darwin/{amd64,arm64}`, `windows/amd64`. Stripped, trimpath,
  version metadata injected via ldflags. `tar.gz` (Unix) / `zip`
  (Windows) archives plus `SHA256SUMS`.
- GitHub Actions:
  - `ci.yml` — Go build/vet/test/gofmt + TS workspace build +
    vitest + end-to-end chat smoke on every push and PR.
  - `docker.yml` — multi-arch image build on every push to `main`
    (verification only); push to `ghcr.io/emdzej/swsrs` on
    v-prefixed semver tags and on manual `workflow_dispatch`.
  - `release.yml` — GoReleaser cross-build attached to the GitHub
    Release on v-prefixed semver tags. `workflow_dispatch` supports
    snapshot dry runs.
  - `publish.yml` — npm Trusted Publishing (OIDC, no `NPM_TOKEN`)
    triggered by `npm-v<X.Y.Z>` release tags. Skips cleanly if the
    version is already published.
- `scripts/smoke-chat.sh` — self-contained end-to-end test runnable
  locally and in CI.

### Added — Testing

- Go: integration tests for `pkg/client` (round-trip, unauthorized,
  send/recv boundaries), `pkg/client/auth` (Discover happy/disabled/
  error, FileTokenStore round-trip), and `internal/discovery`
  (handler against fake IdP, `--no-auth` 404).
- TypeScript: Vitest with 24 unit tests covering `discoverConfig`
  (happy / 404 / 5xx / trailing-slash), `deviceLogin` (all RFC 8628
  outcomes including `slow_down` and `AbortSignal`), `AdminClient`
  (auth header, token rotation, error mapping, URL encoding),
  `MemoryTokenStore`, and `FileTokenStore` (round-trip, mode 0600,
  parent-dir creation, idempotent clear).

### Changed — Release tag convention

- Git tags for the binary, Docker image, Go module, **and the npm
  package** are **`vX.Y.Z`** (v-prefixed, matching Go module
  conventions). A single tag triggers all four release pipelines.
- Docker image tags remain bare (`1.2.3`); only the *git* tag carries
  the `v`.

### Known limitations

- **No transparent reconnect** within the peer-wait grace window —
  the drop surfaces as a read/write error and callers must redial
  with the same token.
- **No server-side WS pings** — the Go SDK pings; browsers can't
  (no `WebSocket.ping()` API). Long-lived browser ↔ Go sessions
  rely on the Go side to keep the connection live.
- **Mixed views** — using `Read`/`Write` and `Send`/`Recv` together
  on the same `Conn` is not supported.
- **Single port** for admin + data plane. A flood of relay traffic
  could starve admin requests; split into separate listeners if
  needed.

## [`@emdzej/swsrs-client` 0.1.0]

Preview release of the TypeScript SDK on npm. Shipped the basic
client surface — `AdminClient`, `dial` / `accept` returning a
`PeerConnection`, and the session types. Auth helpers and the
`/node` subpath landed in 0.2.0.

No corresponding server / Go binary / Docker release at this version.

[Unreleased]: https://github.com/emdzej/swsrs/compare/v0.2.1...HEAD
[0.2.1]: https://github.com/emdzej/swsrs/releases/tag/v0.2.1
[0.2.0]: https://github.com/emdzej/swsrs/releases/tag/v0.2.0
[`@emdzej/swsrs-client` 0.1.0]: https://www.npmjs.com/package/@emdzej/swsrs-client/v/0.1.0
