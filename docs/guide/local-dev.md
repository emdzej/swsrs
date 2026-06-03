# Local development (no IdP)

Pass `--no-auth` to skip OIDC entirely. Useful for hacking on swsrs or
demoing a flow without standing up Keycloak/Auth0/etc.

```bash
swsrs serve --no-auth --addr :8080
# WARN AUTH DISABLED — admin API is open. Do not use in production.
```

The server logs a loud warning. Don't put this on a network you don't
trust.

## What --no-auth changes

| Endpoint | With OIDC | With --no-auth |
|---|---|---|
| `POST /admin/sessions`           | `swsrs:session:create` required | open |
| `GET /admin/sessions`            | `swsrs:session:read` required   | open |
| `GET /admin/sessions/{id}`       | `swsrs:session:read` required   | open |
| `DELETE /admin/sessions/{id}`    | `swsrs:session:delete` required | open |
| `/relay/{id}` (data plane)       | opaque per-slot token           | unchanged — still token-gated |
| `/.well-known/swsrs-config`      | 200 with IdP config             | **404** — nothing to discover |
| `/healthz`                       | 200                             | 200 |

## End-to-end demo

The repo ships a runnable smoke test:

```bash
bash scripts/smoke-chat.sh
```

It builds the binary, starts the relay with `--no-auth`, runs both
sides of [`examples/chat`](/guide/examples/chat), and asserts each side
receives the other's message. Same script runs in CI.

You can also run the demo manually:

```bash
# terminal 1
go run ./cmd/swsrs serve --no-auth --addr :8080

# terminal 2 — create a session
curl -X POST http://localhost:8080/admin/sessions
# → { "id": "...", "initiator_token": "...", "responder_token": "..." }

# terminal 3 — alice (initiator)
swsrs raw --url ws://localhost:8080 --session $ID --token $INIT_TOK --role initiator

# terminal 4 — bob (responder)
swsrs raw --url ws://localhost:8080 --session $ID --token $RESP_TOK --role responder
```

Anything typed in either terminal appears in the other.

## Calling the TS SDK against a --no-auth server

`AdminClient.token` still needs to return *something*, even if the
server ignores it. Return an empty string:

```ts
const admin = new AdminClient({
  baseURL: "http://localhost:8080",
  token: () => "",
});
const session = await admin.createSession();
```

The `discoverConfig()` helper throws `AuthDisabledError` against a
`--no-auth` server — your code should treat that as "no token needed".
