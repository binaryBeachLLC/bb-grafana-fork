// Copyright (c) 2026-present binarybeach, LLC. All Rights Reserved.
// See LICENSE for license information.
//
// binarybeachio: per-app edge-identity validation.
//
// When Grafana sits behind oauth2-proxy at the .binarybeach.io platform
// edge, the edge cookie (_bb_oauth2) and Grafana's own session cookie
// (grafana_session) are independent. Signing out via
// bridge.binarybeach.io/logout clears the edge cookie + Zitadel session
// but leaves grafana_session intact, so signing back in as a different
// Zitadel identity continues to render the previous identity's data
// (the existing grafana_session short-circuits Grafana's OIDC dance
// before any new claims can be evaluated).
//
// This middleware closes that gap. The OAuth callback handler in
// pkg/api/login_oauth.go pins a marker cookie (_bb_edge_sub) to the
// X-Auth-Request-User header value at the moment Grafana mints its
// session; on every subsequent authenticated request, this middleware
// compares the cookie to the header. Mismatch = edge identity has
// been swapped under us → revoke the underlying user_auth_token row,
// clear grafana_session + grafana_session_expiry cookies, redirect
// to /login (which Traefik's bridge regex routes through the per-tenant
// isEmailAllowed gate for a fresh OIDC dance against the new edge identity).
//
// Per docs/conventions/per-app-edge-identity-validation.md (in
// binarybeachio repo). Original bug repro: 2026-05-04 in Mattermost;
// reproduced 2026-05-05 in Grafana immediately after the T2.x identity
// flip with the existing grafana_session cookie short-circuiting the
// new identity's OIDC dance entirely (the grafana DB confirmed only the
// pre-switch user row; user B was never JIT'd).
//
// Three branches:
//   - Header absent: pass through. Either the request didn't flow
//     through oauth2-proxy (mobile, dev, internal tooling) or the
//     deployment isn't behind the platform edge. Don't gate.
//   - Header present, cookie absent: lazy-populate the cookie. Avoids
//     forcing every existing session to re-OIDC immediately on rollout.
//   - Header present, cookie present, mismatch: revoke + redirect.

package middleware

import (
	"strings"

	"github.com/grafana/grafana/pkg/infra/log"
	"github.com/grafana/grafana/pkg/middleware/cookies"
	"github.com/grafana/grafana/pkg/services/auth"
	"github.com/grafana/grafana/pkg/services/authn"
	contextmodel "github.com/grafana/grafana/pkg/services/contexthandler/model"
	"github.com/grafana/grafana/pkg/setting"
	"github.com/grafana/grafana/pkg/web"
)

const (
	// BbEdgeSubCookieName is the marker cookie set at OIDC callback
	// success and read by the BbEdgeIdentity middleware.
	BbEdgeSubCookieName = "_bb_edge_sub"

	// BbEdgeUserHeaderName is the header oauth2-proxy injects with the
	// authenticated Zitadel `sub`. Setting `OAUTH2_PROXY_SET_XAUTHREQUEST=true`
	// in infrastructure/oauth2-proxy/.env is what makes this header
	// available on every gated request.
	BbEdgeUserHeaderName = "X-Auth-Request-User"
)

var bbEdgeIdentityLogger = log.New("bb_edge_identity")

// BbEdgeIdentity returns a middleware that validates the per-app
// session against the oauth2-proxy edge identity. See the package
// doc-comment for the full behavior contract.
//
// Wired in pkg/api/http_server.go after ContextHandler.Middleware so
// c.SignedInUser / c.UserToken / c.IsSignedIn are populated. Skip-paths
// are enumerated in isBbEdgeIdentitySkipPath below.
func BbEdgeIdentity(cfg *setting.Cfg, authTokenService auth.UserTokenService) web.Handler {
	return func(c *contextmodel.ReqContext) {
		edgeSub := c.Req.Header.Get(BbEdgeUserHeaderName)
		if edgeSub == "" {
			return
		}

		if isBbEdgeIdentitySkipPath(c.Req.URL.Path) {
			return
		}

		// No active app session — nothing to invalidate. The /login
		// dance will set the marker cookie on success, so subsequent
		// requests are guarded.
		if !c.IsSignedIn || c.UserToken == nil {
			return
		}

		var cookieSub string
		if existing, err := c.Req.Cookie(BbEdgeSubCookieName); err == nil && existing != nil {
			cookieSub = existing.Value
		}

		if cookieSub == "" {
			// Legacy session pre-dating this patch, or app session
			// minted outside an OIDC callback. Lazy-populate so
			// subsequent requests are guarded.
			cookies.WriteCookie(c.Resp, BbEdgeSubCookieName, edgeSub, 0, nil)
			return
		}

		if cookieSub == edgeSub {
			return
		}

		// Mismatch — edge identity swapped under us. Revoke the local
		// token (severs the rotation chain in user_auth_token), clear
		// the Grafana session cookies, then redirect to /login. The
		// bridge regex in Traefik routes /login through the per-tenant
		// allowlist; Grafana's auto_login then fires a fresh OIDC dance
		// against the new edge identity.
		if err := authTokenService.RevokeToken(c.Req.Context(), c.UserToken, false); err != nil {
			bbEdgeIdentityLogger.Warn("revoke token failed",
				"err", err,
				"user_id", c.SignedInUser.GetID())
		}
		authn.DeleteSessionCookie(c.Resp, cfg)
		cookies.DeleteCookie(c.Resp, BbEdgeSubCookieName, nil)

		bbEdgeIdentityLogger.Info("edge identity changed; invalidated app session",
			"path", c.Req.URL.Path,
			"prior_user_id", c.SignedInUser.GetID())

		if c.IsApiRequest() {
			c.JsonApiErr(401, "edge identity changed", nil)
			return
		}
		c.Redirect(cfg.AppSubURL + "/login")
	}
}

// isBbEdgeIdentitySkipPath enumerates routes that bypass the marker
// check. Per docs/conventions/per-app-edge-identity-validation.md
// §"Routes that should NOT be middleware-gated":
//   - /login + sub-paths (OIDC callback /login/generic_oauth, the
//     local form, the disableAutoLogin break-glass): the marker is
//     being set on this very response, so checking would always
//     lazy-populate, which is wasteful.
//   - /logout: the existing logout flow runs its own cookie teardown;
//     let it run instead of short-circuiting.
//   - /public/*, /avatar/*: static assets, no app session involvement.
//   - /api/live/*: websocket upgrade endpoint; the cookie machinery
//     interferes with hijacked connections and a stale session here
//     is harmless (the bearer-token revalidation on the underlying
//     /api/* fetch covers it).
//
// The healthcheck endpoints (/api/health, /healthz, /metrics) are
// registered BEFORE ContextHandler.Middleware in pkg/api/http_server.go,
// so they don't reach this middleware at all. No explicit skip needed.
func isBbEdgeIdentitySkipPath(p string) bool {
	if strings.HasPrefix(p, "/login") {
		return true
	}
	if p == "/logout" {
		return true
	}
	if strings.HasPrefix(p, "/public/") || strings.HasPrefix(p, "/avatar/") {
		return true
	}
	if strings.HasPrefix(p, "/api/live/") {
		return true
	}
	return false
}
