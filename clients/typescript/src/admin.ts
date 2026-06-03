import type { Session, SessionStatus, TokenProvider } from "./types.js";

export interface AdminClientOptions {
  /** Relay admin base URL, e.g. "https://relay.example.com". */
  baseURL: string;
  /** Returns an OIDC bearer token. Called per request so it can refresh. */
  token: TokenProvider;
  /** Custom fetch implementation (defaults to global fetch). */
  fetch?: typeof fetch;
}

/** Client for the relay's /admin/sessions endpoints. */
export class AdminClient {
  private readonly baseURL: string;
  private readonly tokenFn: TokenProvider;
  private readonly fetchImpl: typeof fetch;

  constructor(opts: AdminClientOptions) {
    if (!opts.baseURL) throw new Error("AdminClient: baseURL is required");
    if (!opts.token) throw new Error("AdminClient: token is required");
    this.baseURL = opts.baseURL.replace(/\/$/, "");
    this.tokenFn = opts.token;
    this.fetchImpl = opts.fetch ?? fetch.bind(globalThis);
  }

  /** Allocate a new relay session. Returns id + per-slot tokens. */
  async createSession(signal?: AbortSignal): Promise<Session> {
    return this.request<Session>("POST", "/admin/sessions", signal);
  }

  /** Fetch current status of a session. */
  async getSession(id: string, signal?: AbortSignal): Promise<SessionStatus> {
    return this.request<SessionStatus>("GET", `/admin/sessions/${encodeURIComponent(id)}`, signal);
  }

  /** List all sessions visible to the caller. */
  async listSessions(signal?: AbortSignal): Promise<SessionStatus[]> {
    const resp = await this.request<{ sessions: SessionStatus[] }>("GET", "/admin/sessions", signal);
    return resp.sessions;
  }

  /** Terminate a session. Idempotent on the caller side. */
  async deleteSession(id: string, signal?: AbortSignal): Promise<void> {
    await this.request<void>("DELETE", `/admin/sessions/${encodeURIComponent(id)}`, signal);
  }

  private async request<T>(method: string, path: string, signal?: AbortSignal): Promise<T> {
    const token = await this.tokenFn();
    const resp = await this.fetchImpl(this.baseURL + path, {
      method,
      headers: { Authorization: `Bearer ${token}` },
      signal,
    });
    if (!resp.ok) {
      const body = await resp.text().catch(() => "");
      throw new AdminError(`${method} ${path} -> ${resp.status} ${resp.statusText}: ${body}`, resp.status);
    }
    if (resp.status === 204) return undefined as T;
    return (await resp.json()) as T;
  }
}

export class AdminError extends Error {
  constructor(message: string, public readonly status: number) {
    super(message);
    this.name = "AdminError";
  }
}
