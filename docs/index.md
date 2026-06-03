---
layout: home

hero:
  name: swsrs
  text: Simple WebSocket Relay Service
  tagline: Embed remote-diagnose and live-support tunnels directly into your Go or TypeScript app — your users never install a CLI, never touch a firewall, never run a VPN. One ~7 MB self-hosted relay in the cloud, an SDK inside the apps you already ship.
  actions:
    - theme: brand
      text: Get started in 3 min
      link: /guide/quickstart
    - theme: alt
      text: How it compares
      link: /guide/comparison
    - theme: alt
      text: View on GitHub
      link: https://github.com/emdzej/swsrs

features:
  - title: Your app IS the tunnel client
    details: Go SDK returns a net.Conn that drops straight into grpc.WithContextDialer, http.Transport, crypto/tls. TypeScript SDK works in browser and Node 22+ with zero runtime deps. No separate CLI to ship to your users, no "first install our tunnel utility" step in your support tickets.
  - title: Built for remote debugging
    details: A support engineer mints a session, your app on the customer's machine connects out, and you're talking to the user's instance like it was local. No port forwarding. No customer-side daemons. No VPN.
  - title: Two-plane auth
    details: Admin API gated by OIDC + scopes. Data plane gated by opaque per-slot tokens minted at session creation. Connecting peers never need an IdP identity — exactly fits the support-engineer-to-customer-machine pattern.
  - title: Protocol-agnostic relay
    details: Forwards opaque WebSocket binary frames. TCP, UDP, gRPC, SSH, raw bytes — all handled by the SDK or CLI adapters. The server stays auditable and unchanged when you add new protocols.
  - title: Tiny by design
    details: A static Go binary, about 7 MB on linux/arm64. Designed to run happily on a $3/month t4g.nano. Multi-stage distroless Docker image too.
  - title: Zero IdP coordinates on clients
    details: /.well-known/swsrs-config publishes the IdP endpoints so the CLI runs OAuth 2.0 device flow without anyone configuring issuer URLs, client IDs, or scopes manually.

---

<style>
:root {
  --vp-home-hero-name-color: transparent;
  --vp-home-hero-name-background: -webkit-linear-gradient(120deg, #2563eb 30%, #06b6d4);
}
</style>

<div style="max-width: 960px; margin: 4rem auto 0; padding: 0 1.5rem;">

## What it's for

You ship an app — a desktop client, a CLI tool, a backend service, an embedded device — and you need a way to **reach into a specific instance running on a user's machine** to debug it, collect diagnostics, or open an interactive session. The usual options are bad:

- **VPN / port-forwarding:** asks users to configure networking they shouldn't have to think about.
- **"Install our debug agent":** another binary to ship, sign, update, and explain.
- **SaaS tunnel like ngrok:** routes your customers' data through someone else's infrastructure.

swsrs sits in the gap. **Your app already has the relay client linked in.** When you need to reach an instance, you mint a session, the app on the user's machine opens an outbound WebSocket to your relay, and you connect from your end. No new software on the user's machine, no firewall changes, no VPN, no third-party data path.

```mermaid
flowchart TB
    relay["<b>swsrs</b> (your cloud)<br/>~7 MB static binary"]
    peerA["<b>your app</b><br/>on user's machine<br/><i>(SDK linked in)</i>"]
    peerB["<b>your support tool</b><br/>or browser UI<br/><i>(SDK linked in)</i>"]
    peerA -- outbound wss:// --> relay
    peerB -- outbound wss:// --> relay
    peerA -. tunneled traffic .- peerB

    classDef cloud fill:#2563eb,stroke:#1d4ed8,color:#fff
    classDef peer  fill:#f1f5f9,stroke:#94a3b8,color:#0f172a
    class relay cloud
    class peerA,peerB peer
```

## Remote debugging your own app

The killer use case. Concretely:

```mermaid
sequenceDiagram
    autonumber
    participant Eng as Support engineer
    participant Backend as Your backend
    participant Relay as swsrs relay
    participant App as Your app<br/>(on user's machine)

    Eng->>Backend: "I need to debug user X's instance"
    Backend->>Relay: POST /admin/sessions<br/>(OIDC bearer, scope=create)
    Relay-->>Backend: { id, initiator_token, responder_token }
    Backend->>App: push responder_token<br/>(your existing control plane)
    App->>Relay: WS /relay/{id} with responder_token
    Eng->>Relay: WS /relay/{id} with initiator_token
    Note over Eng,App: gRPC / HTTP / SSH / raw bytes —<br/>whatever the app speaks
```

Notice what's **not** on this diagram:

- A "swsrs CLI" running on the user's machine.
- A request to open ports on the user's firewall.
- A separate tunneling daemon to install and maintain.
- A third-party SaaS in the data path.

The user's machine has your app. Your app has the SDK. That's the whole story on the customer side.

## What makes it different

Most NAT-traversal tools either (a) skip auth or use a shared secret, (b) bundle a heavyweight gateway you can't fit on a `t4g.nano`, or (c) require a separate tunnel binary on every user's machine. swsrs sits in the intersection:

- **The party who can mint sessions is gated by your IdP** (OIDC, scope-claim).
- **The parties who actually use the tunnel are gated by short-lived per-slot tokens** — they need no IdP identity.
- **The server never inspects payloads** — it forwards opaque frames. Your app decides the protocol.
- **The peer logic is library code, not a separate process.** Link it into the app you're already shipping.

[See the full comparison →](/guide/comparison)

## Try it

```bash
# Run the relay locally with auth disabled (dev only)
go run github.com/emdzej/swsrs/cmd/swsrs@latest serve --no-auth --addr :8080

# In another terminal — end-to-end chat over the relay
bash scripts/smoke-chat.sh
# [smoke] PASS
```

For production: pick an IdP, point `--oidc-issuer` at it, and your clients run `swsrs auth` once. [Step-by-step setup for Keycloak / Auth0 →](/guide/idp/)

</div>
