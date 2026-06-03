# Keycloak

Keycloak is the recommended self-hosted IdP for swsrs. It has every
primitive we need: custom OAuth scopes per realm, device authorization
grant, OIDC discovery, and JWKS.

This guide walks you through the setup that powers
[connect.bimmerz.app](https://connect.bimmerz.app) (the reference
deployment).

## Prerequisites

- A Keycloak instance reachable from your relay (so OIDC discovery
  works). Cloud-hosted, self-hosted, or `kc.sh start-dev` for local
  testing.
- Admin access to a realm — we'll call it `myproduct`.

## 1. Create the swsrs realm or pick an existing one

Pick (or create) the realm you'll use. The issuer URL will be:

```
https://your-keycloak.example.com/realms/myproduct
```

That value goes in `SWSRS_OIDC_ISSUER` on the relay.

## 2. Create a Keycloak client for swsrs

This single client serves two purposes:

- **Resource server** — its name becomes the `aud` claim on access
  tokens; swsrs uses it to enforce audience.
- **Public OAuth client** — your CLI / SDK clients use it as
  `client_id` for device flow.

In Keycloak admin console:

1. **Clients → Create client**
2. **Client type:** `OpenID Connect`
3. **Client ID:** `swsrs` (this becomes both the audience and the
   `client_id_hint`)
4. **Next → Capability config:**
   - **Client authentication:** **OFF** (public client — required for
     device flow without a secret)
   - **Authentication flow:** check **OAuth 2.0 Device Authorization
     Grant**. Uncheck `Standard flow` and `Direct access grants`
     unless you actually need them.
   - **Service accounts roles:** off (this client represents end-user
     identities, not the relay itself)
5. **Save**.

## 3. Define the swsrs scopes

Keycloak calls these "Client Scopes."

1. **Client scopes → Create client scope**, three times — once for each
   of:
   - `swsrs:session:create`
   - `swsrs:session:read`
   - `swsrs:session:delete`
2. For each:
   - **Type:** `Optional` (so users get them only when assigned the
     matching role)
   - **Display on consent screen:** off (CLI users shouldn't see a
     consent prompt for device flow)
   - **Include in token scope:** **on** (so the scope appears in the
     `scope` claim on access tokens — this is what swsrs reads)

## 4. Add the scopes to the swsrs client

1. **Clients → `swsrs` → Client scopes → Add client scope**
2. Add all three swsrs scopes as **Optional** (the client asks for them
   per-request via the `scope=` parameter; the IdP grants them based on
   the user's roles).

## 5. Create realm roles for scope mapping

We want a Keycloak role to act as "permission to create swsrs sessions."

1. **Realm roles → Create role**
   - **Role name:** `swsrs-creator`
2. Repeat for `swsrs-reader` and `swsrs-admin` if you want fine-grained
   access. For most setups, one role with all three scopes is enough —
   call it `swsrs-operator`.

## 6. Map roles to scopes (the bridge)

For each swsrs client scope you defined, add a **Mapper** that emits the
scope name when the user has the corresponding role.

A simple way: use a **Hardcoded scope** mapper.

1. **Client scopes → `swsrs:session:create` → Mappers → Add mapper**
2. **By configuration → Hardcoded claim**
3. Set:
   - **Token claim name:** `scope`
   - **Claim value:** `swsrs:session:create`
   - **Claim JSON Type:** `String`
   - **Add to access token:** on
   - **Add to ID token:** off
   - **Multivalued:** off (Keycloak appends to the existing scope
     string when it's a `String` claim with the same name)
4. Save.
5. Repeat for `swsrs:session:read` and `swsrs:session:delete`.

Actually — **the simpler path** is to give the client scope's name the
same value as the scope you want emitted (which we already did), and
**use the default "Audience" + "Roles" + "User Realm Role" mappers
Keycloak ships**. Read this back: any scope listed in a token's
`scope` claim was either default-included on the client or explicitly
requested. Since we made these scopes Optional and asked for them by
name, they appear automatically when granted.

If a user shouldn't have a scope, **don't grant the role** in the next
step — the access token simply won't carry that scope, and swsrs
returns 403.

## 7. Assign the role(s) to users

For each user (or group) that should mint sessions:

1. **Users → \<user\> → Role mapping → Assign role**
2. Assign `swsrs-operator` (or the specific subset you defined).

For most apps you only need to grant `swsrs:session:create` — your end
users don't need read/delete. See [Authentication](/guide/auth#minimum-scopes-for-typical-roles).

## 8. (Optional) Configure audience claim explicitly

By default, Keycloak puts the client_id in the `aud` claim of access
tokens **only when** the same client requests the token (i.e., a token
issued *for* that client, not just *by* it).

If you ever see swsrs reject tokens with `audience mismatch` errors,
add an **Audience mapper** to the `swsrs` client:

1. **Clients → `swsrs` → Client scopes → swsrs-dedicated → Add mapper**
2. **By configuration → Audience**
3. **Included Client Audience:** `swsrs`
4. **Add to access token:** on

## 9. Configure the relay

Wire it all together:

```bash
SWSRS_OIDC_ISSUER=https://your-keycloak.example.com/realms/myproduct \
SWSRS_OIDC_AUDIENCE=swsrs \
SWSRS_OIDC_CLIENT_ID=swsrs \
swsrs serve
```

Or via Docker:

```yaml
services:
  swsrs:
    image: ghcr.io/emdzej/swsrs:latest
    environment:
      SWSRS_OIDC_ISSUER:    https://your-keycloak.example.com/realms/myproduct
      SWSRS_OIDC_AUDIENCE:  swsrs
      SWSRS_OIDC_CLIENT_ID: swsrs
      SWSRS_ALLOWED_ORIGINS: app.example.com,*.dev.example.com
```

## 10. Smoke test

```bash
# Discovery should return Keycloak's endpoints
curl -s https://relay.example.com/.well-known/swsrs-config | jq

# Run device flow
swsrs auth --relay https://relay.example.com
# follow the Keycloak login URL, sign in, get the code

# Create a session
swsrs create --admin-url https://relay.example.com
# {"id":"...", "initiator_token":"...", "responder_token":"...", ...}
```

If you got back JSON with two tokens, you're done.

## Troubleshooting

| Symptom | Likely cause |
|---|---|
| `401 invalid token: oidc: id token signed by alg "RS256" cannot be verified` | Issuer URL mismatch. The `iss` claim in the token must exactly equal `SWSRS_OIDC_ISSUER`. Trailing-slash differences count. |
| `401 invalid token: oidc: id token issued by ...` | Same as above. |
| `403 missing required scope: swsrs:session:create` | User doesn't have the role that maps to the scope. Check **Users → \<user\> → Role mappings**. |
| `401 invalid token: oidc: audience claim ...` | Token's `aud` doesn't include `swsrs`. Add the audience mapper from step 8. |
| `swsrs auth` says "IdP does not advertise device_authorization_endpoint" | Verify in your IdP that device authorization grant is enabled for the client (step 2). |
| Discovery returns `device_authorization_endpoint: ""` | Keycloak's discovery doc lists device endpoint only when at least one client has the grant enabled. Recycle (`/admin/clear-keys`) if needed. |
