# Google

::: warning Honest caveat
Google's OAuth does **not** support custom scopes for third-party
applications. Their scopes are tied to Google APIs (e.g.
`https://www.googleapis.com/auth/drive.readonly`). You cannot define
`swsrs:session:create` at Google and have it appear on a JWT.

This means **Google can authenticate identities for swsrs but cannot
authorize them via scopes**. You have three workable paths:

1. **Don't use Google.** If you're starting fresh and need scope-based
   gating, pick [Keycloak](./keycloak) or [Auth0](./auth0).
2. **Use Google as the identity bridge** — point swsrs at Google,
   accept that scope claims won't carry your custom values, and use
   group/email-based authorization at a layer above (a reverse proxy or
   a custom middleware) to gate who can mint sessions.
3. **Use a Google-fronted Auth0 / Keycloak.** Configure Google as a
   social login *into* your real IdP (which then issues real JWTs with
   real scopes). This is the cleanest mainstream pattern.

The walkthrough below covers **path 2** — using Google directly with
identity-only auth. swsrs will verify the token but accept any
Google-authenticated user (because no scope check will pass without
custom scope claims).
:::

## Path 2: identity-only with Google

This setup is appropriate when:

- You want a single-tenant deployment where "any logged-in Google
  Workspace user in our org" is the access policy.
- You'll add a reverse proxy in front of swsrs to filter by email
  domain / group before requests reach `/admin/sessions`.

It is **not** appropriate when:

- You want fine-grained scopes per user.
- You want to grant `swsrs:session:create` to some users and `read` to
  others.

If you need either, use [Keycloak](./keycloak) or [Auth0](./auth0).

## 1. Create an OAuth client in Google Cloud

1. Open **[Google Cloud Console → APIs & Services →
   Credentials](https://console.cloud.google.com/apis/credentials)**
2. Create a project if you don't have one.
3. **Configure consent screen** if prompted (Internal if Workspace,
   External otherwise).
4. **+ Create Credentials → OAuth client ID**
5. **Application type:** **TVs and Limited Input devices**

   (This is the official Google name for what RFC 8628 calls "device
   flow." Yes, really.)
6. **Name:** `swsrs`
7. **Create** — save the **Client ID** that's shown. You won't need a
   secret because TV/Limited-Input clients are public clients.

## 2. Configure the relay to accept "identity only"

Google's scope check is unworkable for our model, so we must turn off
the swsrs scope enforcement. swsrs **always** verifies signature,
issuer, audience, and expiry — only the *scope* check is bypassed
because Google won't grant our custom scopes.

::: warning
We don't currently expose a "skip scope check" flag separately —
`--no-auth` is the only way to disable scope enforcement, and it
disables OIDC verification entirely. So in this mode you'd lose the
identity check too.

**This is the showstopper for using Google directly.** Without
modifying swsrs to support a third mode ("identity check only"), there
is no way to combine "Google verifies the user" + "swsrs enforces an
allow-list" within the relay itself. Run a reverse proxy in front that
does the allow-list check, or use [path 3](#path-3-google-fronted-keycloak-auth0).
:::

::: tip
If you want to add an "identity-only" mode to swsrs (scope check off,
JWT verification on), open an issue. It's a small change but
intentionally not enabled by default — the auth split's whole point is
that the IdP grants specific scopes per user.
:::

## Path 3: Google-fronted Keycloak or Auth0

The recommended pattern when you want Google login UX with real
scope-based authorization:

1. Set up [Keycloak](./keycloak) or [Auth0](./auth0) per their guides.
2. Configure **Google as an Identity Provider** within that IdP:
   - **Keycloak:** **Identity Providers → Add provider → Google**.
     Users click "Sign in with Google" on the Keycloak login page;
     Keycloak issues its own JWT with your custom scopes.
   - **Auth0:** **Authentication → Social → Google**. Same idea.
3. Point swsrs at Keycloak/Auth0 (not at Google).

This gives you:

- Google as the user experience (single sign-on, password-free).
- Your IdP as the policy enforcement point (real scopes, real RBAC).
- swsrs unchanged — it doesn't know or care that Google is in the
  background.

## Summary

| Goal | Recommendation |
|---|---|
| "I want my users to sign in with Google AND have swsrs scope-gated auth." | Path 3 — Google → Keycloak/Auth0 → swsrs |
| "I want a quick swsrs setup for a small team and Google is the only IdP I have." | Path 2 with a reverse-proxy allow-list. Slightly clunky. |
| "I want swsrs with real scope-based gating and don't care about Google specifically." | Skip Google. Use [Keycloak](./keycloak) or [Auth0](./auth0) directly. |
