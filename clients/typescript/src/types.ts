/** Shape of the admin API's create-session response. */
export interface Session {
  id: string;
  initiator_token: string;
  responder_token: string;
  initiator_url?: string;
  responder_url?: string;
  created_at: string;
  expires_at: string;
  label?: string;
  single_attach?: boolean;
}

/** A session's own settings at creation. Unset: the server's; a TTL or peer wait over the server's is refused. */
export interface SessionOptions {
  ttlSeconds?: number;
  peerWaitSeconds?: number;
  /** The caller's own, opaque, up to 256 bytes; returned in status. */
  label?: string;
  /** Each slot attaches once, and the session closes when either peer leaves. */
  singleAttach?: boolean;
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
  label?: string;
  single_attach?: boolean;
  peer_wait_seconds?: number;
  /** The lifecycle: each slot's first attach, when both first were, when and why it closed. A closed session stays readable for the server's --closed-retention. */
  initiator_attached_at?: string;
  responder_attached_at?: string;
  opened_at?: string;
  closed_at?: string;
  close_reason?: "deleted" | "expired" | "peer_left" | "shutdown" | string;
}

/** Async function that returns an OIDC bearer token for the admin API. */
export type TokenProvider = () => Promise<string> | string;
