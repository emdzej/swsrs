# Configuration

All options accept either env vars or flags. Flags override env.

| Env / Flag | Default | Description |
|---|---|---|
| `SWSRS_ADDR` / `--addr` | `:8080` | Listen address |
| `SWSRS_OIDC_ISSUER` / `--oidc-issuer` | *(required unless `--no-auth`)* | OIDC issuer URL (autodiscovery) |
| `SWSRS_OIDC_AUDIENCE` / `--oidc-audience` | — | Expected `aud` claim (your IdP client_id). Empty disables audience check — strongly discouraged in production. |
| `SWSRS_OIDC_CLIENT_ID` / `--oidc-client-id` | — | Shared OAuth `client_id` surfaced via `/.well-known/swsrs-config` (clients use this with device flow) |
| `SWSRS_SESSION_TTL` / `--session-ttl` | `1h` | Maximum session lifetime |
| `SWSRS_PEER_WAIT` / `--peer-wait` | `2m` | How long a peer waits for its counterpart |
| `SWSRS_REAP_INTERVAL` / `--reap-interval` | `30s` | Expired-session sweep cadence |
| `SWSRS_PUBLIC_BASE_URL` / `--public-base-url` | — | Public ws(s) URL embedded in admin responses |
| `SWSRS_ALLOWED_ORIGINS` | — | Comma-separated host patterns allowed as `Origin` for both WebSocket upgrades and HTTP/CORS on `/admin/*` and `/.well-known/*`. Glob hosts supported (`app.example.com`, `*.example.com`, `localhost:*`). Empty = same-origin only. |
| `SWSRS_TLS_CERT` / `--tls-cert` | — | PEM cert; with `--tls-key` enables in-process TLS |
| `SWSRS_TLS_KEY` / `--tls-key` | — | PEM key |
| `SWSRS_NO_AUTH` / `--no-auth` | `false` | **Dev only** — disable OIDC verification on the admin API |
| `SWSRS_MAX_FRAME_SIZE` / `--max-frame-size` | `-1` | Max WS frame size (bytes) accepted on the data plane. `-1` = unlimited. Default fits the protocol-agnostic relay model — peers are already authenticated. Set a positive cap (e.g. `67108864` = 64 MB) if you want defence-in-depth against a compromised peer. |

## OIDC scopes

The admin API enforces these scopes per route:

| Method & Path | Required scope |
|---|---|
| `POST   /admin/sessions`       | `swsrs:session:create` |
| `GET    /admin/sessions`       | `swsrs:session:read`   |
| `GET    /admin/sessions/{id}`  | `swsrs:session:read`   |
| `DELETE /admin/sessions/{id}`  | `swsrs:session:delete` |

A typical client app needs only `swsrs:session:create`. See
[Authentication](/guide/auth) for the model.

## Endpoints

| Path | Auth | Notes |
|---|---|---|
| `GET /.well-known/swsrs-config`     | none (public) | 404s when `--no-auth` |
| `GET /healthz`                       | none (public) | 200 ok |
| `POST /admin/sessions`               | OIDC + `swsrs:session:create` | 201 + session JSON |
| `GET /admin/sessions`                | OIDC + `swsrs:session:read`   | 200 + `{ sessions: [...] }` |
| `GET /admin/sessions/{id}`           | OIDC + `swsrs:session:read`   | 200 or 404 |
| `DELETE /admin/sessions/{id}`        | OIDC + `swsrs:session:delete` | 204 or 404 |
| `GET /relay/{id}` (WS upgrade)       | opaque per-slot token         | 101 or 401/404 |
