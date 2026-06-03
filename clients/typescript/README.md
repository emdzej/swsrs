# @emdzej/swsrs-client

TypeScript client for the [swsrs](https://github.com/emdzej/swsrs) relay.
Works in browsers and Node 22+. Zero runtime dependencies — uses native
`fetch` and `WebSocket`.

## Install

```bash
pnpm add @emdzej/swsrs-client    # or: npm install / yarn add
```

This package is developed and published with pnpm. Consumers can install it
with any package manager — the published artifact is the same.

## Admin API

```ts
import { AdminClient } from "@emdzej/swsrs-client";

const admin = new AdminClient({
  baseURL: "https://relay.example.com",
  token: async () => await getOIDCToken(),  // called per request, may refresh
});

const session = await admin.createSession();
// { id, initiator_token, responder_token, expires_at, ... }

const status = await admin.getSession(session.id);
const all    = await admin.listSessions();
await admin.deleteSession(session.id);
```

`token` may be a sync function, async function, or plain string-returning
function. It is invoked on every request so you can rotate without rebuilding
the client.

## Peer connection

`dial` and `accept` are wire-identical — the names express caller intent.
Both return a `PeerConnection` wrapping a native `WebSocket`.

```ts
import { dial } from "@emdzej/swsrs-client";

const conn = await dial({
  relayURL: "wss://relay.example.com",  // http(s):// auto-upgraded
  sessionId: session.id,
  token: session.initiator_token,
});

conn.socket.addEventListener("message", (e) => {
  // e.data is ArrayBuffer (socket.binaryType defaults to "arraybuffer")
  console.log("got", new Uint8Array(e.data as ArrayBuffer));
});

conn.send(new TextEncoder().encode("hello"));

await conn.closed;  // resolves on disconnect with the CloseEvent
```

### Why the token is in the URL

Browsers cannot set the `Authorization` header on a WebSocket upgrade, so the
SDK passes `?token=` instead. The swsrs server accepts either form. Tokens
are short-lived and bound to a specific slot — but you should still always
serve `wss://`, never `ws://`, to keep them out of plaintext.

### Cancellation

Pass an `AbortSignal` to abort the handshake or close an open connection:

```ts
const ctrl = new AbortController();
setTimeout(() => ctrl.abort(), 5_000);

await dial({ relayURL, sessionId, token, signal: ctrl.signal });
```

## End-to-end example: probe ↔ UI

```ts
// === UI side (browser) ===
const admin = new AdminClient({ baseURL: "https://relay.example.com", token: getOIDC });
const session = await admin.createSession();

// hand session.responder_token to the probe via your existing control plane
await myBackend.tellProbe(probeId, { sessionId: session.id, token: session.responder_token });

const conn = await dial({
  relayURL: "wss://relay.example.com",
  sessionId: session.id,
  token: session.initiator_token,
});

conn.socket.addEventListener("message", handleProbeData);
conn.send(encode({ cmd: "runCheck" }));
```

## API reference

### `class AdminClient`
- `new AdminClient({ baseURL, token, fetch? })`
- `createSession(signal?): Promise<Session>`
- `getSession(id, signal?): Promise<SessionStatus>`
- `listSessions(signal?): Promise<SessionStatus[]>`
- `deleteSession(id, signal?): Promise<void>`

### `function dial(opts: PeerOptions): Promise<PeerConnection>`
### `function accept(opts: PeerOptions): Promise<PeerConnection>`

```ts
interface PeerOptions {
  relayURL: string;
  sessionId: string;
  token: string;
  signal?: AbortSignal;
  WebSocketImpl?: typeof WebSocket;  // tests
  protocols?: string | string[];
}

interface PeerConnection {
  readonly socket: WebSocket;
  readonly opened: Promise<void>;
  readonly closed: Promise<CloseEvent>;
  send(data: ArrayBufferLike | ArrayBufferView | Blob | string): void;
  close(code?: number, reason?: string): void;
}
```

## Known limitations

- **No transparent reconnect** within the peer-wait grace window.
- **No application-layer pings** (browsers can't send WS protocol pings). For
  long-lived sessions, ensure the other peer or the server pings.

## License

MIT.
