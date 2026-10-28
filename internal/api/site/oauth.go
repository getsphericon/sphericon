package site

import (
	"context"
	"errors"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/samber/lo"
	"net/http"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/oauthserver"
)

// SiteOAuthDescribe validates an OAuth authorization request for the consent
// screen (the client is untrusted: only its registered redirect URIs are honored).
func (h *Handlers) SiteOAuthDescribe(ctx context.Context, params siteapi.SiteOAuthDescribeParams) (siteapi.SiteOAuthDescribeRes, error) {
	req, err := h.oauth.Describe(ctx, params.ClientId, params.RedirectUri, params.Scope.Or(""))
	switch {
	case errors.Is(err, oauthserver.ErrUnknownClient):
		v := siteapi.SiteOAuthDescribeNotFound(problem(http.StatusNotFound, err.Error()))
		return &v, nil
	case errors.Is(err, oauthserver.ErrInvalidRequest):
		v := siteapi.SiteOAuthDescribeBadRequest(problem(http.StatusBadRequest, err.Error()))
		return &v, nil
	case err != nil:
		return nil, err
	}
	out := &siteapi.SiteOAuthAuthorizationRequest{
		ClientName:    req.ClientName,
		RedirectUri:   req.RedirectURI,
		LocalRedirect: req.LocalRedirect,
		Scopes:        lo.Map(req.Scopes, func(s string, _ int) siteapi.GrantableScope { return siteapi.GrantableScope(s) }),
		SendScopes:    lo.Map(req.SendScopes, func(s string, _ int) siteapi.SendScope { return siteapi.SendScope(s) }),
		SetupScopes:   lo.Map(req.SetupScopes, func(s string, _ int) siteapi.SetupScope { return siteapi.SetupScope(s) }),
	}
	if req.ClientHost != "" {
		out.ClientHost = siteapi.NewOptString(req.ClientHost)
	}
	return out, nil
}

// SiteOAuthDecide records the signed-in user's consent decision. The token the
// client later receives belongs to the chosen workspace, which the user must be
// a member of with a role that may manage API tokens: consent mints one.
func (h *Handlers) SiteOAuthDecide(ctx context.Context, req *siteapi.SiteOAuthDecisionInput) (siteapi.SiteOAuthDecideRes, error) {
	s, role, err := h.scopedWithRoleFor(ctx, req.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteOAuthDecideNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	if !accounts.CanManageTokens(role) {
		v := siteapi.SiteOAuthDecideForbidden(problem(http.StatusForbidden, "only owners and admins can connect an application"))
		return &v, nil
	}

	target, err := h.oauth.Decide(ctx, oauthserver.Decision{
		ClientID:      req.ClientId,
		RedirectURI:   req.RedirectUri,
		State:         req.State.Or(""),
		CodeChallenge: req.CodeChallenge,
		Scope:         req.Scope.Or(""),
		WorkspaceID:   s.WorkspaceID(),
		Approve:       req.Approve,
		AllowSend:     req.AllowSend.Or(false),
		AllowSetup:    req.AllowSetup.Or(false),
	})
	switch {
	case errors.Is(err, oauthserver.ErrUnknownClient):
		v := siteapi.SiteOAuthDecideNotFound(problem(http.StatusNotFound, err.Error()))
		return &v, nil
	case errors.Is(err, oauthserver.ErrInvalidRequest):
		v := siteapi.SiteOAuthDecideBadRequest(problem(http.StatusBadRequest, err.Error()))
		return &v, nil
	case err != nil:
		return nil, err
	}
	return &siteapi.SiteOAuthDecisionResult{RedirectUrl: target}, nil
}
