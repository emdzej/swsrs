---
layout: home

hero:
  name: swsrs
  text: Tunnel two peers behind NAT through a tiny self-hosted relay.
  tagline: One ~7 MB binary. OIDC-gated admin, opaque per-slot tokens on the wire. First-class Go and TypeScript SDKs so your app becomes the peer — no extra daemons to install.
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
  - title: Tiny by design
    details: A static Go binary, about 7 MB on linux/arm64. Designed to run happily on a $3/month t4g.nano. Multi-stage distroless Docker image too.
  - title: Two-plane auth
    details: Admin API gated by OIDC + scopes. Data plane gated by opaque per-slot tokens minted at session creation. Connecting peers never need an IdP identity — the canonical pattern for support-engineer-to-customer-machine tunnels.
  - title: Protocol-agnostic relay
    details: Forwards opaque WebSocket binary frames. TCP, UDP, gRPC, SSH, raw bytes — all handled client-side via SDK or CLI adapters. The server stays auditable and unchanged when you add new protocols.
  - title: Embed it directly
    details: Go SDK returns a net.Conn that drops into grpc.WithContextDialer, http.Transport, crypto/tls. TypeScript SDK works in browser and Node 22+, zero runtime deps. Your app IS the peer — no CLI on the user's machine.
  - title: Zero IdP coordinates on clients
    details: /.well-known/swsrs-config publishes the IdP endpoints so swsrs auth runs OAuth 2.0 device flow without anyone configuring issuer URLs, client IDs, or scopes manually.
  - title: Ready to deploy
    details: Multi-arch (amd64 + arm64) Docker image at ghcr.io. GoReleaser cross-builds for Linux, macOS, Windows. npm Trusted Publishing (OIDC, no NPM_TOKEN). Single git tag releases everything.

---

<style>
:root {
  --vp-home-hero-name-color: transparent;
  --vp-home-hero-name-background: -webkit-linear-gradient(120deg, #2563eb 30%, #06b6d4);
}
</style>

<div style="max-width: 960px; margin: 4rem auto 0; padding: 0 1.5rem;">

## What it's for

You have a service behind NAT — a diagnostic probe, a dev server, a customer's machine — and you need to reach it from somewhere else without poking holes in firewalls or running a VPN. **swsrs is one rendezvous endpoint that both sides open an outbound WebSocket to.** No port forwarding, no inbound rules, no client-side daemons in your customers' networks.

```
              ┌──────────────────────┐
              │   swsrs (cloud)      │
              │   ~7 MB static bin   │
              └──────────▲───────────┘
                         │ wss://
        ┌────────────────┴────────────────┐
        │                                 │
   outbound WS                       outbound WS
        │                                 │
  ┌─────┴──────┐                  ┌───────┴──────┐
  │ probe / sshd│  ────────────▶  │ UI / browser │
  │  behind NAT │     bytes flow  │ behind NAT   │
  └─────────────┘                 └──────────────┘
```

## What makes it different

Most NAT-traversal tools either (a) skip auth or use a shared secret, or (b) bundle a heavyweight gateway you can't fit on a `t4g.nano`. swsrs sits in the gap:

- **The party who can mint sessions is gated by your IdP** (OIDC, scope-claim).
- **The parties who actually use the tunnel are gated by short-lived per-slot tokens** — they need no IdP identity.
- **The server never inspects payloads** — it forwards opaque frames. Your app decides the protocol.

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
