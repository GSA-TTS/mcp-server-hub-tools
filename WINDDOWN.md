# Wind-Down Notes — GSA-TTS fork of Obot `tools` (login.gov auth provider)

> **Status:** The GSA-TTS MCP Server Hub effort is being **wound down.** This
> file explains what this fork customizes and how to resume it. It is a snapshot
> for developers, not GSA policy.
>
> **This repo is one of four.** See the cross-repo map in the
> [`GSA-TTS/mcp-server-hub`](https://github.com/GSA-TTS/mcp-server-hub) repo
> (`planning/WINDDOWN-INDEX.md`) for how `mcp-server-hub`, the `obot` fork,
> `mcp-server-hub-catalog`, and this repo fit together.

---

## 1. What this repo is

A GSA-TTS **fork of upstream `obot-platform/tools`** — Obot's monorepo of
pluggable **model providers** and **auth providers**. The fork exists to hold
**one** customization:

> A new **login.gov auth provider** (`login.gov-auth-provider/`) so Obot can
> authenticate users via login.gov (OIDC), alongside the stock github/google
> providers.

Everything else is upstream. `index.yaml` is the registry manifest; `tool.gpt`
at the root loads it. The full login.gov *integration roadmap* (Partner Portal
setup, sandbox app, resume steps) lives in the `mcp-server-hub` repo at
`planning/login_auth_roadmap.md` — **read that alongside this file.**

## 2. Fork status vs. upstream

- **Remote (`origin`):** `https://github.com/GSA-TTS/mcp-server-hub-tools.git`
  (the fork).
- **No `upstream` remote configured.** The obot origin survives only in Go
  module paths (`github.com/obot-platform/tools/...`). To sync upstream:
  `git remote add upstream https://github.com/obot-platform/tools.git`.
- The GSA content (login.gov provider + 2 registry edits) is now merged to the
  fork's `main` (branch `add-login-auth`, PR #3). Aside from that, history is
  upstream commits.

## 3. The GSA customization — login.gov auth provider

**Files (all under `login.gov-auth-provider/`):** `main.go` (190 lines),
`pkg/profile/profile.go` (+ `_test.go`), `tool.gpt`, `go.mod`/`go.sum`.
**Registry edits:** `index.yaml` (registers the provider under `authProviders`)
and `Makefile` (adds it to the `test` target).

**How it works:**
- Go daemon (`#!sys.daemon`) on `127.0.0.1:9999` (or `$PORT`) wrapping
  oauth2-proxy. **Modeled on the `google-auth-provider`** — same structure,
  shared `state.ObotGetState` helper, same `FetchXProfile` pattern. Look at
  google's provider first to understand the common shape.
- OIDC with login.gov **`private_key_jwt`** client authentication — **no client
  secret.** Instead: `ClientID` is the SP issuer URN, `JWTKey` (RSA private key
  PEM) signs the `client_assertion`, and `PubJWKURL` publishes the verifying
  JWKS.
- **Environment switch:** `sandbox` (`idp.int.identitysandbox.gov`) vs
  `production` (`secure.login.gov`). `main.go` sets all four OIDC endpoints
  explicitly (authorize/token/userinfo) because the underlying `logingov.go`
  only fills empty values and defaults to production.
- Scope `openid email`; `acr_values` defaults to `urn:acr.login.gov:auth-only`
  (AAL2 — MFA, **no IAL2 identity proofing**). Cookie session (`obot_access_token`),
  optional Postgres store (`logingov_` table prefix), email-domain allowlist.
- Obot integration routes: `/obot-get-state`, `/obot-get-user-info` (calls
  `profile.FetchLoginGovProfile` → `sub`, `email`, name), and
  `/obot-list-user-auth-groups` (returns **404** — groups intentionally
  unsupported).

## 4. Critical dependency — forked oauth2-proxy

`login.gov-auth-provider/go.mod` pins a **fork of oauth2-proxy** via a `replace`
directive:

```
github.com/oauth2-proxy/oauth2-proxy/v7 => github.com/obot-platform/oauth2-proxy/v7 v7.0.0-20260410175959-7ef5428d1af3
```

That fork contains the `logingov.go` provider that this daemon drives. **The
provider will not work without it.** A future maintainer must track and refresh
this fork for security updates — it is a pinned pseudo-version, not an official
release.

## 5. Configuration / secrets (names only)

login.gov-specific (from `tool.gpt`):
- `OBOT_LOGINGOV_AUTH_PROVIDER_CLIENT_ID` — SP issuer/URN (not sensitive)
- `OBOT_LOGINGOV_AUTH_PROVIDER_JWT_KEY` — **RSA private key PEM (SENSITIVE)**
- `OBOT_LOGINGOV_AUTH_PROVIDER_PUBJWK_URL` — public JWKS URL
- `OBOT_LOGINGOV_AUTH_PROVIDER_ACR_VALUES` — optional, default `urn:acr.login.gov:auth-only`
- `OBOT_LOGINGOV_AUTH_PROVIDER_ENVIRONMENT` — optional, `sandbox`|`production` (default `sandbox`)

Shared auth-provider vars:
- `OBOT_AUTH_PROVIDER_COOKIE_SECRET` — **SENSITIVE** (base64, 16/24/32 bytes)
- `OBOT_AUTH_PROVIDER_EMAIL_DOMAINS` (default `*`)
- `OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_DSN` — optional, **SENSITIVE**
- `OBOT_AUTH_PROVIDER_TOKEN_REFRESH_DURATION` (default `1h`), `OBOT_AUTH_PROVIDER_ENABLE_LOGGING`
- `OBOT_SERVER_PUBLIC_URL` / `OBOT_SERVER_URL`, `PORT`, `GPTSCRIPT_TOOL_DIR`

**No secret values are committed.** The provider icon is referenced from the
catalog repo: `.../GSA-TTS/mcp-server-hub-catalog/main/icons/login_gov.png`.

## 6. Known gaps / blockers

> **Update (2026-10-06):** cloud.gov HTTPS is live. Stage 3 work now packages
> this provider into a GSA provider-registry image and configures it through
> Obot's encrypted Auth Providers UI/API. The table below is the earlier AWS
> snapshot.

| Item | Notes |
|------|-------|
| **End-to-end testing blocked on HTTPS** | login.gov requires an HTTPS redirect; the hub's ALB is HTTP-only. This is the hard blocker (see `mcp-server-hub` ROADMAP + login_auth_roadmap). |
| **Partner Portal redirect URI is a `localhost` placeholder** | Per `login_auth_roadmap.md`, the sandbox app's redirect URI still points at `http://localhost:3000/auth/callback` and must be replaced with the real Obot callback. |
| **No README inside the provider dir** | Setup/Partner-Portal steps live only in the hub's `planning/login_auth_roadmap.md`. |
| **Deploy image unverified** | The provider must ship in the Obot providers image built linux/amd64; not validated end to end. |
| **No IAL2 / identity proofing** | By design — `ial`/`aal`/`verified_at` claims are deliberately not surfaced. Auth-only (AAL2). |

## 7. Verification (as of wind-down)

- `cd login.gov-auth-provider && go build ./...` — **passes** (confirmed).
- `cd login.gov-auth-provider && go test ./...` — **passes** (`pkg/profile`
  unit tests OK; provider root has no test files).

## 8. If you resume — start here

1. Read `mcp-server-hub/planning/login_auth_roadmap.md` — it has a "RESUME HERE"
   progress log and an ordered "Next actions" list. It is the authoritative
   resume guide.
2. **Unblock HTTPS on the gateway first** (obot ALB) — nothing round-trips
   without it.
3. Fix the Partner Portal **redirect URI** (replace `localhost` with the real
   Obot callback URL from the Admin UI).
4. Add an `upstream` remote and rebase onto current `obot-platform/tools` before
   building further.
5. Confirm/refresh the **forked oauth2-proxy** pin (§4); it is required.
6. Compare against `google-auth-provider/` for the common provider pattern.

## 9. Related repos

- **`GSA-TTS/mcp-server-hub`** — umbrella project + all planning docs
  (`planning/login_auth_roadmap.md` is the login.gov resume guide). Start there.
- **`GSA-TTS/obot`** — the gateway fork (Cloud Foundry runtime backend).
- **`GSA-TTS/mcp-server-hub-catalog`** — MCP server catalog; hosts the
  `login_gov.png` icon this provider references.

## 10. Provenance

- **Remote:** `https://github.com/GSA-TTS/mcp-server-hub-tools.git`
- Point of contact: _(fill in team/POC before archiving)_
- The login.gov provider and this wind-down documentation were AI-assisted and
  require human review before being relied upon operationally.
