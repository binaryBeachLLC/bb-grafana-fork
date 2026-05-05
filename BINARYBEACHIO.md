# bb-grafana-fork

binarybeachio fork of [grafana/grafana](https://github.com/grafana/grafana).

**Status:** v11.2.0-mine.1 — vanilla retag of upstream `grafana/grafana-oss:11.2.0`. **Zero source-level patches at this tag.** The fork repo + image-on-Forgejo-registry exist for AGPL-source-archive + the git.binarybeach.io image-pin convention. Mirrors `bb-bulwark-fork` mine.1's same posture.

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

| Path | Status | Conflict-risk on upstream merge |
|------|--------|---------------------------------|
| (none yet) | mine.1 is bit-for-bit equivalent to upstream `grafana/grafana-oss:11.2.0` | n/a |

When the first source patch lands:
- New file under `pkg/middleware/bb_edge_identity.go` (additive, MIT/AGPL-compatible)
- One-line registration in `pkg/api/api.go` middleware chain
- Cookie set in `pkg/login/social/connectors/generic_oauth_provider.go` after callback completes

Skip-paths for the marker-cookie middleware: `/api/health`, `/login/generic_oauth`, `/public/*`, `/avatar/*`.

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
