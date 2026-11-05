# Admin API

All admin endpoints sit under `/admin/sessions` and expect
`Authorization: Bearer <jwt>` with the appropriate scope (see
[Authentication](/guide/auth#admin-plane-oidc-jwt-scope-gated)).

## Create — `POST /admin/sessions`

Mint a new session. The response carries the per-slot tokens — the only
time they're ever returned. Store them or hand them to the parties
immediately; the server itself never reveals tokens again.

**Scope:** `swsrs:session:create`

```http
POST /admin/sessions HTTP/1.1
Host: relay.example.com
Authorization: Bearer <jwt>
```

The body is optional. Without one the session takes the server's settings; with one it can have its
own:

```json
{ "ttl_seconds": 900, "peer_wait_seconds": 120, "label": "support:ticket-42", "single_attach": true }
```

| Field | Meaning |
|---|---|
| `ttl_seconds` | Its lifetime. 0: the server's `--session-ttl`, which is also the most it may ask for |
| `peer_wait_seconds` | How long a peer waits for the other. 0: the server's `--peer-wait`, also the most |
| `label` | Yours, opaque, up to 256 bytes (what the session is for); returned in status, never read |
| `single_attach` | Each slot attaches once, and the session closes when either peer leaves (`close_reason` `peer_left`); a token can't be used again. For sessions a person ends by leaving |

Unknown fields, a negative value, or more than the server allows: `400` with `{"error": "..."}`.

Response (`201 Created`):

```json
{
  "id": "OVMnxnEzZA9QvFc...",
  "initiator_token": "Gi-jvZfpHgahXc618jUP8A",
  "responder_token": "tKuylZcPfwjvy43...",
  "initiator_url": "wss://relay.example.com/relay/OVM...",
  "responder_url": "wss://relay.example.com/relay/OVM...",
  "created_at": "2026-06-03T12:00:00Z",
  "expires_at": "2026-06-03T13:00:00Z",
  "label": "support:ticket-42",
  "single_attach": true
}
```

`initiator_url` and `responder_url` are convenience fields; they're
populated only when `SWSRS_PUBLIC_BASE_URL` is configured.

## List — `GET /admin/sessions`

List all sessions.

**Scope:** `swsrs:session:read`

Response (`200 OK`):

```json
{
  "sessions": [
    { "id": "...", "state": "open", "created_at": "...", "expires_at": "...",
      "last_activity": "...", "bytes_in": 1234, "bytes_out": 4321,
      "initiator_connected": true, "responder_connected": true }
  ]
}
```

State values: `pending` → `half_open` → `open` → `closed`.

Each item also carries its options (`label`, `single_attach`, `peer_wait_seconds`, when given) and its
lifecycle, for a control plane polling it: `initiator_attached_at` and `responder_attached_at` (each
slot's first attach), `opened_at` (when both first were), `closed_at` and `close_reason` (`deleted`,
`expired`, `peer_left`, `shutdown`).

A closed session stays readable, `state` `closed`, for the server's `--closed-retention` (10 minutes by
default), so whoever made it can see how it ended and how many bytes it carried. Its tokens are useless
from the moment it closes: the data plane answers `410 Gone`.

## Get — `GET /admin/sessions/{id}`

Get a single session's status. Same shape as the list response items.

**Scope:** `swsrs:session:read`

`404 Not Found` if the session doesn't exist, or closed longer than `--closed-retention` ago.

## Delete — `DELETE /admin/sessions/{id}`

Terminate a session immediately. Connected peers see a close frame. Its status stays readable for
`--closed-retention`, with `close_reason` `deleted`.

**Scope:** `swsrs:session:delete`

Response: `204 No Content` (or `404` if already gone).

## Errors

| Status | Meaning |
|---|---|
| `401 Unauthorized` | Missing or invalid bearer token |
| `403 Forbidden`    | Valid token, but missing the required scope |
| `404 Not Found`    | Session id unknown or session deleted/expired |
