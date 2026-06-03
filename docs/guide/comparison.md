# How swsrs compares

NAT-traversal is a crowded space. Honest mapping of where swsrs fits and
when you should use something else.

## TL;DR — when to pick swsrs

Reach for swsrs if **all** of these are true:

- You need a **rendezvous** between two parties (initiator + responder),
  not a "publish my service on a public URL" gateway.
- You want to **self-host** on a tiny machine.
- The party minting sessions **has IdP credentials**, but the party
  connecting to the tunnel **does not** (and shouldn't need to).
- You want to **embed the peer logic in your app** rather than ship a
  separate CLI to your users.

Reach for something else if you want:

- A public URL exposing your local dev server: **ngrok / Cloudflare
  Tunnel**.
- Reverse-proxy a long-running internal service to the internet:
  **frp / chisel / rathole / boringproxy**.
- Direct peer-to-peer (audio, video, file-transfer at scale):
  **WebRTC + a TURN server**.

## Closest comparables — self-hostable WS/HTTP tunnels

| Tool | Lang | Auth | Session model | Embeddable SDK | Footprint | License |
|---|---|---|---|---|---|---|
| **swsrs**            | Go     | OIDC + scopes on admin, opaque per-slot tokens on data | **Ephemeral two-slot rendezvous** | **Go + TypeScript** | ~7 MB binary | MIT |
| [wstunnel](https://github.com/erebe/wstunnel)        | Rust   | Optional shared secret  | Point-to-point port forward | — | small | BSD-3 |
| [chisel](https://github.com/jpillora/chisel)         | Go     | SSH-style user/pass     | Long-running reverse-proxy  | — | small | MIT |
| [frp](https://github.com/fatedier/frp)               | Go     | Token; dashboard auth   | Pre-registered services     | — | medium | Apache-2.0 |
| [rathole](https://github.com/rapiz1/rathole)         | Rust   | Token                   | Pre-registered services     | — | small | Apache-2.0 |
| [bore](https://github.com/ekzhang/bore)              | Rust   | Optional shared secret  | Point-to-point port forward | — | tiny | MIT |
| [piping-server](https://github.com/nwtgck/piping-server) | TS/Rust | None (path is the credential) | Path-based pipe       | — | small | MIT |
| [boringproxy](https://github.com/boringproxy/boringproxy) | Go | OIDC-friendly        | Reverse-proxy gateway        | — | small | MIT |

### vs wstunnel

Our explicit inspiration. wstunnel is excellent if you want one
side-to-side TCP/UDP forward and you're OK managing a shared secret.

What swsrs adds: orchestrated session lifecycle, an admin API gated by
real OIDC (scopes), and per-tunnel tokens you can hand to a peer
without granting them anything else. Plus the SDKs — your app speaks the
relay protocol directly instead of spawning `wstunnel` as a child
process.

What you give up: wstunnel is more battle-tested and has a richer
port-forwarding feature set.

### vs chisel

Closest in spirit on the "small Go binary" axis. chisel uses an
SSH-style auth model (users in a file, fingerprint pinning). Strong
choice if you want long-running reverse-proxy tunnels and a single
operator.

What swsrs adds: OIDC + scope-based auth (no users-in-a-file file to
manage), session-based ephemeral tunnels, embeddable SDKs.

What you give up: chisel has connection multiplexing built in. swsrs
is one TCP/UDP per session by design.

### vs frp / rathole

Different shape. frp clients register **predefined services** with the
server at startup; the server then exposes them. Great for "I have a
service in my homelab, expose it permanently."

swsrs is **rendezvous-shaped**: sessions are ephemeral, the two sides
are typically dynamic ("this support engineer wants to talk to that
customer's probe right now").

### vs piping-server

Philosophically closest. piping-server is a brilliant 1-page idea: two
HTTP clients meet at a URL and pipe bytes through it.

What swsrs adds: real auth (piping-server treats path as credential),
session lifecycle (TTL, peer-wait, reaper), WebSocket framing (not
chunked HTTP), and SDKs.

What you give up: piping-server is gorgeously simple.

## SaaS alternatives

Not self-hosted, but worth honest comparison:

| Service | When it wins | When swsrs wins |
|---|---|---|
| **ngrok**             | "Expose my dev server in 30 seconds." Best one-line UX. | You control the data path. No vendor lock-in. Cost at scale. |
| **Cloudflare Tunnel** | "Run my service behind Cloudflare with WAF, DDoS, edge cache." Free tier. | You don't want everything routed through Cloudflare; data-residency requirements; SaaS-allergic. |
| **Tailscale Funnel**  | You're already on Tailscale and want to expose one node publicly. | You don't want to put every endpoint on a mesh VPN. |
| **inlets Pro**        | Lightweight commercial tunnel for k8s ingress. | Self-hosting on a single $3/mo VM. |

If your scenario is "expose an HTTP service publicly with TLS-at-edge
and request inspection," **Cloudflare Tunnel is hard to beat** — that's
not swsrs's sweet spot.

## Different model entirely — direct P2P

| Approach | When it wins | When swsrs wins |
|---|---|---|
| **WebRTC + TURN**  | Audio/video, large-scale media, latency-critical. Direct connection when possible. | Lower complexity. No SDP/ICE/DTLS stack to operate. Always-relay is fine for your use case. |
| **libp2p circuit relay** | You're already on libp2p (IPFS, etc.). | Way simpler. swsrs is one binary; libp2p is a framework. |

If you find yourself wanting WebRTC, you probably need WebRTC. swsrs is
the right choice when the value of a direct peer-to-peer connection
isn't worth the NAT-hole-punching complexity.

## What swsrs is **not** trying to be

Worth being explicit. swsrs is **not**:

- A web proxy or HTTP gateway (use Caddy, Traefik, Cloudflare).
- A VPN (use WireGuard, Tailscale).
- A connection multiplexer (one session, one stream — by design).
- A media-routing SFU (use mediasoup / LiveKit).
- A pub/sub or message bus (use NATS / Redis Streams).

It's a **two-party rendezvous + opaque byte relay**, with proper auth
and tiny operational cost. If that's what you need, swsrs is in the
sweet spot.
