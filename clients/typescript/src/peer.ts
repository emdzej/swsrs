export interface PeerOptions {
  /** Relay base URL, e.g. "wss://relay.example.com". http(s):// is auto-upgraded to ws(s)://. */
  relayURL: string;
  sessionId: string;
  token: string;
  /** Aborting cancels the handshake and closes any open connection. */
  signal?: AbortSignal;
  /** Custom WebSocket constructor (defaults to global). Useful for tests. */
  WebSocketImpl?: typeof WebSocket;
  /** Subprotocols passed to the WebSocket constructor. */
  protocols?: string | string[];
}

export interface PeerConnection {
  /** Underlying WebSocket. Use this for `binaryType`, `bufferedAmount`, etc. */
  readonly socket: WebSocket;
  /** Resolves when the WS opens. Rejects if it fails to open. */
  readonly opened: Promise<void>;
  /** Resolves when the WS closes. Includes the close event. */
  readonly closed: Promise<CloseEvent>;
  /** Send a binary or text message. Equivalent to `socket.send`. */
  send(data: ArrayBufferLike | ArrayBufferView | Blob | string): void;
  /** Close the connection. */
  close(code?: number, reason?: string): void;
}

/** Connect to the relay as the initiator role. */
export async function dial(opts: PeerOptions): Promise<PeerConnection> {
  return connect(opts);
}

/** Connect to the relay as the responder role. Wire-identical to dial(). */
export async function accept(opts: PeerOptions): Promise<PeerConnection> {
  return connect(opts);
}

async function connect(opts: PeerOptions): Promise<PeerConnection> {
  if (!opts.relayURL) throw new Error("peer: relayURL is required");
  if (!opts.sessionId) throw new Error("peer: sessionId is required");
  if (!opts.token) throw new Error("peer: token is required");

  const url = buildRelayURL(opts.relayURL, opts.sessionId, opts.token);
  const Ctor = opts.WebSocketImpl ?? WebSocket;
  const ws = opts.protocols !== undefined ? new Ctor(url, opts.protocols) : new Ctor(url);
  ws.binaryType = "arraybuffer";

  const opened = new Promise<void>((resolve, reject) => {
    const onOpen = () => {
      cleanup();
      resolve();
    };
    const onError = () => {
      cleanup();
      reject(new Error("peer: websocket failed to open"));
    };
    const onClose = (e: CloseEvent) => {
      cleanup();
      reject(new Error(`peer: closed during handshake (${e.code} ${e.reason})`));
    };
    const cleanup = () => {
      ws.removeEventListener("open", onOpen);
      ws.removeEventListener("error", onError);
      ws.removeEventListener("close", onClose);
    };
    ws.addEventListener("open", onOpen);
    ws.addEventListener("error", onError);
    ws.addEventListener("close", onClose);
  });

  const closed = new Promise<CloseEvent>((resolve) => {
    ws.addEventListener("close", (e) => resolve(e), { once: true });
  });

  if (opts.signal) {
    if (opts.signal.aborted) {
      ws.close();
      throw new DOMException("aborted", "AbortError");
    }
    opts.signal.addEventListener("abort", () => ws.close(), { once: true });
  }

  await opened;
  return {
    socket: ws,
    opened: Promise.resolve(),
    closed,
    send: (data) => ws.send(data),
    close: (code, reason) => ws.close(code, reason),
  };
}

/**
 * Browsers cannot set the Authorization header on WebSocket upgrades, so the
 * SDK passes the session token as a query parameter. The relay server accepts
 * either form on `/relay/{id}`.
 */
function buildRelayURL(base: string, sessionId: string, token: string): string {
  const u = new URL(base);
  switch (u.protocol) {
    case "http:":
      u.protocol = "ws:";
      break;
    case "https:":
      u.protocol = "wss:";
      break;
    case "ws:":
    case "wss:":
      break;
    default:
      throw new Error(`peer: unsupported scheme ${u.protocol}`);
  }
  u.pathname = u.pathname.replace(/\/$/, "") + "/relay/" + encodeURIComponent(sessionId);
  u.searchParams.set("token", token);
  return u.toString();
}
