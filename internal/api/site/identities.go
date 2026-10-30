package site

import (
	"context"
	"errors"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/signin"
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
	rows, err := h.accounts.Identities(ctx, userID)
	if err != nil {
		return nil, err
	}
	items := make([]siteapi.SiteLinkedIdentity, len(rows))
	for i, row := range rows {
		items[i] = siteapi.SiteLinkedIdentity{
			ID:          siteapi.EntityId(row.ID),
			Provider:    row.Provider,
			DisplayName: signin.DisplayName(row.Provider),
			CreatedAt:   siteapi.Timestamp(row.CreatedAt),
		}
	}
	return &siteapi.SiteLinkedIdentities{Items: items}, nil
}

// SiteIdentitiesLink begins linking a provider account to the session User: the same flow as
// a sign-in, but the signed flow cookie also names the User the callback adds the subject to.
func (h *Handlers) SiteIdentitiesLink(ctx context.Context, params siteapi.SiteIdentitiesLinkParams) (siteapi.SiteIdentitiesLinkRes, error) {
	userID, err := sessionUserID(ctx)
	if err != nil {
		return nil, err
	}
	authURL, flow, err := h.signin.Start(params.Provider, signin.SafeReturnTo(params.ReturnTo.Or("")))
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
		SetCookie: h.sessions.SigninFlow(sealed, signin.FlowTTL).String(),
	}, nil
}

// SiteIdentitiesUnlink removes a Linked identity of the session User, unless it is the last
// way to sign in, ends the other sessions and reissues the acting one.
func (h *Handlers) SiteIdentitiesUnlink(ctx context.Context, params siteapi.SiteIdentitiesUnlinkParams) (siteapi.SiteIdentitiesUnlinkRes, error) {
	userID, err := sessionUserID(ctx)
	if err != nil {
		return nil, err
	}
	u, err := h.accounts.UnlinkIdentity(ctx, userID, int64(params.ID))
	if err != nil {
		return nil, err
	}
	cookie, err := h.sessions.Issue(u, currentSession(ctx))
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteIdentitiesUnlinkNoContent{SetCookie: cookie.String()}, nil
}

// linkProvider finishes a link flow: the callback of the browser that is signed in as the
// User the flow names adds the provider's subject to that User, ends the other sessions and
// reissues this one (a new identity keeps the session's provider login time), and lands on the page the flow began from. Anything else is a refusal
// that lands on the settings page with the Reason.
func (h *Handlers) linkProvider(ctx context.Context, flow signin.Flow, session string, profile signin.Profile) (*siteapi.SiteAuthSigninCallbackFound, error) {
	holds, err := h.holdsSession(ctx, session, flow.LinkUserID)
	if err != nil {
		return nil, err
	}
	if !holds {
		return h.signinRefused(flow, signin.ReasonInvalidState), nil
	}
	u, added, err := h.accounts.LinkIdentity(ctx, flow.LinkUserID, flow.Provider, profile.Subject)
	if errors.Is(err, accounts.ErrIdentityInUse) {
		return h.signinRefused(flow, signin.ReasonIdentityInUse), nil
	}
	if err != nil {
		return nil, err
	}
	// Signing in again with a provider account the User already had is a fresh provider
	// login. A provider account added now proves nothing about the person (a stolen session
	// could add its own), so the session keeps the login time it had.
	sess, _ := h.sessions.SessionOf(session)
	if !added {
		sess.ProviderLoginAt = h.now()
	}
	cookie, err := h.sessions.Issue(u, sess)
	if err != nil {
		return nil, err
	}
	return h.signinLanded(flow, cookie.String()), nil
}
