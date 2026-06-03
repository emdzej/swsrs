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

Response (`201 Created`):

```json
{
  "id": "OVMnxnEzZA9QvFc...",
  "initiator_token": "Gi-jvZfpHgahXc618jUP8A",
  "responder_token": "tKuylZcPfwjvy43...",
  "initiator_url": "wss://relay.example.com/relay/OVM...",
  "responder_url": "wss://relay.example.com/relay/OVM...",
  "created_at": "2026-06-03T12:00:00Z",
  "expires_at": "2026-06-03T13:00:00Z"
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

## Get — `GET /admin/sessions/{id}`

Get a single session's status. Same shape as the list response items.

**Scope:** `swsrs:session:read`

`404 Not Found` if the session doesn't exist or has been reaped.

## Delete — `DELETE /admin/sessions/{id}`

Terminate a session immediately. Connected peers see a close frame.

**Scope:** `swsrs:session:delete`

Response: `204 No Content` (or `404` if already gone).

## Errors

| Status | Meaning |
|---|---|
| `401 Unauthorized` | Missing or invalid bearer token |
| `403 Forbidden`    | Valid token, but missing the required scope |
| `404 Not Found`    | Session id unknown or session deleted/expired |
