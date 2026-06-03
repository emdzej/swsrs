# Two-party chat

A minimal Node + TypeScript chat over a relay session, using
`@emdzej/swsrs-client` and `commander`. Demonstrates session creation,
out-of-band token handoff, and `dial` / `accept` on both sides.

[Source on GitHub →](https://github.com/emdzej/swsrs/tree/main/examples/chat)

## Run locally (no IdP)

In one terminal, start the relay without auth:

```bash
go run ./cmd/swsrs serve --no-auth --addr :8080
```

In a second terminal, install + build the workspace from the repo root:

```bash
pnpm install
pnpm -r run build
```

### Host side (alice)

```bash
node examples/chat/dist/index.js host \
  --relay ws://localhost:8080 \
  --name alice
```

It prints:

```
[swsrs-chat] session id:        OVMnxnEzZA9QvFc…
[swsrs-chat] responder token:   tKuylZcPfwjvy43…
[swsrs-chat] tell the other side to run:
  swsrs-chat join --relay ws://localhost:8080 --session OVM… --token tKu…
```

### Joining side (bob)

```bash
node examples/chat/dist/index.js join \
  --relay ws://localhost:8080 \
  --session OVMnxnEzZA9QvFc… \
  --token tKuylZcPfwjvy43… \
  --name bob
```

Type lines on either side. Press Ctrl-D to disconnect.

### One-liner CI-style verification

If you just want to confirm the whole stack works without driving it
manually:

```bash
bash scripts/smoke-chat.sh
# [smoke] PASS
```

## Run against a real relay (with OIDC)

Same flow, two differences:

1. Start the relay **without** `--no-auth` and configure your IdP
   (`SWSRS_OIDC_ISSUER`, `SWSRS_OIDC_AUDIENCE`).
2. Pass `--token <jwt>` to `host` (must include `swsrs:session:create`).

```bash
node examples/chat/dist/index.js host \
  --relay wss://relay.example.com \
  --admin https://relay.example.com \
  --token "$OIDC_JWT" \
  --name alice
```

The joining side never needs the OIDC token — only the responder token
that the host prints.

## What this example demonstrates

- `AdminClient.createSession()` — host mints a session and receives two
  short-lived tokens.
- **Out-of-band token handoff** — in this demo the host prints the
  responder token to the terminal and you copy-paste it. In a real app
  this would travel through your existing control plane (push
  notification, backend API, fleet manager). swsrs has no opinion about
  this step.
- `dial()` / `accept()` — both sides open a WebSocket to the relay with
  their slot's token. The relay is wire-symmetric; the name expresses
  caller intent only.
- **Protocol-agnostic relay** — chat lines are UTF-8 in WS binary
  frames. The relay server never inspects them; it could be carrying
  gRPC, SSH, or raw bytes with no code change.
