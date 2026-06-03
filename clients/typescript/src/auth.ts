// OAuth helpers for obtaining a token usable on the swsrs admin API.
//
// The flow:
//   1) discoverConfig(relayURL)  → reads /.well-known/swsrs-config
//   2) deviceLogin({ config, … })→ runs RFC 8628 device flow against the IdP
//   3) caller persists the resulting TokenResponse somewhere
//      (FileTokenStore in @emdzej/swsrs-client/node, or your own)
//
// Browser caveat: most IdPs do NOT enable CORS on their token endpoint for
// device flow (it's designed for CLIs, not browsers). For browser apps,
// run your own auth-code + PKCE flow with a library that handles CORS, and
// pass the resulting access token to AdminClient directly.

/** Parsed shape of /.well-known/swsrs-config. */
export interface RelayConfig {
  issuer: string;
  audience?: string;
  scopes: string[];
  authorization_endpoint: string;
  token_endpoint: string;
  device_authorization_endpoint?: string;
  client_id_hint?: string;
}

/** Thrown by discoverConfig when the relay is running with --no-auth. */
export class AuthDisabledError extends Error {
  constructor() {
    super("relay is running with auth disabled (no token needed)");
    this.name = "AuthDisabledError";
  }
}

/**
 * Fetch the relay's discovery document.
 *
 * @throws {AuthDisabledError} if the server responds 404 (--no-auth mode)
 */
export async function discoverConfig(
  relayURL: string,
  options?: { signal?: AbortSignal; fetch?: typeof fetch },
): Promise<RelayConfig> {
  if (!relayURL) throw new Error("discoverConfig: relayURL is required");
  const fetchImpl = options?.fetch ?? fetch.bind(globalThis);
  const url = relayURL.replace(/\/$/, "") + "/.well-known/swsrs-config";
  const resp = await fetchImpl(url, { signal: options?.signal });
  if (resp.status === 404) {
    throw new AuthDisabledError();
  }
  if (!resp.ok) {
    const body = await resp.text().catch(() => "");
    throw new Error(`discoverConfig: ${resp.status} ${resp.statusText}: ${body}`);
  }
  return (await resp.json()) as RelayConfig;
}

/** Successful device-flow token response (RFC 6749 §5.1). */
export interface TokenResponse {
  access_token: string;
  token_type: string;
  expires_in?: number;
  /** Computed at receive time as Date.now() + expires_in*1000 — convenient for callers. */
  expires_at?: number;
  refresh_token?: string;
  id_token?: string;
  scope?: string;
}
