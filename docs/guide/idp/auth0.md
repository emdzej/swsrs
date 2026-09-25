# Auth0

Auth0 is the recommended SaaS IdP for swsrs. It has a generous free
tier and supports everything we need: Custom APIs with arbitrary
scopes, OIDC discovery, and device flow.

The Auth0 vocabulary doesn't match Keycloak's, so the mapping is:

| swsrs concept | Auth0 concept |
|---|---|
| Resource server / audience | **Custom API** (its "Identifier") |
| Scope (`swsrs:session:create`) | **Permission** on the Custom API |
| OAuth client | **Application** (Native type for device flow) |

## 1. Create a Custom API in Auth0

This is the resource server that represents swsrs.

1. **Auth0 Dashboard → APIs → + Create API**
2. **Name:** `swsrs`
3. **Identifier:** `swsrs` (or a stable URN — this becomes the `aud`
   claim and the `SWSRS_OIDC_AUDIENCE` value; **change it once and
   never again**, Auth0 won't let you edit it later)
4. **Signing algorithm:** `RS256`
5. **Create**

On the resulting API page:

6. **Settings → Allow Offline Access:** on (so refresh tokens work)
7. **Permissions** tab → add three permissions:
   - `swsrs:session:create` — "Create sessions"
   - `swsrs:session:read`   — "Read sessions"
   - `swsrs:session:delete` — "Delete sessions"

## 2. Create a Native Application for end-user clients

Public OAuth client, used by `swsrs auth` and the SDKs.

1. **Applications → + Create Application**
2. **Name:** `swsrs CLI`
3. **Application Type:** **Native** (required for device flow without a
   client secret)
4. **Create**

On the application's **Settings**:

5. **Token Endpoint Authentication Method:** `None` (it's a public
   client)
6. **Grant Types** (under Advanced Settings → Grant Types):
   - **Enable:** `Device Code`, `Refresh Token`
   - **Disable** everything else unless you need it for other purposes.
7. **Save changes**

Copy the **Client ID** from the top — that's your
`SWSRS_OIDC_CLIENT_ID`.

## 3. Authorize the application to call the API

1. **Applications → swsrs CLI → APIs** tab
2. Toggle **Authorized** for the `swsrs` API.
3. Expand it and check the permissions you want the client to be able
   to request (typically all three).

Important nuance: enabling a permission here lets the client **ask for**
it. Whether a specific user actually gets it on their token is governed
by **role assignments** (next step) plus your **RBAC settings**.

## 4. Enable RBAC + permissions on the API

Back on **APIs → swsrs → Settings**:

1. **RBAC Settings:**
   - **Enable RBAC:** on
   - **Add Permissions in the Access Token:** on (this is what makes
     scopes appear in the `scope` claim that swsrs reads)

## 5. Create roles for scope mapping

1. **User Management → Roles → + Create Role**
   - **Name:** `swsrs-operator` (or `swsrs-creator` for the narrow role)
2. **Permissions** tab on the role → **Add Permissions**
   - Select the `swsrs` API and the permissions this role should grant.
3. **User Management → Users → \<user\> → Roles → Assign Roles** —
   assign the role to each user that should be able to mint sessions.

For most apps you'll grant only `swsrs:session:create` to your users.

## 6. Configure the relay

Auth0's issuer URL is `https://<your-tenant>.<region>.auth0.com/`
(trailing slash required — Auth0 is picky about it):

```bash
SWSRS_OIDC_ISSUER=https://your-tenant.us.auth0.com/ \
SWSRS_OIDC_AUDIENCE=swsrs \
SWSRS_OIDC_CLIENT_ID=<the Client ID from step 2> \
swsrs serve
```

## 7. Smoke test

```bash
# Discovery should now return Auth0's endpoints
curl -s https://relay.example.com/.well-known/swsrs-config | jq

# Run device flow
swsrs auth --relay https://relay.example.com

# Create a session
swsrs create --admin-url https://relay.example.com
```

## Troubleshooting

The admin API answers only `401 invalid token` or `403 insufficient scope`.
The reason is in the relay's log as an `admin auth rejected` entry (`err`
or `missing_scope` field); the table below quotes those fields.

| Symptom | Likely cause |
|---|---|
| `err`: `audience claim ... not valid` | Token was issued for Auth0's `userinfo` endpoint, not for your API. Make sure your client code requests `audience=swsrs` (the SDK does this automatically when `SWSRS_OIDC_CLIENT_ID` is set and discovery surfaces the audience). |
| `missing_scope`: `swsrs:session:create` | RBAC is off, OR the user doesn't have the role with that permission, OR "Add Permissions in the Access Token" is off (step 4). |
| Discovery returns empty `device_authorization_endpoint` | Auth0 always advertises this; if it's empty, your relay couldn't reach the tenant. Check egress firewall. |
| `swsrs auth` says "not authorized for grant_type:device_code" | Step 2.6 wasn't completed — device code grant isn't enabled on the application. |
| `swsrs auth` hangs on poll forever | User closed the browser tab without completing the consent. Re-run. |
