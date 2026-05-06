# bb-grafana-fork

binarybeachio fork of [grafana/grafana](https://github.com/grafana/grafana).

**Status:** v11.2.0-mine.2 — first source patch on top of upstream `grafana/grafana-oss:11.2.0`: the `_bb_edge_sub` marker-cookie middleware required by the platform's per-app edge-identity validation convention. Prior tag `v11.2.0-mine.1` was a vanilla retag of upstream (bit-for-bit equivalent); see Tag history at the bottom of this file. The fork repo + image-on-Forgejo-registry continue to satisfy AGPL §13 source-availability for the `analytics.binarybeach.io` deployment.

**License:** AGPLv3 (Grafana OSS relicensed Apache-2.0 → AGPLv3 in v10.3, April 2024). Per `feedback_default_private_fork_visibility` in the binarybeachio repo: AGPL forks default **public** on both Forgejo and the GitHub mirror.

## Upstream

- Repo: https://github.com/grafana/grafana
- Default branch: `main`
- Currently integrated tag: `v11.2.0`

## Why the fork exists today

1. **AGPL §13 source-availability** — running a modified version on a network requires offering the source. Even though no patches exist at mine.1, the convention is: any fork repo that backs a Forgejo-registry-hosted image is published source-side from day one.
2. **Image-pin convention** — every binarybeachio runtime image is pinned to `git.binarybeach.io/binarybeach/<image>:v<X.Y.Z>-mine.<n>`, so deploys never depend on Docker Hub being reachable. mine.1 is the tag where the registry path goes live.
3. **Patch-ready foundation** — when source patches do land (e.g., `_bb_edge_sub` marker-cookie middleware per `docs/conventions/per-app-edge-identity-validation.md` in the binarybeachio repo, or login-page logo embed), the branch structure is already in place.

## What's customized

| Path | Change | Lines | Conflict-risk on upstream merge |
|------|--------|-------|---------------------------------|
| `BINARYBEACHIO.md` | This file | ~140 | None — net-new file |
| `pkg/middleware/bb_edge_identity.go` | NEW. Per-app edge-identity validation middleware. Reads `X-Auth-Request-User` header (set by oauth2-proxy at the platform edge) and `_bb_edge_sub` cookie (set at OIDC callback success). Three branches: header absent → pass through; cookie absent → lazy-populate; mismatch → revoke local `user_auth_token` row, clear `grafana_session` + `grafana_session_expiry` cookies, redirect to `/login` (which Traefik's `grafana-signin-redirect` middleware routes through the platform bridge for a fresh OIDC dance against the new edge identity). Skip-paths: `/login*`, `/logout`, `/public/*`, `/avatar/*`, `/api/live/*`. Per `docs/conventions/per-app-edge-identity-validation.md` in the binarybeachio repo. | ~135 | None — net-new file |
| `pkg/api/http_server.go` | Register `BbEdgeIdentity` in the global middleware chain immediately after `ContextHandler.Middleware` (so `c.SignedInUser` / `c.UserToken` are populated) and before `OrgRedirect`. One `m.Use(...)` line + a doc-comment block. | ~10 | Low — additive insertion at a stable hand-off point in the chain |
| `pkg/api/login_oauth.go` | At OIDC callback success (after `metrics.MApiLoginOAuth.Inc()`, before `authn.HandleLoginRedirect`), set the `_bb_edge_sub` cookie to the value of `X-Auth-Request-User`. Inert when the request didn't flow through oauth2-proxy. Also adds `pkg/middleware` import for the constant names. | ~12 | Low — additive at a single stable insertion point in `OAuthLogin` |
| `Dockerfile` | Pin `bingo` to `v0.9.0` instead of `@latest` in the go-builder stage (line 68). Upstream's `go install github.com/bwplotka/bingo@latest` worked when Grafana 11.2.0 shipped (Sept 2024) but bingo subsequently bumped to v0.10.0 which requires Go ≥ 1.24.9; Grafana's Dockerfile pins `golang:1.22.4-alpine`, so the build fails today. v0.9.0 is the last release before the Go floor bump. **Build-system patch only**, not a runtime patch — the running Grafana binary is unaffected. Becomes obsolete when upstream bumps GO_IMAGE past 1.24.9 OR pins bingo themselves. | ~1 | None — single-line version pin; refresh-from-upstream will re-introduce `@latest` and need this re-applied. |

## Required runtime config

All env-driven; the binarybeachio runtime config lives at `infrastructure/grafana/.env` in the operator's infra repo, layered on top of `_shared/.env.bb-admin` + `_shared/.env.zitadel-grafana` + `_shared/.env.r2-grafana`.

Key envs:
- `GF_SERVER_ROOT_URL=https://analytics.binarybeach.io` — must match Zitadel redirect URI
- `GF_AUTH_GENERIC_OAUTH_NAME=BinaryBeach.io` — SSO button label per the cross-fork convention
- `GF_AUTH_GENERIC_OAUTH_AUTO_LOGIN=true` + `GF_AUTH_DISABLE_LOGIN_FORM=true` — SSO-only posture
- `GF_AUTH_GENERIC_OAUTH_USE_REFRESH_TOKEN=true` + `GF_AUTH_GENERIC_OAUTH_USE_PKCE=true` — centralized-revocation alignment (defaults are false upstream)
- `GF_AUTH_GENERIC_OAUTH_LOGIN_PROMPT=select_account` — replaces the listmonk-style `auth_url_params` patch (verified via upstream docs)
- `GF_EXTERNAL_IMAGE_STORAGE_*` — R2 image storage for alert screenshots

Break-glass (when SSO is broken or wrong identity locked in):
```
https://analytics.binarybeach.io/login?disableAutoLogin=true
```
+ bb-admin from `_shared/.env.bb-admin` (the documented Grafana break-glass URL param).

## Tag history

| Tag | Status | What changed |
|---|---|---|
| `v11.2.0-mine.1` | superseded | Initial fork tag. Vanilla retag of upstream `grafana/grafana-oss:11.2.0` — image was `docker pull grafana/grafana-oss:11.2.0 && docker tag && docker push` to the Forgejo registry. Source repo carried only `BINARYBEACHIO.md` (no upstream tree, no patches). The Forgejo repo's git history was first populated for real at mine.2 — upstream `v11.2.0` rebased under the existing 2 BINARYBEACHIO.md commits. |
| `v11.2.0-mine.2` | active | Per-app edge-identity validation per `docs/conventions/per-app-edge-identity-validation.md` in the binarybeachio repo. **Why**: when a user signs out at the platform edge via `bridge.binarybeach.io/logout` and signs back in as a different Zitadel identity, Grafana's `grafana_session` cookie survives the swap and short-circuits Grafana's OIDC dance — leaving the previous identity's data fully visible and interactive. Reproduced 2026-05-05 against `analytics.binarybeach.io` immediately after the T2.x identity-flip deploy: signed in as user A, signed out, signed in as user B, returned to Grafana → still saw user A; the grafana DB confirmed user B was never JIT'd because the `grafana_session` cookie short-circuited. **The patch** (3 files, ~157 lines net): (1) NEW `pkg/middleware/bb_edge_identity.go` — middleware comparing `X-Auth-Request-User` header to `_bb_edge_sub` cookie on every authenticated request; mismatch → `authTokenService.RevokeToken(ctx, userToken, false)` + `authn.DeleteSessionCookie` + clear `_bb_edge_sub` + 302 to `/login` (or 401 JSON for `/api/*`). (2) `pkg/api/http_server.go` — registers the middleware in the global chain after `ContextHandler.Middleware` so `c.SignedInUser` / `c.UserToken` are populated. (3) `pkg/api/login_oauth.go` — at OAuth callback success, sets the `_bb_edge_sub` cookie to the value of `X-Auth-Request-User` so subsequent requests are guarded. **Why this layer is needed despite oauth2-proxy revoking on Zitadel deactivation**: oauth2-proxy's revoke-on-refresh-fail covers the deactivation case (request never reaches Grafana), but **active identity-switching** carries a *valid* edge cookie for a *different* identity than the existing app session. The marker cookie is the only way to detect that mismatch from inside the app process. The companion sign-out half (Grafana's user-menu → `bridge.binarybeach.io/logout`) was already wired in mine.1 via the native `GF_AUTH_SIGNOUT_REDIRECT_URL` env knob — no fork patch needed for that half. |

## Refresh from upstream

Standard binarybeachio Path B refresh (per `docs/architecture/customizations.md` in the binarybeachio repo):

```bash
git fetch upstream
git checkout upstream
git reset --hard upstream/v<NEW.X.Y>
git push origin upstream
git checkout main
git rebase upstream    # apply our patches on top of new upstream tag
git push origin main
```

Then rebuild the image (vanilla retag while no source patches; full build once patches land):

```bash
docker pull grafana/grafana-oss:v<NEW.X.Y>
docker tag grafana/grafana-oss:v<NEW.X.Y> git.binarybeach.io/binarybeach/bb-grafana-fork:v<NEW.X.Y>-mine.1
docker tag grafana/grafana-oss:v<NEW.X.Y> git.binarybeach.io/binarybeach/bb-grafana-fork:latest
docker push git.binarybeach.io/binarybeach/bb-grafana-fork:v<NEW.X.Y>-mine.1
docker push git.binarybeach.io/binarybeach/bb-grafana-fork:latest
```

Then bump `GRAFANA_IMAGE` in the operator infra `.env` and re-run `bootstrap-grafana.py`.

## Build

Today: vanilla retag — no `docker build` needed. Once source patches land, build from this repo's root with the upstream `Dockerfile`:

```bash
docker build -t git.binarybeach.io/binarybeach/bb-grafana-fork:v<X.Y.Z>-mine.<n> .
```

## License compliance

- This repo is **public** on both Forgejo (`git.binarybeach.io/binarybeach/bb-grafana-fork`) and the GitHub mirror (`github.com/binaryBeachLLC/bb-grafana-fork`) per AGPL §13 source-availability.
- Future deployment will surface a footer link to this repo (TODO when first customer-facing tenant onboards — AGPL §13 "prominent" notice).

## Test plan (when source patches land)

Until patches exist, the test plan is "bb-admin can sign in via SSO at analytics.binarybeach.io and reach a dashboard" — covered by the operator's bootstrap end-to-end verification.
