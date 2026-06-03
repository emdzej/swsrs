# TLS & CORS

## TLS

Two modes, controlled by whether `--tls-cert` and `--tls-key` are set:

- **off** (default) — plain HTTP; assume an upstream gateway (ALB,
  Cloudflare, Caddy) terminates TLS
- **BYO** — pass both flags; the binary uses `ListenAndServeTLS`

In either case, the data plane and admin plane **must** be reached over
TLS in production — session and OIDC tokens both travel in
`Authorization`.

The current build doesn't include automatic ACME / Let's Encrypt
autocert. The decision was deliberate: a single instance behind
Cloudflare or an ALB gets free TLS at the gateway, and embedding
autocert adds ~1 MB to the binary plus a cert cache directory.

## CORS

The relay's HTTP surfaces (`/admin/*` and `/.well-known/*`) emit proper
CORS headers when the request `Origin` matches `SWSRS_ALLOWED_ORIGINS`.
**The same allowlist controls the WebSocket Origin check**, so you
configure browser origins in one place:

```bash
SWSRS_ALLOWED_ORIGINS="app.example.com,*.dev.example.com,localhost:*" \
  swsrs serve …
```

Matching semantics mirror `coder/websocket`'s `OriginPatterns` — glob
on the **host** portion of the Origin header (lowercased, including
port). Examples:

| Pattern | Matches |
|---|---|
| `app.example.com`          | exact host only |
| `*.example.com`            | any single-label subdomain (`app.example.com`, but **not** `example.com`) |
| `localhost:*`              | any port on localhost |

`OPTIONS` preflight requests short-circuit with:

- `Access-Control-Allow-Methods: GET, POST, PUT, PATCH, DELETE, OPTIONS`
- `Access-Control-Allow-Headers: <echoed from request, or default>`
- `Access-Control-Allow-Credentials: true`
- `Access-Control-Max-Age: 600`
- `Vary: Origin`

Disallowed origins fall through silently — no CORS headers are set, so
the browser blocks the response. `Vary: Origin` is always emitted to
prevent cache poisoning across origins.

Empty `SWSRS_ALLOWED_ORIGINS` disables CORS entirely (no headers; the
HTTP surfaces are same-origin only, and WS upgrades reject all
cross-origin requests).
