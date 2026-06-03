import { describe, expect, it } from "vitest";
import { AuthDisabledError, discoverConfig } from "../src/auth.js";

function mockFetch(handler: (req: Request) => Response | Promise<Response>): typeof fetch {
  return async (input, init) => {
    const req = input instanceof Request ? input : new Request(input as string, init);
    return handler(req);
  };
}

describe("discoverConfig", () => {
  it("parses a happy-path response", async () => {
    const fetch = mockFetch((req) => {
      expect(req.url).toBe("https://relay.example.com/.well-known/swsrs-config");
      return new Response(
        JSON.stringify({
          issuer: "https://idp.example.com",
          audience: "swsrs",
          scopes: ["swsrs:session:create"],
          authorization_endpoint: "https://idp.example.com/auth",
          token_endpoint: "https://idp.example.com/token",
          device_authorization_endpoint: "https://idp.example.com/device",
          client_id_hint: "swsrs-cli",
        }),
        { status: 200, headers: { "content-type": "application/json" } },
      );
    });

    const cfg = await discoverConfig("https://relay.example.com", { fetch });
    expect(cfg.issuer).toBe("https://idp.example.com");
    expect(cfg.device_authorization_endpoint).toBe("https://idp.example.com/device");
    expect(cfg.client_id_hint).toBe("swsrs-cli");
  });

  it("strips a trailing slash from the relay URL", async () => {
    const fetch = mockFetch((req) => {
      expect(req.url).toBe("https://relay.example.com/.well-known/swsrs-config");
      return new Response(JSON.stringify({ issuer: "x", scopes: [], authorization_endpoint: "x", token_endpoint: "x" }), { status: 200 });
    });
    await discoverConfig("https://relay.example.com/", { fetch });
  });

  it("throws AuthDisabledError on 404", async () => {
    const fetch = mockFetch(() => new Response("nope", { status: 404 }));
    await expect(discoverConfig("https://relay.example.com", { fetch })).rejects.toBeInstanceOf(AuthDisabledError);
  });

  it("surfaces non-404 errors with status code", async () => {
    const fetch = mockFetch(() => new Response("boom", { status: 500, statusText: "Internal Server Error" }));
    await expect(discoverConfig("https://relay.example.com", { fetch })).rejects.toThrow(/500/);
  });

  it("requires a relayURL", async () => {
    await expect(discoverConfig("" as string)).rejects.toThrow(/relayURL is required/);
  });
});
