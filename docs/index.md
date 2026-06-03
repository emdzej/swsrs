---
layout: home

hero:
  name: swsrs
  text: Simple WebSocket relay service
  tagline: Connect two parties behind NAT or firewalls through a single bidirectional WebSocket tunnel. Self-hostable. About 7 MB on ARM64.
  actions:
    - theme: brand
      text: Get started
      link: /guide/quickstart
    - theme: alt
      text: View on GitHub
      link: https://github.com/emdzej/swsrs

features:
  - title: Tiny footprint
    details: One static Go binary, around 7 MB on linux/arm64. Designed to run happily on a t4g.nano.
  - title: Two-plane auth
    details: Admin API gated by OIDC scopes; data plane gated by opaque per-slot tokens. The connecting peers never need an IdP identity.
  - title: Protocol-agnostic
    details: The relay forwards opaque WebSocket binary frames. TCP, UDP, gRPC, SSH — anything bytes-shaped works without server changes.
  - title: Embeddable
    details: First-class Go and TypeScript SDKs so your app becomes a relay peer without anyone installing a CLI.
  - title: Discovery-driven
    details: /.well-known/swsrs-config lets clients run device-flow auth without hard-coding the IdP. Built-in `swsrs auth` does it for you.
  - title: Ready to deploy
    details: Multi-arch Docker image, GoReleaser cross-builds, multi-arch CI, distroless runtime, npm trusted publishing.
---

<style>
:root {
  --vp-home-hero-name-color: transparent;
  --vp-home-hero-name-background: -webkit-linear-gradient(120deg, #2563eb 30%, #06b6d4);
}
</style>
