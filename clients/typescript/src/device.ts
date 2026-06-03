// Device authorization grant (RFC 8628) implementation.
//
// Browser caveat: most IdPs don't enable CORS on the token endpoint when
// used for device flow. If you call deviceLogin() from a browser context
// and see a CORS error, that's expected — device flow is designed for
// CLIs / native apps. See discoverConfig docs for the recommended browser
// alternative.

import type { RelayConfig, TokenResponse } from "./auth.js";

export interface DevicePrompt {
  /** The short code the user must enter at verificationUri. */
  userCode: string;
  /** Where the user should go to enter the code. */
  verificationUri: string;
  /** Some IdPs (e.g. Google) supply a URL with the code pre-embedded. */
  verificationUriComplete?: string;
  /** When the user_code expires. */
  expiresAt: Date;
  /** Poll interval the IdP wants us to honor. */
  interval: number;
}

export interface DeviceLoginOptions {
  config: RelayConfig;
  /** OAuth client_id. Defaults to config.client_id_hint. */
  clientId?: string;
  /** Scopes to request. Defaults to config.scopes plus `openid offline_access`. */
  scopes?: string[];
  /** Called once the IdP returns a user_code. Caller renders it however. */
  onPrompt: (prompt: DevicePrompt) => void | Promise<void>;
  /** Aborts the device flow (both initial request and polling). */
  signal?: AbortSignal;
  /** Disable the auto-added `openid` / `offline_access` scopes. */
  omitOpenIDScopes?: boolean;
  /** Custom fetch impl (tests). */
  fetch?: typeof fetch;
}

/**
 * Run the OAuth 2.0 Device Authorization Grant against the IdP discovered
 * by {@link discoverConfig}. Blocks until the user completes login or the
 * device code expires.
 */
export async function deviceLogin(opts: DeviceLoginOptions): Promise<TokenResponse> {
  const cfg = opts.config;
  if (!cfg.device_authorization_endpoint) {
    throw new Error("deviceLogin: IdP does not advertise device_authorization_endpoint");
  }
  if (!cfg.token_endpoint) {
    throw new Error("deviceLogin: discovery missing token_endpoint");
  }
  const clientId = opts.clientId ?? cfg.client_id_hint;
  if (!clientId) {
    throw new Error("deviceLogin: no clientId supplied and discovery had no client_id_hint");
  }

  let scopes = opts.scopes ?? [...cfg.scopes];
  if (!opts.omitOpenIDScopes) {
    if (!scopes.includes("openid")) scopes = [...scopes, "openid"];
    if (!scopes.includes("offline_access")) scopes = [...scopes, "offline_access"];
  }

  const fetchImpl = opts.fetch ?? fetch.bind(globalThis);

  // Step 1: request a device code.
  const daResp = await fetchImpl(cfg.device_authorization_endpoint, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded", Accept: "application/json" },
    body: new URLSearchParams({ client_id: clientId, scope: scopes.join(" ") }),
    signal: opts.signal,
  });
  if (!daResp.ok) {
    const body = await daResp.text().catch(() => "");
    throw new Error(`deviceLogin: device authorization ${daResp.status}: ${body}`);
  }
  const da = (await daResp.json()) as {
    device_code: string;
    user_code: string;
    verification_uri: string;
    verification_uri_complete?: string;
    expires_in: number;
    interval?: number;
  };

  const expiresAt = new Date(Date.now() + da.expires_in * 1000);
  let interval = da.interval ?? 5;

  await opts.onPrompt({
    userCode: da.user_code,
    verificationUri: da.verification_uri,
    verificationUriComplete: da.verification_uri_complete,
    expiresAt,
    interval,
  });

  // Step 2: poll the token endpoint until the user completes or we time out.
  while (true) {
    if (opts.signal?.aborted) throw new DOMException("aborted", "AbortError");
    if (Date.now() > expiresAt.getTime()) {
      throw new Error("deviceLogin: device code expired before user authenticated");
    }
    await sleep(interval * 1000, opts.signal);

    const tokResp = await fetchImpl(cfg.token_endpoint, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded", Accept: "application/json" },
      body: new URLSearchParams({
        grant_type: "urn:ietf:params:oauth:grant-type:device_code",
        device_code: da.device_code,
        client_id: clientId,
      }),
      signal: opts.signal,
    });
    if (tokResp.ok) {
      const tok = (await tokResp.json()) as TokenResponse;
      if (tok.expires_in) tok.expires_at = Date.now() + tok.expires_in * 1000;
      return tok;
    }
    // RFC 8628 §3.5: errors come back as JSON with an `error` field.
    const body = (await tokResp.json().catch(() => ({}))) as { error?: string; error_description?: string };
    switch (body.error) {
      case "authorization_pending":
        // keep polling
        break;
      case "slow_down":
        interval += 5; // RFC 8628 says we MUST increase by 5s
        break;
      case "access_denied":
        throw new Error("deviceLogin: user denied authorization");
      case "expired_token":
        throw new Error("deviceLogin: device code expired");
      default:
        throw new Error(
          `deviceLogin: token endpoint ${tokResp.status}: ${body.error ?? "unknown error"}` +
            (body.error_description ? ` — ${body.error_description}` : ""),
        );
    }
  }
}

function sleep(ms: number, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      signal?.removeEventListener("abort", onAbort);
      resolve();
    }, ms);
    const onAbort = () => {
      clearTimeout(timer);
      reject(new DOMException("aborted", "AbortError"));
    };
    if (signal?.aborted) return onAbort();
    signal?.addEventListener("abort", onAbort, { once: true });
  });
}
