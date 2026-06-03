# swsrs — simple websocket relay service

A minimal, self-hostable WebSocket relay that lets two parties behind NAT or
firewalls communicate over a single bidirectional tunnel. Similar in spirit
to [wstunnel](https://github.com/erebe/wstunnel), but with orchestrated
sessions and a clean separation between the **admin plane** (OIDC-protected)
and the **data plane** (opaque per-slot tokens).

```
                ┌────────────────────┐
                │   swsrs (cloud)    │
                │  one small binary  │
                └─────────▲──────────┘
                          │ wss://
            ┌─────────────┴─────────────┐
            │                           │
       outbound WS                  outbound WS
            │                           │
   ┌────────┴────────┐         ┌────────┴────────┐
   │   peer A        │         │   peer B        │
   │  (behind NAT)   │ <─────> │  (behind NAT)   │
   └─────────────────┘  relay  └─────────────────┘
```

## Why

- **Cheap to run** — single static Go binary, about 7 MB on ARM. Designed to
  fit on a `t4g.nano` (around $3/mo).
- **Protocol-agnostic** — the server forwards opaque WebSocket binary frames.
  TCP/UDP semantics, gRPC, SSH, anything — handled by client-side adapters or
  the SDK, never the server.
- **Embeddable** — first-class Go and TypeScript SDKs so your app can be a
  relay peer without anyone installing a CLI.

## Architecture

Two distinct HTTP surfaces with deliberately different auth:

| Surface | Path | Auth | Purpose |
|---|---|---|---|
| Admin API | `/admin/sessions` | OIDC JWT (JWKS auto-discovery, scope-gated) | Create / list / delete sessions |
| Data plane | `/relay/{id}` | Opaque per-slot token (minted by admin) | Two peers connect; opaque bytes flow between them |

A session has two slots — `initiator` and `responder`. The admin API mints a
session id and a token per slot; the parties present those tokens on the
WebSocket upgrade. Once both attach, frames are pumped verbatim.

## Components

- **`cmd/swsrs/`** — single binary with subcommands (`serve`, `create`,
  `tcp-listen`, `tcp-dial`, `raw`)
- **`pkg/client/`** — Go SDK ([docs](pkg/client/README.md))
- **`clients/typescript/`** — `@emdzej/swsrs-client` for browser + Node 22+
  ([docs](clients/typescript/README.md))
- **`examples/chat/`** — runnable two-party text-chat demo using the TS SDK
  ([docs](examples/chat/README.md))

## Quickstart

### Run the server

```bash
SWSRS_OIDC_ISSUER=https://your-idp.example.com \
SWSRS_OIDC_AUDIENCE=swsrs \
go run ./cmd/swsrs serve
```

Or via Docker:

```bash
docker run --rm -p 8080:8080 \
  -e SWSRS_OIDC_ISSUER=https://your-idp.example.com \
  -e SWSRS_OIDC_AUDIENCE=swsrs \
  ghcr.io/emdzej/swsrs:latest
```

Or with docker-compose (see `docker-compose.yml`).

### Tunnel SSH end-to-end

On a machine that already has the OIDC token:

```bash
SWSRS_ADMIN_URL=https://relay.example.com \
SWSRS_OIDC_TOKEN=eyJ... \
eval $(swsrs create --output env)
echo $SWSRS_INITIATOR_TOKEN  # → give to peer A (out-of-band)
echo $SWSRS_RESPONDER_TOKEN  # → give to peer B
```

On peer A (the machine running sshd):

```bash
swsrs tcp-dial \
  --url wss://relay.example.com \
  --session $SWSRS_SESSION --token $SWSRS_RESPONDER_TOKEN --role responder \
  --target 127.0.0.1:22
```

On peer B (the one connecting):

```bash
swsrs tcp-listen \
  --url wss://relay.example.com \
  --session $SWSRS_SESSION --token $SWSRS_INITIATOR_TOKEN --role initiator \
  --listen 127.0.0.1:2222 &
ssh -p 2222 user@127.0.0.1
```

### Embed in your app (Go)

```go
import "github.com/emdzej/swsrs/pkg/client"

admin := &client.Admin{BaseURL: "https://relay.example.com", Token: client.StaticToken(oidcToken)}
sess, _ := admin.CreateSession(ctx)
// distribute sess.ResponderToken to the other side via your control plane

conn, _ := client.Dial(ctx, client.DialOptions{
    RelayURL: "wss://relay.example.com",
    SessionID: sess.ID,
    Token: sess.InitiatorToken,
})
// conn is a net.Conn — plug into grpc.WithContextDialer, http.Transport, etc.
```

### Embed in your app (TypeScript / browser)

```ts
import { AdminClient, dial } from "@emdzej/swsrs-client";

const admin = new AdminClient({ baseURL: "https://relay.example.com", token: async () => oidcToken });
const sess = await admin.createSession();
// distribute sess.responder_token to the other side

const conn = await dial({
  relayURL: "wss://relay.example.com",
  sessionId: sess.id,
  token: sess.initiator_token,
});
conn.socket.addEventListener("message", (e) => console.log(e.data));
conn.send(new Uint8Array([1, 2, 3]));
```

## Configuration

All options accept either env vars or flags. Flags override env.

| Env / Flag | Default | Description |
|---|---|---|
| `SWSRS_ADDR` / `--addr` | `:8080` | Listen address |
| `SWSRS_OIDC_ISSUER` / `--oidc-issuer` | *(required)* | OIDC issuer URL (autodiscovery) |
| `SWSRS_OIDC_AUDIENCE` / `--oidc-audience` | — | Expected `aud` claim (client_id) |
| `SWSRS_SESSION_TTL` / `--session-ttl` | `1h` | Maximum session lifetime |
| `SWSRS_PEER_WAIT` / `--peer-wait` | `2m` | How long a peer waits for its counterpart |
| `SWSRS_REAP_INTERVAL` / `--reap-interval` | `30s` | Expired-session sweep cadence |
| `SWSRS_PUBLIC_BASE_URL` / `--public-base-url` | — | Public ws(s) URL embedded in admin responses |
| `SWSRS_ALLOWED_ORIGINS` | — | Comma-separated allowed `Origin` values |
| `SWSRS_TLS_CERT` / `--tls-cert` | — | PEM cert; with `--tls-key` enables in-process TLS |
| `SWSRS_TLS_KEY` / `--tls-key` | — | PEM key |
| `SWSRS_NO_AUTH` / `--no-auth` | `false` | **Dev only** — disable OIDC verification on the admin API |

## Authentication & authorization

swsrs has **two independent auth domains**, by design:

```
┌──────────────────────────────────┐    ┌──────────────────────────────────┐
│ Admin plane  /admin/sessions     │    │ Data plane  /relay/{id}          │
│                                  │    │                                  │
│  OIDC JWT (Bearer)               │    │  Opaque per-slot token (Bearer   │
│   ├─ JWKS auto-discovery from    │    │   or ?token= query)              │
│   │  $SWSRS_OIDC_ISSUER          │    │                                  │
│   ├─ Audience checked against    │    │  Minted by the admin API at      │
│   │  $SWSRS_OIDC_AUDIENCE        │    │  session creation; 128-bit       │
│   └─ Scope check per route       │    │  random; constant-time compared  │
│                                  │    │                                  │
│  Operators / control planes      │    │  Peers (machines, browsers)      │
│  authenticate with the IdP       │    │  authenticate with the token     │
└──────────────────────────────────┘    └──────────────────────────────────┘
```

**Why split?** Connecting peers may not have IdP identities — the canonical
use case is a support engineer (with an IdP account) tunneling to a customer
machine (no IdP account). The IdP gates *who can create sessions*; the
relay gates *who can use a specific session*. Keeps the data-plane hot path
dependency-free (no JWKS roundtrip per WS upgrade).

### Admin plane — OIDC JWT, scope-gated

Every admin request must carry `Authorization: Bearer <jwt>`. The JWT is
verified using JWKS auto-discovered from `$SWSRS_OIDC_ISSUER`'s
`.well-known/openid-configuration`. The `aud` claim must match
`$SWSRS_OIDC_AUDIENCE` (skip the check by leaving the env unset — local
dev only).

Scopes are pulled from either the standard `scope` claim (space-delimited
string, RFC 8693) or `scp` (array, Azure-style). Required scope per route:

| Method & Path | Required scope |
|---|---|
| `POST   /admin/sessions`       | `swsrs:session:create` |
| `GET    /admin/sessions`       | `swsrs:session:read`   |
| `GET    /admin/sessions/{id}`  | `swsrs:session:read`   |
| `DELETE /admin/sessions/{id}`  | `swsrs:session:delete` |

A typical operator role bundles all three; a control-plane service account
that only mints sessions needs just `swsrs:session:create`. Tokens missing
the required scope get `403 Forbidden`; missing or invalid tokens get
`401 Unauthorized`.

### Data plane — opaque per-slot tokens

The admin API's `POST /admin/sessions` returns:

```json
{
  "id": "OVMnxnEzZA9QvFc...",
  "initiator_token": "Gi-jvZfpHgahXc618jUP8A",
  "responder_token": "tKuylZcPfwjvy43...",
  "expires_at": "..."
}
```

These tokens are **not JWTs and do not carry scopes**. They are 128-bit
random strings, each bound to a single slot of a single session. To attach
on the data plane:

```http
GET /relay/{id} HTTP/1.1
Authorization: Bearer <initiator_token-or-responder_token>
Upgrade: websocket
```

Browsers can't set the `Authorization` header on a WS upgrade, so the
data-plane also accepts `?token=<token>` as a fallback. The TS SDK uses
this form. Always serve `wss://` so tokens don't leak in plaintext.

What a token grants:
- Attach to **its slot only** (initiator or responder). The other token
  attaches to the other slot. A token mismatched against a slot is
  rejected with `401`.
- For the lifetime of the session and within `--peer-wait` after a drop —
  the slot is reusable for reconnect, but never *concurrently* held by
  more than one connection.
- No admin-plane access. Data-plane tokens cannot create, list, or delete
  sessions.

Once the session is closed or `expires_at` passes, the tokens are useless.

### Local development — `--no-auth`

For local testing without standing up an IdP:

```bash
swsrs serve --no-auth --addr :8080
# WARN: AUTH DISABLED — admin API is open. Do not use in production.
```

Effect: admin routes skip OIDC verification and scope checks entirely.
Data-plane tokens still work the same way (they aren't OIDC-related). Never
run with `--no-auth` exposed to a network you don't trust.

## TLS

Two modes, controlled by whether `--tls-cert` and `--tls-key` are set:

- **off** (default) — plain HTTP; assume an upstream gateway (ALB, Cloudflare,
  Caddy) terminates TLS
- **BYO** — pass both flags; the binary uses `ListenAndServeTLS`

In either case, the data plane and admin plane **must** be reached over TLS
in production — session and OIDC tokens both travel in `Authorization`.

## Subcommands

```
swsrs serve           run the relay server
swsrs create          create a session (uses the admin API)
swsrs tcp-listen      accept local TCP and tunnel through the relay
swsrs tcp-dial        receive a relayed connection and dial a local TCP target
swsrs raw             bridge stdin/stdout to a relay session
swsrs version         print build information
```

Run `swsrs <command> --help` for command-specific flags.

## Project layout

```
cmd/swsrs/           binary + subcommands
internal/            relay server internals (not importable)
pkg/client/          Go SDK (importable)
clients/typescript/  npm package (@emdzej/swsrs-client)
examples/chat/       two-party text chat demo (Node, commander, TS SDK)
.github/workflows/   CI + release pipelines
```

## Local development

### Prerequisites

- Go 1.26+
- Node 22+ and [pnpm](https://pnpm.io) 10+ (only needed for the TS SDK and
  examples)

### Run the server without an IdP

Pass `--no-auth` to disable OIDC verification on the admin API. This is for
local testing only — it leaves session creation open to anyone. The server
logs a loud warning when started this way.

```bash
go run ./cmd/swsrs serve --no-auth --addr :8080
```

### Build the TS workspace

The TypeScript SDK and the examples form a pnpm workspace rooted at the repo
top level:

```bash
pnpm install            # one-time, from repo root
pnpm -r run build       # builds clients/typescript and examples/*
```

### One-shot end-to-end smoke test

```bash
bash scripts/smoke-chat.sh
```

Builds the binary, the TS workspace, starts the relay with `--no-auth`, runs
both sides of [`examples/chat`](examples/chat/README.md), and asserts each
side receives the other's message. Same script runs in CI.

## CI / release

- **`.github/workflows/ci.yml`** — Go (build/vet/test/gofmt) + TS workspace
  build + the end-to-end smoke test on every push and PR.
- **`.github/workflows/docker.yml`** — multi-arch (`linux/amd64`,
  `linux/arm64`) image to `ghcr.io/${{ github.repository }}` on push to
  `main` and on plain-semver tags (`1.2.3`, no `v` prefix).
- **`.github/workflows/release.yml`** — [GoReleaser](https://goreleaser.com):
  cross-platform binary archives (`linux/amd64`, `linux/arm64`,
  `darwin/amd64`, `darwin/arm64`, `windows/amd64`) attached to the GitHub
  Release on the same plain-semver tags. Includes `SHA256SUMS` and an
  auto-generated changelog. `workflow_dispatch` can produce a snapshot
  build without publishing.
- **`.github/workflows/npm-publish.yml`** — publishes
  `@emdzej/swsrs-client` to npm via [Trusted
  Publishing](https://docs.npmjs.com/trusted-publishers) (OIDC, no
  `NPM_TOKEN`) when a GitHub Release is published with a tag of the form
  `npm-<X.Y.Z>` (e.g. `npm-0.2.1`).

A single `git tag 1.2.3 && git push --tags` therefore produces, in
parallel:
  1. A GitHub Release with prebuilt binaries (`release.yml`)
  2. A multi-arch Docker image at `ghcr.io/emdzej/swsrs:1.2.3` (`docker.yml`)

The npm package is released independently via `npm-<X.Y.Z>` tags so the two
versioning streams stay decoupled.

## Releases

See [CHANGELOG.md](CHANGELOG.md) for the curated change list.

## License

MIT — see [LICENSE](LICENSE).
