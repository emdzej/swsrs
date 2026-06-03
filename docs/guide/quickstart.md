# Quickstart

Get a relay running, mint a session, and exchange messages — about three
minutes.

## 1. Run the server

For the impatient — local dev without an IdP:

```bash
go run github.com/emdzej/swsrs/cmd/swsrs@latest serve --no-auth --addr :8080
```

You'll see the loud warning:

```
WARN AUTH DISABLED — admin API is open. Do not use in production.
INFO listening addr=:8080 tls=false
```

For production, point at your IdP:

```bash
swsrs serve \
  --oidc-issuer  https://your-idp.example.com/realms/foo \
  --oidc-audience swsrs \
  --oidc-client-id swsrs                # for clients' device flow
```

Or Docker:

```bash
docker run --rm -p 8080:8080 \
  -e SWSRS_OIDC_ISSUER=https://your-idp.example.com/realms/foo \
  -e SWSRS_OIDC_AUDIENCE=swsrs \
  -e SWSRS_OIDC_CLIENT_ID=swsrs \
  ghcr.io/emdzej/swsrs:latest
```

## 2. Log in (production only)

```bash
swsrs auth --relay https://relay.example.com
```

This drives the OAuth 2.0 device flow against the IdP configured on the
server side — no IdP coordinates required from you. The CLI prints a URL
and a one-time code, you sign in on any device, the token is cached at
`~/.config/swsrs/credentials.json` (or the OS-equivalent).

Skip this step entirely with `--no-auth`.

## 3. Create a session

```bash
swsrs create --admin-url https://relay.example.com
```

Output (JSON):

```json
{
  "id": "OVMnxnEzZA9QvFc…",
  "initiator_token": "Gi-jvZfpHgahXc618jUP8A",
  "responder_token": "tKuylZcPfwjvy43…",
  "created_at": "2026-06-03T12:00:00Z",
  "expires_at": "2026-06-03T13:00:00Z"
}
```

Two opaque tokens, one per slot. Give one to each side that needs to
attach.

## 4. Connect both sides

Pick any combo of the adapter subcommands; both peers need to use the
same session id with their own slot's token.

### Echo over stdin/stdout (`raw`)

Terminal A — host:

```bash
swsrs raw --url wss://relay.example.com \
  --session OVMnxnEzZA9QvFc… --token $INITIATOR_TOKEN --role initiator
```

Terminal B — the other side:

```bash
swsrs raw --url wss://relay.example.com \
  --session OVMnxnEzZA9QvFc… --token $RESPONDER_TOKEN --role responder
```

Anything you type on one side appears on the other.

### TCP tunnel

Have the peer machine **expose** local TCP (e.g. its SSH port):

```bash
swsrs tcp-dial \
  --url wss://relay.example.com \
  --session ... --token $RESPONDER_TOKEN --role responder \
  --target 127.0.0.1:22
```

On the connecting side, **bind** a local TCP listener:

```bash
swsrs tcp-listen \
  --url wss://relay.example.com \
  --session ... --token $INITIATOR_TOKEN --role initiator \
  --listen 127.0.0.1:2222 &
ssh -p 2222 user@127.0.0.1
```

## Next

- [Architecture](/guide/architecture) — why the design looks like this
- [Authentication](/guide/auth) — scopes, --no-auth, audience, device flow
- [Go SDK](/guide/go-sdk) / [TypeScript SDK](/guide/typescript-sdk) — embed in your app
- [Chat example](/guide/examples/chat) — runnable two-party text chat
