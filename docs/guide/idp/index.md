# IdP setup

swsrs's admin API verifies OIDC JWTs and gates routes by **custom
scope claims** (`swsrs:session:create`, `…:read`, `…:delete`). For this
to work end-to-end you need an IdP that supports:

1. **OIDC discovery** — `.well-known/openid-configuration` endpoint
2. **Custom scopes** (or roles mapped to scopes) on access tokens
3. **OAuth 2.0 device authorization grant** (RFC 8628) — so `swsrs auth`
   can run a browser-less login on the client side

Not every IdP supports all three. Quick recommendation matrix:

| IdP | Custom scopes | Device flow | OIDC discovery | Recommendation |
|---|---|---|---|---|
| **[Keycloak](./keycloak)**  | ✓ Native | ✓ Native | ✓ | **Best fit.** Self-hosted, free, all the right primitives. |
| **[Auth0](./auth0)**         | ✓ Native (Custom APIs) | ✓ Native | ✓ | **Best SaaS option.** Generous free tier. |
| **Okta**                     | ✓ Native (Custom Authorization Server) | ✓ Native | ✓ | Works well; not documented here yet. |
| **Microsoft Entra ID (Azure AD)** | ✓ (App Roles + custom scopes) | ✓ Native | ✓ | Works; uses `scp` claim instead of `scope`. swsrs supports both. |
| **[Google](./google)**       | ✗ **Not for arbitrary apps** | ✓ Native | ✓ | **Identity-only.** Cannot grant `swsrs:*` scopes. See the page for the workaround. |
| **AWS Cognito**              | ✓ (Resource Servers) | ✓ Native | ✓ | Works; not documented here yet. |
| **GitHub OAuth**             | ✗ (predefined scopes) | ✗ | ✗ (not OIDC) | Not usable directly. |

The two we recommend and have written end-to-end guides for are
**Keycloak** (self-hosted) and **Auth0** (SaaS). Pick whichever fits
your existing infrastructure.

## Required client-side configuration

Regardless of IdP, swsrs needs:

| Server env var | What goes in it |
|---|---|
| `SWSRS_OIDC_ISSUER`     | Your IdP's issuer URL (the value the IdP advertises at `/.well-known/openid-configuration` as `issuer`) |
| `SWSRS_OIDC_AUDIENCE`   | The `aud` your IdP puts in access tokens for swsrs (usually the API/resource-server identifier) |
| `SWSRS_OIDC_CLIENT_ID`  | The shared OAuth `client_id` your end-user clients use with device flow |

## Required scopes

The admin API enforces these on the JWT:

- `swsrs:session:create` — required for `POST /admin/sessions`
- `swsrs:session:read`   — required for `GET /admin/sessions[/{id}]`
- `swsrs:session:delete` — required for `DELETE /admin/sessions/{id}`

A typical client app only needs `swsrs:session:create`. See
[Authentication](/guide/auth) for the model.

## Pick your IdP

- [Keycloak](./keycloak) — recommended for self-hosted
- [Auth0](./auth0) — recommended for SaaS
- [Google](./google) — identity-only, with honest caveats
