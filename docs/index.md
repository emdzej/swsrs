---
layout: home

hero:
  name: swsrs
  text: Simple WebSocket Relay Service
  tagline: Reach your app where it runs — on a customer's laptop, a workshop machine, a field device — without asking them to open ports, run a VPN, or install anything extra. One ~7 MB self-hosted relay in the cloud, an SDK inside the apps you already ship.
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
    details: Go SDK returns a net.Conn that drops straight into grpc.WithContextDialer, http.Transport, crypto/tls. TypeScript SDK works in browser and Node 22+ with zero runtime deps. No separate CLI to ship to your users, no "first install our tunnel utility" step in your instructions.
  - title: Built for live customer sessions
    details: Tune, configure, install, support, diagnose — any live workflow where your operator-side software needs to reach an instance of your customer-side software, on a network you don't control. Customer runs the app, opens a session, hands you a token; you connect.
  - title: Two-plane auth
    details: Admin API gated by OIDC + scopes. Data plane gated by opaque per-slot tokens minted at session creation. The party that creates sessions needs an IdP identity; the connecting peers don't. Fits the operator-to-customer-machine pattern exactly.
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

You sell software that needs to **reach instances of itself running on customers' machines**. You don't control their network. They can't (or shouldn't have to) expose ports. The usual answers all push the cost onto the customer:

- **VPN / port-forwarding:** asks customers to configure networking they shouldn't have to think about.
- **"Install our agent":** another binary to ship, sign, update, and explain.
- **SaaS tunnel like ngrok:** routes your customers' data through someone else's infrastructure.

swsrs sits in the gap. **Your customer-side app already has the relay client linked in** — it's the same SDK you'd link in for any other library. When you need to reach an instance, the customer's app opens a session and hands you a token; your operator-side software connects. Nothing new on the customer's machine, no firewall changes, no VPN, no third-party data path.

```mermaid
flowchart TB
    relay["<b>swsrs</b> (your cloud)<br/>~7 MB static binary"]
    peerA["<b>your customer-side app</b><br/>on the customer's machine<br/><i>(SDK linked in)</i>"]
    peerB["<b>your operator UI / tool</b><br/>browser, desktop, CLI<br/><i>(SDK linked in)</i>"]
    peerA -- outbound wss:// --> relay
    peerB -- outbound wss:// --> relay
    peerA -. tunneled traffic .- peerB

    classDef cloud fill:#2563eb,stroke:#1d4ed8,color:#fff
    classDef peer  fill:#f1f5f9,stroke:#94a3b8,color:#0f172a
    class relay cloud
    class peerA,peerB peer
```

## A concrete example

You sell BMW diagnostic and coding software. Picture the flow:

> An owner wants help reading fault codes or applying coding changes. They run the **diagnostic app** you provided — a small Go binary that talks to the ECU over OBD-II. The app calls into the swsrs SDK and creates a session. Your backend pushes the responder token to a more experienced specialist via your existing UX. The specialist opens your **operator UI** in the browser. It uses the swsrs SDK to connect as the initiator. The UI is now talking live to the ECU on the owner's actual car. The specialist reads adaptation values, writes coding changes, validates against real-time telemetry. Done, hang up, session closes.

This isn't hypothetical — it's the real shape of
[Bimmerz Connect](/guide/case-studies/bimmerz), the production swsrs
deployment that powers the [bimmerz.app](https://bimmerz.app) suite of
BMW apps.

What this is NOT:

- It is **not** "the customer installs our tunneling utility." They installed your tuning client. The relay is just a library inside it.
- It is **not** "we VPN into their network." There's no network access at all — only one specific WebSocket session, gated by a one-time token.
- It is **not** "our data goes through a SaaS." The relay is yours; the data path is yours.

Swap "ECU tuning" for any of:

- **Hardware tuning / configuration** — printers, drones, audio interfaces, embedded controllers.
- **Software activation / installation walkthroughs** — interactive setup of complex on-prem deployments.
- **Live diagnostics / support** — pulling logs, profiling, attaching a debugger to a deployed service.
- **Remote pair-operation** — two operators acting on the same instance.
- **Field-engineer-to-deployed-device** — IoT or industrial gear in a customer site.

Same shape every time: a piece of your software on the customer side, a piece of your software on the operator side, a private rendezvous between them.

## How it actually wires up

```mermaid
sequenceDiagram
    autonumber
    participant Op as Operator UI<br/>(your software)
    participant Backend as Your backend
    participant Relay as swsrs relay
    participant App as Customer-side app<br/>(your software)

    Op->>Backend: "I want to operate on instance X"
    Backend->>Relay: POST /admin/sessions<br/>(OIDC bearer, scope=create)
    Relay-->>Backend: { id, initiator_token, responder_token }
    Backend->>App: push responder_token<br/>(your existing control plane / out-of-band)
    App->>Relay: WS /relay/{id} with responder_token
    Op->>Relay: WS /relay/{id} with initiator_token
    Note over Op,App: gRPC / HTTP / OBD-II frames / SSH / raw bytes —<br/>whatever the apps speak between each other
```

Notice what's **not** on this diagram:

- A "swsrs CLI" running on the customer's machine.
- A request to open ports on the customer's firewall.
- A separate tunneling daemon to install and maintain.
- A third-party SaaS in the data path.

The customer's machine has your app. Your app has the SDK. That's the whole story on the customer side.

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

## In production

<div style="border:1px solid var(--vp-c-divider); border-radius:12px; padding:1.25rem 1.5rem; margin:1.5rem 0;">

**Bimmerz Connect** — the relay behind [bimmerz.app](https://bimmerz.app)'s
BMW diagnostic and coding suite. Owners run a diagnostic app at home
plugged into their car; experienced users connect remotely from a web
UI and work on the live ECU. Single-instance swsrs against Keycloak,
running on a small EC2 instance behind Cloudflare.

[Read the case study →](/guide/case-studies/bimmerz)

</div>

*Using swsrs in production? [Open an issue](https://github.com/emdzej/swsrs/issues) — happy to feature you here.*

</div>
