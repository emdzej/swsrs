# swsrs-chat

A minimal two-party text chat over a swsrs relay session. The smallest
useful example of how to consume [`@emdzej/swsrs-client`](../../clients/typescript/README.md)
from a Node app.

## Prerequisites

- Go 1.26+ (only to run the relay locally — not needed against a remote one)
- Node 22+
- [pnpm](https://pnpm.io) 10+

## Quickstart (no IdP needed)

This example is a member of the repo's pnpm workspace, so installs happen
from the **repo root**, not from `examples/chat/`.

### 1. Build everything

```bash
# from repo root
pnpm install
pnpm -r run build
```

### 2. Start the relay with auth disabled

```bash
# from repo root, in its own terminal
go run ./cmd/swsrs serve --no-auth --addr :8080
```

The relay will log:
```
level=WARN msg="AUTH DISABLED — admin API is open. Do not use in production."
level=INFO msg=listening addr=:8080 tls=false
```

### 3. Start the host side (alice)

```bash
# from repo root, in a new terminal
node examples/chat/dist/index.js host \
  --relay ws://localhost:8080 \
  --name alice
```

It will print:

```
[swsrs-chat] session id:        OVMnxnEzZA9QvFc...
[swsrs-chat] responder token:   tKuylZcPfwjvy43...
[swsrs-chat] tell the other side to run:
  swsrs-chat join --relay ws://localhost:8080 --session OVM... --token tKu...
[swsrs-chat] waiting for peer to attach...
```

Copy the `join` command line.

### 4. Start the joining side (bob)

```bash
# in a third terminal, using the values printed by alice
node examples/chat/dist/index.js join \
  --relay ws://localhost:8080 \
  --session OVMnxnEzZA9QvFc... \
  --token tKuylZcPfwjvy43... \
  --name bob
```

Both terminals will print `[swsrs-chat] connected.`. Now type lines on
either side — every line you type is prefixed with your `--name` and shown
on the other side. Press **Ctrl-D** to disconnect; the other side closes
automatically.

### One-liner CI-style verification

If you just want to confirm the whole stack works without driving it
manually, the repo includes a self-contained smoke test that does the same
thing and asserts message delivery:

```bash
bash scripts/smoke-chat.sh
# [smoke] PASS
```

## Run against a real relay (with OIDC)

Same flow, with two differences:

1. Start the relay **without** `--no-auth` and configure your IdP
   (`SWSRS_OIDC_ISSUER`, `SWSRS_OIDC_AUDIENCE`).
2. Pass `--token <jwt>` to `host`. The JWT must include the
   `swsrs:session:create` scope.

```bash
node examples/chat/dist/index.js host \
  --relay wss://relay.example.com \
  --admin https://relay.example.com \
  --token "$OIDC_JWT" \
  --name alice
```

The joining side never needs the OIDC token — only the responder token that
the host prints.

## CLI reference

```
swsrs-chat host  --relay <url> [--admin <url>] [--token <oidc>] [--name <name>]
swsrs-chat join  --relay <url> --session <id> --token <responder-token> [--name <name>]
```

| Flag | Side | Description |
|---|---|---|
| `--relay`   | both | Relay base URL (`ws://` / `wss://`). Required. |
| `--admin`   | host | Admin base URL. Defaults to `--relay` with the scheme swapped to `http(s)`. |
| `--token`   | host | OIDC bearer for the admin API. Omit when the relay runs `--no-auth`. |
| `--session` | join | Session id printed by the host. |
| `--token`   | join | Responder token printed by the host. |
| `--name`    | both | Display name prefixed onto outgoing messages. Default: `host` or `guest`. |

## What this example demonstrates

- **`AdminClient.createSession()`** — the host mints a session and receives
  two short-lived tokens (`initiator_token`, `responder_token`).
- **Out-of-band token handoff** — in this demo the host prints the
  responder token to the terminal and you copy-paste it. In a real app this
  would travel through your existing control plane (push notification,
  backend API, fleet manager, etc.). swsrs has no opinion about this step.
- **`dial()` / `accept()`** — both sides open a WebSocket to the relay
  with their slot's token. The relay is wire-symmetric; the name expresses
  caller intent only.
- **Protocol-agnostic relay** — chat lines are UTF-8 in WS binary frames.
  The relay server never inspects them; it could be carrying gRPC, SSH, or
  raw bytes with no code change.

## File map

```
examples/chat/
  package.json          @emdzej/swsrs-example-chat (private, workspace member)
  tsconfig.json
  src/index.ts          commander-based CLI, ~120 lines
  dist/                 build output (gitignored)
  README.md             this file
```
