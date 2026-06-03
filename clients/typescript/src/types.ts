/** Shape of the admin API's create-session response. */
export interface Session {
  id: string;
  initiator_token: string;
  responder_token: string;
  initiator_url?: string;
  responder_url?: string;
  created_at: string;
  expires_at: string;
}

/** Shape of the admin API's get/list status response. */
export interface SessionStatus {
  id: string;
  state: "pending" | "half_open" | "open" | "closed" | string;
  created_at: string;
  expires_at: string;
  last_activity: string;
  bytes_in: number;
  bytes_out: number;
  initiator_connected: boolean;
  responder_connected: boolean;
}

/** Async function that returns an OIDC bearer token for the admin API. */
export type TokenProvider = () => Promise<string> | string;
