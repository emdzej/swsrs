import { describe, expect, it, vi } from "vitest";
import type { RelayConfig } from "../src/auth.js";
import { deviceLogin } from "../src/device.js";

const baseConfig: RelayConfig = {
  issuer: "https://idp.example.com",
  audience: "swsrs",
  scopes: ["swsrs:session:create"],
  authorization_endpoint: "https://idp.example.com/auth",
  token_endpoint: "https://idp.example.com/token",
  device_authorization_endpoint: "https://idp.example.com/device",
  client_id_hint: "swsrs-cli",
};

// Builds a stub fetch that walks through a fixed sequence of responses
// keyed by URL substring. Each call records the request body so tests can
// assert on it.
function scriptedFetch(plan: { match: string; respond: (req: Request, body: URLSearchParams) => Response | Promise<Response> }[]): {
  fetch: typeof fetch;
  calls: { url: string; body: URLSearchParams }[];
} {
  const calls: { url: string; body: URLSearchParams }[] = [];
  const counters = new Map<string, number>();
  const fetch: typeof fetch = async (input, init) => {
    const url = typeof input === "string" ? input : input instanceof URL ? input.toString() : input.url;
    const rawBody = (init?.body as string) ?? "";
    const body = new URLSearchParams(rawBody);
    calls.push({ url, body });
    for (const step of plan) {
      if (url.includes(step.match)) {
        const n = (counters.get(step.match) ?? 0) + 1;
        counters.set(step.match, n);
        return step.respond(new Request(url, init), body);
      }
    }
    throw new Error(`unexpected fetch ${url}`);
  };
  return { fetch, calls };
}

describe("deviceLogin", () => {
  it("posts to device endpoint, prompts, polls until token", async () => {
    let pollCount = 0;
    const { fetch, calls } = scriptedFetch([
      {
        match: "/device",
        respond: () =>
          new Response(
            JSON.stringify({
              device_code: "DEV-CODE",
              user_code: "WDJB-MJHT",
              verification_uri: "https://idp.example.com/device",
              verification_uri_complete: "https://idp.example.com/device?code=WDJB-MJHT",
              expires_in: 1800,
              interval: 0, // poll immediately for the test
            }),
            { status: 200 },
          ),
      },
      {
        match: "/token",
        respond: () => {
          pollCount++;
          if (pollCount === 1) {
            return new Response(JSON.stringify({ error: "authorization_pending" }), { status: 400 });
          }
          return new Response(
            JSON.stringify({
              access_token: "atk-123",
              token_type: "Bearer",
              expires_in: 3600,
              refresh_token: "rtk-123",
            }),
            { status: 200 },
          );
        },
      },
    ]);

    const onPrompt = vi.fn();
    const tok = await deviceLogin({ config: baseConfig, onPrompt, fetch, omitOpenIDScopes: true });

    expect(onPrompt).toHaveBeenCalledOnce();
    const prompt = onPrompt.mock.calls[0]![0];
    expect(prompt.userCode).toBe("WDJB-MJHT");
    expect(prompt.verificationUriComplete).toContain("WDJB-MJHT");

    expect(tok.access_token).toBe("atk-123");
    expect(tok.refresh_token).toBe("rtk-123");
    expect(tok.expires_at).toBeTypeOf("number");

    // verify the device request body
    const deviceReq = calls.find((c) => c.url.includes("/device"))!;
    expect(deviceReq.body.get("client_id")).toBe("swsrs-cli");
    expect(deviceReq.body.get("scope")).toBe("swsrs:session:create");
  });

  it("auto-adds openid + offline_access scopes by default", async () => {
    const { fetch, calls } = scriptedFetch([
      {
        match: "/device",
        respond: () =>
          new Response(
            JSON.stringify({ device_code: "x", user_code: "x", verification_uri: "x", expires_in: 10, interval: 0 }),
            { status: 200 },
          ),
      },
      {
        match: "/token",
        respond: () => new Response(JSON.stringify({ access_token: "x", token_type: "Bearer" }), { status: 200 }),
      },
    ]);
    await deviceLogin({ config: baseConfig, onPrompt: () => {}, fetch });
    const scope = calls.find((c) => c.url.includes("/device"))!.body.get("scope")!;
    expect(scope).toContain("openid");
    expect(scope).toContain("offline_access");
    expect(scope).toContain("swsrs:session:create");
  });

  // RFC 8628 mandates a +5s bump on slow_down, so this test sleeps ~5s by design.
  it("handles slow_down by increasing the poll interval", { timeout: 10_000 }, async () => {
    let phase = 0;
    const { fetch } = scriptedFetch([
      {
        match: "/device",
        respond: () =>
          new Response(
            JSON.stringify({ device_code: "x", user_code: "x", verification_uri: "x", expires_in: 60, interval: 0 }),
            { status: 200 },
          ),
      },
      {
        match: "/token",
        respond: () => {
          phase++;
          if (phase === 1) return new Response(JSON.stringify({ error: "slow_down" }), { status: 400 });
          return new Response(JSON.stringify({ access_token: "ok", token_type: "Bearer" }), { status: 200 });
        },
      },
    ]);
    const tok = await deviceLogin({ config: baseConfig, onPrompt: () => {}, fetch, omitOpenIDScopes: true });
    expect(tok.access_token).toBe("ok");
    expect(phase).toBe(2);
  });

  it("rejects on access_denied", async () => {
    const { fetch } = scriptedFetch([
      {
        match: "/device",
        respond: () =>
          new Response(
            JSON.stringify({ device_code: "x", user_code: "x", verification_uri: "x", expires_in: 60, interval: 0 }),
            { status: 200 },
          ),
      },
      {
        match: "/token",
        respond: () => new Response(JSON.stringify({ error: "access_denied" }), { status: 400 }),
      },
    ]);
    await expect(deviceLogin({ config: baseConfig, onPrompt: () => {}, fetch, omitOpenIDScopes: true })).rejects.toThrow(
      /user denied/,
    );
  });

  it("rejects on expired_token", async () => {
    const { fetch } = scriptedFetch([
      {
        match: "/device",
        respond: () =>
          new Response(
            JSON.stringify({ device_code: "x", user_code: "x", verification_uri: "x", expires_in: 60, interval: 0 }),
            { status: 200 },
          ),
      },
      {
        match: "/token",
        respond: () => new Response(JSON.stringify({ error: "expired_token" }), { status: 400 }),
      },
    ]);
    await expect(deviceLogin({ config: baseConfig, onPrompt: () => {}, fetch, omitOpenIDScopes: true })).rejects.toThrow(
      /expired/,
    );
  });

  it("fails fast when discovery lacks the device endpoint", async () => {
    const cfg: RelayConfig = { ...baseConfig, device_authorization_endpoint: undefined };
    await expect(deviceLogin({ config: cfg, onPrompt: () => {} })).rejects.toThrow(/does not advertise/);
  });

  it("fails fast when no client_id is available", async () => {
    const cfg: RelayConfig = { ...baseConfig, client_id_hint: undefined };
    await expect(deviceLogin({ config: cfg, onPrompt: () => {} })).rejects.toThrow(/client_id/);
  });

  it("honors AbortSignal during polling", async () => {
    const ctrl = new AbortController();
    const { fetch } = scriptedFetch([
      {
        match: "/device",
        respond: () =>
          new Response(
            JSON.stringify({ device_code: "x", user_code: "x", verification_uri: "x", expires_in: 60, interval: 1 }),
            { status: 200 },
          ),
      },
      {
        match: "/token",
        respond: () => new Response(JSON.stringify({ error: "authorization_pending" }), { status: 400 }),
      },
    ]);
    setTimeout(() => ctrl.abort(), 100);
    await expect(
      deviceLogin({ config: baseConfig, onPrompt: () => {}, fetch, signal: ctrl.signal, omitOpenIDScopes: true }),
    ).rejects.toThrow();
  });
});
