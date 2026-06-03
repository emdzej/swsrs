# Authentication & authorization

swsrs has **two independent auth domains**, by design.

```
┌──────────────────────────────────┐    ┌──────────────────────────────────┐
│ Admin plane  /admin/sessions     │    │ Data plane  /relay/{id}          │
│                                  │    │                                  │
│  OIDC JWT (Bearer)               │    │  Opaque per-slot token (Bearer   │
│   • JWKS auto-discovery from     │    │   or ?token= query)              │
│     SWSRS_OIDC_ISSUER            │    │                                  │
│   • Audience = SWSRS_OIDC_       │    │  Minted by the admin API at      │
│     AUDIENCE                     │    │  session creation; 128-bit       │
│   • Scope check per route        │    │  random; constant-time compared  │
│                                  │    │                                  │
│  Operators / control planes      │    │  Peers (machines, browsers)      │
│  authenticate with the IdP       │    │  authenticate with the token     │
└──────────────────────────────────┘    └──────────────────────────────────┘
```

**Why split?** Connecting peers may not have IdP identities — the
canonical use case is a support engineer (with an IdP account) tunneling
to a customer machine (no IdP account). The IdP gates *who can create
sessions*; the relay gates *who can use a specific session*. Keeps the
data-plane hot path dependency-free (no JWKS roundtrip per WS upgrade).

## Admin plane — OIDC JWT, scope-gated

Every admin request must carry `Authorization: Bearer <jwt>`. The JWT is
verified using JWKS auto-discovered from
`$SWSRS_OIDC_ISSUER/.well-known/openid-configuration`. The `aud` claim
must match `$SWSRS_OIDC_AUDIENCE`.

Scopes are pulled from either the standard `scope` claim
(space-delimited, RFC 8693) or `scp` (array, Azure-style).

| Method & Path | Required scope |
|---|---|
| `POST   /admin/sessions`       | `swsrs:session:create` |
| `GET    /admin/sessions`       | `swsrs:session:read`   |
| `GET    /admin/sessions/{id}`  | `swsrs:session:read`   |
| `DELETE /admin/sessions/{id}`  | `swsrs:session:delete` |

Tokens missing the required scope get `403 Forbidden`; missing or
invalid tokens get `401 Unauthorized`.

### Minimum scopes for typical roles

| Role | Scopes needed |
|---|---|
| **Peer connecting to a session** | **None** — opaque tokens, no IdP involved |
| Client app that mints sessions  | `swsrs:session:create` |
| Operator / ops dashboard         | `…create` + `…read` |
| Service that terminates early    | + `…delete` |

For most apps, granting just `swsrs:session:create` to your client is
enough. End users (the actual relay peers) never need OIDC scopes.

## Data plane — opaque per-slot tokens

The admin API's `POST /admin/sessions` returns:

```json
{
  "id": "OVMnxnEzZA9QvFc…",
  "initiator_token": "Gi-jvZfpHgahXc618jUP8A",
  "responder_token": "tKuylZcPfwjvy43…",
  "expires_at": "…"
}
```

These tokens are **not JWTs** and carry no scopes. They are 128-bit
random strings, each bound to a single slot of a single session.

To attach on the data plane:

```http
GET /relay/{id} HTTP/1.1
Authorization: Bearer <initiator_token-or-responder_token>
Upgrade: websocket
```

Browsers can't set the `Authorization` header on a WS upgrade, so the
data plane also accepts `?token=<token>` as a fallback. The TS SDK uses
this form. Always serve `wss://` so tokens don't leak in plaintext.

### What a token grants

- Attach to **its slot only** (initiator or responder). A token used
  against the wrong slot is rejected with `401`.
- For the lifetime of the session and within `--peer-wait` after a drop —
  the slot is reusable for reconnect, never *concurrently* held by more
  than one connection.
- No admin-plane access. Data-plane tokens cannot create, list, or
  delete sessions.

Once the session is closed or `expires_at` passes, the tokens are dead.

## Client login flow — discovery + device code

Clients shouldn't need to know the IdP URL or scopes. The server
publishes them:

```
GET /.well-known/swsrs-config         (public)
→ { issuer, audience, scopes,
    authorization_endpoint, token_endpoint, device_authorization_endpoint,
    client_id_hint }
```

`swsrs auth` consumes this and drives the OAuth 2.0 **device
authorization grant** (RFC 8628) — no browser redirects, no callback
URLs:

```bash
swsrs auth --relay https://relay.example.com

# discovering OIDC config from https://relay.example.com ...
#
#   1. open this URL on any device:
#        https://idp.example.com/device?user_code=WDJB-MJHT
#
#   2. enter code: WDJB-MJHT
#
# ✓ saved token to ~/.config/swsrs/credentials.json
```

Subsequent commands (`swsrs create`, etc.) pick up the cached token
automatically. Tokens are refreshed transparently when the IdP supplies
a refresh token; otherwise the command exits with a clear error pointing
the user back at `swsrs auth`.

**Server side**, register **one** OAuth client (public, device-flow
enabled) at your IdP and pass its id via `--oidc-client-id`. All swsrs
clients of this deployment share that single client_id — the rendezvous
party is the deployment, not the individual user.

**Browser apps** should NOT use device flow (the token endpoint isn't
CORS-enabled at most IdPs). Run your own auth-code + PKCE flow with an
IdP-supported library and pass the resulting access token to the
`AdminClient` directly.

## Local development — `--no-auth`

For local testing without an IdP:

```bash
swsrs serve --no-auth --addr :8080
# WARN AUTH DISABLED — admin API is open. Do not use in production.
```

Effect: admin routes skip OIDC verification and scope checks entirely.
Data-plane tokens still work the same way (they aren't OIDC-related).
Never expose a `--no-auth` server to a network you don't trust.
