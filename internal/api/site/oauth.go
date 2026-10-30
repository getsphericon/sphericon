package site

import (
	"context"

	"github.com/samber/lo"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/oauthserver"
)

// SiteOAuthDescribe validates an OAuth authorization request for the consent
// screen (the client is untrusted: only its registered redirect URIs are honored).
func (h *Handlers) SiteOAuthDescribe(ctx context.Context, params siteapi.SiteOAuthDescribeParams) (siteapi.SiteOAuthDescribeRes, error) {
	req, err := h.oauth.Describe(ctx, params.ClientId, params.RedirectUri, params.Scope.Or(""))
	if err != nil {
		return nil, err
	}
	out := &siteapi.SiteOAuthAuthorizationRequest{
		ClientName:    req.ClientName,
		RedirectUri:   req.RedirectURI,
		LocalRedirect: req.LocalRedirect,
		Scopes:        lo.Map(req.Scopes, func(s string, _ int) siteapi.GrantableScope { return siteapi.GrantableScope(s) }),
		SendScopes:    lo.Map(req.SendScopes, func(s string, _ int) siteapi.SendScope { return siteapi.SendScope(s) }),
		SetupScopes:   lo.Map(req.SetupScopes, func(s string, _ int) siteapi.SetupScope { return siteapi.SetupScope(s) }),
		RequiredPermissions: siteapi.SiteOAuthAuthorizationRequestRequiredPermissions(
			lo.MapValues(req.RequiredPermissions, func(p accounts.Permission, _ string) siteapi.Permission { return siteapi.Permission(p) })),
	}
	if req.ClientHost != "" {
		out.ClientHost = siteapi.NewOptString(req.ClientHost)
	}
	return out, nil
}

// SiteOAuthDecide records the signed-in user's consent decision. Approving creates a Connection
// tied to the User's Membership of the chosen Workspace (ADR 0061), so any member may approve,
// but only scopes their Role may grant: oauthserver.Decide refuses the rest with the Permission's
// refusal (403 permission_required). The Workspace is named in the body, not the path, so Access
// does not resolve it: the handler loads the Membership itself and admits it as Access admits
// the Membership of any operation of a Workspace.
func (h *Handlers) SiteOAuthDecide(ctx context.Context, req *siteapi.SiteOAuthDecisionInput) (siteapi.SiteOAuthDecideRes, error) {
	session := auth.GetSiteAuth(ctx)
	if session == nil {
		return nil, accounts.ErrWorkspaceNotFound
	}
	_, m, err := h.accounts.Scope(ctx, session.UserID, string(req.WorkspaceId))
	if err != nil {
		return nil, err
	}
	err = h.accounts.Admit(ctx, accounts.Admission{
		UserID:        session.UserID,
		Membership:    m,
		StrongSession: currentSession(ctx).Method.Strong(),
		At:            h.now(),
	})
	if err != nil {
		return nil, err
	}

	target, err := h.oauth.Decide(ctx, oauthserver.Decision{
		ClientID:      req.ClientId,
		RedirectURI:   req.RedirectUri,
		State:         req.State.Or(""),
		CodeChallenge: req.CodeChallenge,
		Scope:         req.Scope.Or(""),
		Membership:    m,
		Approve:       req.Approve,
		AllowSend:     req.AllowSend.Or(false),
		AllowSetup:    req.AllowSetup.Or(false),
	})
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteOAuthDecisionResult{RedirectUrl: target}, nil
}
