package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/identity/signinprovider"
)

// signinSettingsPath is the account page that lists the Linked identities: where a link flow
// lands, with the Reason in the query when it was refused. The SPA owns the route.
const signinSettingsPath = "/account/security"

// SiteIdentitiesList lists the session User's Linked identities (ADR 0048).
func (h *Handlers) SiteIdentitiesList(ctx context.Context) (*siteapi.SiteLinkedIdentities, error) {
	userID, err := sessionUserID(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := h.users.Identities(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteLinkedIdentities{Items: mapper.IdentitiesToResources(rows)}, nil
}

// SiteIdentitiesLink begins linking a provider account to the session User: the same flow as
// a sign-in, but the signed flow cookie also names the User the callback adds the subject to.
func (h *Handlers) SiteIdentitiesLink(ctx context.Context, params siteapi.SiteIdentitiesLinkParams) (*siteapi.SiteIdentitiesLinkFound, error) {
	userID, err := sessionUserID(ctx)
	if err != nil {
		return nil, err
	}
	authURL, flow, err := h.signin.Start(params.Provider, signinprovider.SafeReturnTo(params.ReturnTo.Or("")))
	if err != nil {
		return nil, err
	}
	flow.LinkUserID = userID
	sealed, err := h.signin.SealFlow(flow)
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteIdentitiesLinkFound{
		Location:  authURL,
		SetCookie: h.sessions.SigninFlow(sealed, signinprovider.FlowTTL).String(),
	}, nil
}

// SiteIdentitiesUnlink removes a Linked identity of the session User, unless it is the last
// way to sign in, ends the other sessions and reissues the acting one.
func (h *Handlers) SiteIdentitiesUnlink(ctx context.Context, params siteapi.SiteIdentitiesUnlinkParams) (*siteapi.SiteIdentitiesUnlinkNoContent, error) {
	userID, err := sessionUserID(ctx)
	if err != nil {
		return nil, err
	}
	u, err := h.users.UnlinkIdentity(ctx, userID, int64(params.ID))
	if err != nil {
		return nil, err
	}
	cookie, err := h.sessions.Issue(u, currentSession(ctx))
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteIdentitiesUnlinkNoContent{SetCookie: cookie.String()}, nil
}
