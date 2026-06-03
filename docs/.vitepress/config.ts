import { defineConfig } from "vitepress";

export default defineConfig({
  title: "swsrs",
  description:
    "Simple WebSocket relay service: tunnel two parties behind NAT through a tiny self-hostable relay.",
  cleanUrls: true,
  lastUpdated: true,
  sitemap: { hostname: "https://swsrs.emdzej.pl" },

  head: [
    ["link", { rel: "icon", href: "/favicon.svg", type: "image/svg+xml" }],
    ["meta", { name: "theme-color", content: "#2563eb" }],
    ["meta", { property: "og:title", content: "swsrs — simple websocket relay service" }],
    ["meta", { property: "og:description", content: "Connect two parties behind NAT or firewalls through a single bidirectional WebSocket tunnel. Self-hostable. ~7 MB ARM64 binary." }],
    ["meta", { property: "og:url", content: "https://swsrs.emdzej.pl/" }],
    ["meta", { name: "twitter:card", content: "summary" }],
  ],

  themeConfig: {
    siteTitle: "swsrs",

    nav: [
      { text: "Guide", link: "/guide/quickstart", activeMatch: "/guide/" },
      { text: "Reference", link: "/reference/configuration", activeMatch: "/reference/" },
      {
        text: "0.2.1",
        items: [
          { text: "Changelog", link: "https://github.com/emdzej/swsrs/blob/main/CHANGELOG.md" },
          { text: "Releases", link: "https://github.com/emdzej/swsrs/releases" },
        ],
      },
    ],

    sidebar: {
      "/guide/": [
        {
          text: "Getting started",
          items: [
            { text: "Overview", link: "/guide/" },
            { text: "Quickstart", link: "/guide/quickstart" },
            { text: "Local dev (no IdP)", link: "/guide/local-dev" },
          ],
        },
        {
          text: "Architecture",
          items: [
            { text: "How it works", link: "/guide/architecture" },
            { text: "Authentication", link: "/guide/auth" },
            { text: "TLS & CORS", link: "/guide/tls-cors" },
          ],
        },
        {
          text: "Clients",
          items: [
            { text: "CLI", link: "/guide/cli" },
            { text: "Go SDK", link: "/guide/go-sdk" },
            { text: "TypeScript SDK", link: "/guide/typescript-sdk" },
          ],
        },
        {
          text: "Examples",
          items: [
            { text: "Two-party chat", link: "/guide/examples/chat" },
            { text: "Tunnel SSH", link: "/guide/examples/ssh" },
          ],
        },
      ],
      "/reference/": [
        {
          text: "Reference",
          items: [
            { text: "Configuration", link: "/reference/configuration" },
            { text: "Admin API", link: "/reference/admin-api" },
            { text: "Discovery", link: "/reference/discovery" },
          ],
        },
      ],
    },

    socialLinks: [
      { icon: "github", link: "https://github.com/emdzej/swsrs" },
      { icon: "npm", link: "https://www.npmjs.com/package/@emdzej/swsrs-client" },
    ],

    editLink: {
      pattern: "https://github.com/emdzej/swsrs/edit/main/docs/:path",
      text: "Edit this page on GitHub",
    },

    footer: {
      message: "Released under the MIT License.",
      copyright: "© 2026 Michał Jaskólski",
    },

    search: {
      provider: "local",
    },

    outline: { level: [2, 3] },
  },
});
