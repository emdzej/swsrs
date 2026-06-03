# AGENTS.md

A briefing for AI agents (and humans new to the codebase) on what this
project is, how it's organized, and the conventions that aren't visible
from the file tree alone. Read this once and you should be productive
in minutes.

## What this is

**swsrs** — Simple WebSocket Relay Service. A minimal self-hostable
WebSocket relay that brokers a private rendezvous between two
authenticated peers behind NAT. The headline use case: **operator-side
software (your tuner UI, your support tool) needs live access to
customer-side software (your diagnostic head, your service agent)
running on a machine you don't control.**

Reference deployment in production: [Bimmerz
Connect](https://connect.bimmerz.app/) powering the
[bimmerz.app](https://bimmerz.app) BMW diagnostic & coding suite.

What it's NOT trying to be: a VPN, a reverse-proxy gateway, a media
SFU, a stream multiplexer, a pub/sub. See
[`docs/guide/comparison.md`](docs/guide/comparison.md) for the full map
of "use this instead when…".

## Architecture in one screen

Two distinct HTTP surfaces, deliberately different — the load-bearing
design decision:

| Surface | Path | Auth | Purpose |
|---|---|---|---|
| Admin   | `/admin/sessions`         | **OIDC JWT** (JWKS auto-discovery, scope-gated) | Create / list / delete sessions |
| Data    | `/relay/{id}` (WS upgrade) | **Opaque per-slot token** (128-bit, minted by admin) | Two peers attach; opaque bytes flow |
| Discovery | `/.well-known/swsrs-config` | Public | Lets clients run their own OAuth flow without IdP coords |
| Health  | `/healthz`                 | Public | Liveness |

Why split? The party who mints sessions has an IdP identity; the party
who *uses* the tunnel does not. Operators have OIDC accounts; their
customers don't need them. This split is in load-bearing memory across
the project — **do not** unify the two planes.

Session model: two-slot rendezvous (`initiator` + `responder`),
ephemeral, gated by TTL + peer-wait + an idle-but-not-implemented hook.

The relay is **protocol-agnostic**: it forwards opaque WebSocket binary
frames and never inspects payloads. TCP/UDP/gRPC/SSH semantics live
client-side via the SDKs or CLI adapters.

## Repo layout

```
cmd/swsrs/                  CLI binary — `serve`, `auth`, `create`,
                            `tcp-listen`, `tcp-dial`, `raw`, `version`
internal/admin/             Admin API handlers, scopes, route registration
internal/auth/              OIDC verifier (go-oidc); exposes IdP endpoints
                            for discovery
internal/relay/             WebSocket data-plane handler
internal/session/           In-memory session store, state machine, reaper
internal/discovery/         /.well-known/swsrs-config handler
internal/cors/              CORS middleware (HTTP); reuses SWSRS_ALLOWED_ORIGINS

pkg/client/                 PUBLIC Go SDK (peer + admin)
pkg/client/auth/            PUBLIC Go SDK auth helpers (Discover,
                            DeviceLogin, FileTokenStore, AdminTokenSource)

clients/typescript/         @emdzej/swsrs-client npm package (browser + Node 22+)
clients/typescript/src/     main entry (admin, peer, auth, device, store, types)
clients/typescript/test/    Vitest tests
                            Subpath: @emdzej/swsrs-client/node (FileTokenStore)

docs/                       VitePress site → https://swsrs.emdzej.pl
docs/.vitepress/config.ts   Nav / sidebar / mermaid plugin wiring
docs/guide/                 Tutorial-shaped pages
docs/reference/             API reference

examples/chat/              Two-party text chat (Node, commander, TS SDK)

scripts/smoke-chat.sh       End-to-end CI smoke test
.goreleaser.yml             Cross-build matrix
.github/workflows/          ci, docker, release, publish, pages
Dockerfile                  Multi-stage distroless build
```

The repo is a **pnpm workspace** (`pnpm-workspace.yaml`) covering
`clients/typescript`, `docs`, and `examples/*`. `pnpm install` from
the repo root pulls everything; `pnpm -r run build` builds all
packages.

## Conventions

### Go

- **Format strictly.** `gofmt -l .` must be clean. CI enforces this.
- **Minimal dependencies.** The runtime deps are `coder/websocket`,
  `go-oidc`, and `golang.org/x/oauth2`. Don't pull in heavy frameworks
  (no `gorilla/mux`, no `chi`, no `gin`). The stdlib `http.ServeMux`
  (Go 1.22 patterns) is what we use.
- **No comments that restate code.** Only comment WHY when it's
  non-obvious or load-bearing.
- **No premature abstraction.** Three similar lines beats a generic
  interface used once. Trust internal code.
- **Error handling at boundaries only.** Validate at HTTP entry, IdP
  responses, file I/O. Internal helpers can return raw errors.
- **Build flags injected by GoReleaser**: `main.version`,
  `main.commit`, `main.date`. Default to `dev / unknown / unknown` for
  local builds. `swsrs version` prints them.
- **`go test ./... -count=1`** is the test invocation. `-race` in CI.

### TypeScript

- **pnpm only.** `packageManager: pnpm@10.x` is pinned in
  `clients/typescript/package.json`. Don't introduce npm/yarn lockfiles.
- **ESM only.** `"type": "module"` everywhere. No CJS.
- **Zero runtime deps for the SDK.** The `@emdzej/swsrs-client`
  package uses native `fetch` and `WebSocket` from globals — works in
  browser and Node 22+. **Do not pull in `ws`, `axios`, `node-fetch`,
  or similar.** Adding a runtime dep needs a real justification.
- **`tsc --strict`.** Build is the typecheck — don't add a separate
  `tsc --noEmit` step.
- **Browser-safe main entry; Node-only subpath.** Anything that
  imports `node:*` lives under `clients/typescript/src/node.ts` and is
  exposed as `@emdzej/swsrs-client/node`. Keep the main entry
  bundler-friendly for browsers.
- **Vitest** for tests. `pnpm --filter @emdzej/swsrs-client test`.
- **`publishConfig.access: public`** is in package.json (scoped
  packages default to private on npm). `provenance` is NOT in
  publishConfig — it's CI-only because local publishes don't have
  OIDC.

### Docs

- **VitePress** in `docs/`. Mermaid via `vitepress-plugin-mermaid` for
  all diagrams — **no new ASCII art**. The README is allowed to use
  Mermaid too (GitHub renders it natively).
- **Custom domain:** `swsrs.emdzej.pl` via `docs/public/CNAME`. GitHub
  Pages settings + DNS are configured externally.
- **No emojis on the site.** Maintainer dislikes them. Functional
  glyphs (✓/✗) in support tables are tolerated, but skip decorative
  icons on hero / feature cards.
- **Avoid `~N` inline in markdown.** GFM strikethrough treats `~N ... ~M`
  as a span. Write "about 7 MB" instead of `~7 MB`.
- **CHANGELOG** uses Keep-a-Changelog format. `[Unreleased]` section
  at top; promote to a dated version section on release. Comparison
  links at bottom always reference `vX.Y.Z` tags.

### Git / releases

- **Single unified version stream.** Server binary, Docker image, Go
  module, and npm package all release together at `vX.Y.Z`. The npm
  package version in `clients/typescript/package.json` must match the
  tag (CI fails fast otherwise).
- **Tag format: `vX.Y.Z`** (v-prefixed, Go module convention). Image
  tag on GHCR is bare (`X.Y.Z`); only the git tag carries the `v`.
- **One tag triggers everything**:
  - `release.yml` → GoReleaser binaries + GitHub Release
  - `docker.yml` → multi-arch image push to `ghcr.io/emdzej/swsrs`
  - `publish.yml` → npm publish (skips cleanly if already published)
- **Conventional commits**, GoReleaser uses them to group changelog
  entries: `feat:`, `fix:`, `chore:`, `docs:`, `ci:`, `test:`. `chore:`,
  `docs:`, `test:` are excluded from the auto-generated release notes.
- **Never commit secrets**, `.env`, IdP credentials, JWTs. There are
  none in the repo today; keep it that way.
- **Workflow filenames are stable.** `publish.yml` is the workflow
  filename registered as a Trusted Publisher on npmjs.com. Renaming
  it breaks publishing until npm config is updated.

### Workflows / CI

`.github/workflows/ci.yml` runs on every push + PR:

1. Go: gofmt check, vet, build, test (`-race`).
2. TS: pnpm install (frozen lockfile), build all workspace packages
   (includes docs — broken docs fail CI), vitest, pack-dry-run.
3. End-to-end smoke: `scripts/smoke-chat.sh` (relay + chat example).

If you change something that touches user-facing behavior, run all
three locally before pushing:

```bash
gofmt -l .                           # must be empty
go vet ./...
go test ./... -count=1
pnpm install --frozen-lockfile
pnpm -r run build
pnpm --filter @emdzej/swsrs-client test
bash scripts/smoke-chat.sh           # ← the load-bearing check
```

## How to make a release

1. Bump `clients/typescript/package.json` version.
2. Promote `[Unreleased]` content to `[X.Y.Z] — YYYY-MM-DD` in
   `CHANGELOG.md`. Add a fresh `[Unreleased]` _Nothing yet._
3. Update compare/release links at the bottom of `CHANGELOG.md`.
4. Update version label in `docs/.vitepress/config.ts` (`text: "X.Y.Z"`
   in the nav dropdown).
5. Commit: `chore: bump to X.Y.Z (...)`.
6. `git push`.
7. `git tag vX.Y.Z && git push --tags`.

Verify after release: GitHub Release exists with binaries,
`ghcr.io/emdzej/swsrs:X.Y.Z` is pullable, `npm view
@emdzej/swsrs-client version` reports the new version.

## Footguns

- **CHANGELOG comparison links must use `vX.Y.Z` tags.** Go modules
  require the `v` prefix. Bare `X.Y.Z` tags would silently break
  `go get ...@vX.Y.Z` for the SDK.
- **`SWSRS_OIDC_AUDIENCE` unset = audience check is silently
  skipped.** Always set it in production. See
  `internal/auth/oidc.go`.
- **`SWSRS_ALLOWED_ORIGINS` matches `host:port`.** Use `localhost:*`
  for any port; bare `localhost` won't match `localhost:5173`. Same
  allowlist drives both WS Origin check and HTTP CORS.
- **`coder/websocket` default ReadLimit is 32 KB.** We disable it via
  `SetReadLimit(-1)` (configurable as `SWSRS_MAX_FRAME_SIZE`). Don't
  re-add a default limit thinking it's safer.
- **The Go SDK errors loudly on auth failure** — it does NOT silently
  re-run device flow. Callers handle the error explicitly. Keep this
  contract; libraries that grab stdin / pop browsers are surprising.
- **Browser device flow doesn't work** — most IdPs don't enable CORS
  on token endpoints. Document this clearly; don't try to "fix" it on
  the SDK side. Browser apps should run auth-code + PKCE with a real
  OAuth library and hand swsrs the access token.
- **Google IdP doesn't support custom scopes.** Don't pretend it
  does. The honest answer in `docs/guide/idp/google.md` is "use
  Google → Keycloak/Auth0 → swsrs as a chain."
- **One TCP/UDP connection per session by design.** No multiplexing.
  If someone asks for it, they want a different tool (or they want to
  multiplex client-side, which is fine).

## Things we deliberately do NOT have

- No web admin dashboard (curl + the Go SDK + the CLI cover ops).
- No transparent reconnect within the peer-wait grace window. Drop
  surfaces as a read/write error; caller redials with the same token.
- No server-side WS pings (yet — see open ideas below).
- No idle-timeout (yet — see open ideas below). Only TTL and
  peer-wait.
- No per-user identity on the data plane. Slot tokens are opaque, not
  JWTs. Audit "who connected" via your control plane (where you mint
  tokens), not via swsrs.
- No autocert / Let's Encrypt embedded. Terminate TLS upstream or BYO
  cert via `--tls-cert`/`--tls-key`.
- No connection multiplexing per session.
- No first-class HTTP semantics. swsrs is for byte streams, not as a
  reverse proxy for HTTP.

## Open ideas (parking lot — not started)

- Server-side WS pings + configurable idle timeout. Both small (~50
  LOC each); pair them. The hook (`lastActivity`) already exists in
  `internal/session/session.go`.
- Install script + Homebrew tap + `.deb`/`.rpm` packages (GoReleaser
  `nfpm`) for easier on-box installs.
- Browser auth-code + PKCE helper in the TS SDK (optional, only if
  someone hits real demand).
- Identity-only mode (verify JWT, skip scope check) for Google-direct
  setups.

## Style for communication and code changes

The maintainer prefers:

- **Push back honestly.** If an idea has a flaw, name it. Don't
  agree-and-implement.
- **Recommend with tradeoffs.** Give 2–3 options with their costs.
- **Brief.** No advertorial language, no "amazing", no "powerful". State
  what something does; let the reader judge.
- **No emojis** in code, docs, or commit messages.
- **Tables and lists for structure** when comparing options.
- **Don't add features beyond what was asked.** A bug fix doesn't
  need surrounding cleanup; a one-shot doesn't need a helper. Three
  similar lines is better than a premature abstraction.
- **Verify before claiming done.** Run the build, the tests, and the
  smoke script.
- **For UI/site changes**, run the docs site (`pnpm --filter
  @emdzej/swsrs-docs dev`) and check visually before claiming it
  works. Mermaid syntax errors don't fail at parse time — they fail
  at render.

## References

| Doc | What it covers |
|---|---|
| `README.md`                            | Top-level overview, quickstart, full config table |
| `CHANGELOG.md`                         | Versioned change log; the source of truth for what shipped when |
| `docs/guide/architecture.md`           | Session lifecycle, surfaces, protocol-agnostic property |
| `docs/guide/auth.md`                   | Auth model, scope matrix, --no-auth, device flow |
| `docs/guide/comparison.md`             | vs wstunnel, chisel, frp, ngrok, Cloudflare Tunnel, WebRTC |
| `docs/guide/idp/`                      | Keycloak / Auth0 / Google setup |
| `docs/reference/configuration.md`      | Every env / flag |
| `docs/reference/admin-api.md`          | HTTP admin API |
| `docs/reference/discovery.md`          | `/.well-known/swsrs-config` shape |
| `examples/chat/`                       | End-to-end demo using the TS SDK |
| `scripts/smoke-chat.sh`                | The load-bearing CI smoke test |

## Hard-won "why" answers

These show up repeatedly; don't re-litigate without strong reason:

- **Why is npm under `@emdzej`?** Maintainer's scope.
- **Why pnpm?** Workspace ergonomics. Maintainer prefers it.
- **Why VitePress and not Docusaurus/MkDocs?** Lightweight, native Vue
  3, sidecar tooling we already use.
- **Why `coder/websocket`?** Successor to `nhooyr/websocket`, modern
  context-based API, small, well-maintained.
- **Why Go for the server?** Single static binary, fast cross-build
  via GoReleaser, low memory footprint, mature stdlib HTTP, OIDC
  library quality.
- **Why TypeScript for the browser SDK and not vanilla JS?** The
  surface is small (~6 source files) but types are valuable for
  embedders. Build to JS + .d.ts, ship both.
- **Why opaque per-slot tokens instead of JWTs?** No JWKS roundtrip
  on the WS upgrade hot path; no per-user identity needed for
  peers. See `docs/guide/auth.md`.
- **Why single port for admin + data plane?** Simplicity. If you need
  to split, run two `swsrs` instances behind a reverse proxy. No
  built-in option.
- **Why one TCP per session?** Multiplexing inside the relay would
  break the protocol-agnostic property. Multiplex client-side if you
  need it.
