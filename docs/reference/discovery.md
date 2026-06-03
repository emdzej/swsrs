# Discovery endpoint

`GET /.well-known/swsrs-config` is public and tells clients where the
relay's IdP lives and which scopes the admin API requires. It exists so
clients can run OAuth flows themselves (device flow, PKCE) without
hard-coded IdP coordinates.

## Response

`200 OK` (`Cache-Control: public, max-age=300`):

```json
{
  "issuer": "https://auth.example.com/realms/myproduct",
  "audience": "swsrs",
  "scopes": [
    "swsrs:session:create",
    "swsrs:session:read",
    "swsrs:session:delete"
  ],
  "authorization_endpoint": "https://auth.example.com/realms/myproduct/protocol/openid-connect/auth",
  "token_endpoint":         "https://auth.example.com/realms/myproduct/protocol/openid-connect/token",
  "device_authorization_endpoint": "https://auth.example.com/realms/myproduct/protocol/openid-connect/auth/device",
  "client_id_hint": "swsrs"
}
```

| Field | Notes |
|---|---|
| `issuer`                          | OIDC issuer URL (from `SWSRS_OIDC_ISSUER`) |
| `audience`                        | Expected `aud` claim (from `SWSRS_OIDC_AUDIENCE`); empty when unset |
| `scopes`                          | The full set of swsrs scopes the admin API knows about |
| `authorization_endpoint`          | IdP's auth-code endpoint (from OIDC discovery) |
| `token_endpoint`                  | IdP's token endpoint |
| `device_authorization_endpoint`   | IdP's RFC 8628 endpoint, if advertised. Empty when the IdP doesn't support device flow. |
| `client_id_hint`                  | Shared OAuth `client_id` (from `SWSRS_OIDC_CLIENT_ID`). Optional. Clients may use their own client_id and ignore this. |

## `--no-auth` mode

When the relay is running `--no-auth`, this endpoint returns **404**:

```
HTTP/1.1 404 Not Found
Content-Type: text/plain; charset=utf-8

auth disabled on this deployment (--no-auth)
```

Both SDKs map this to a sentinel (`auth.ErrAuthDisabled` in Go,
`AuthDisabledError` in TS) so callers can treat it as "no token
needed".

## Why a custom path

The OIDC spec already defines `.well-known/openid-configuration` —
that's the IdP's discovery doc, served by the IdP itself, not by us.
swsrs publishes its own `.well-known/swsrs-config` because it has
relay-specific metadata (the supported scopes, the shared `client_id_hint`)
that doesn't belong in the IdP's response.
