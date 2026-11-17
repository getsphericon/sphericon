package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/identity/signinprovider"
)

// signinSettingsPath is the account page that lists the Linked identities: where a link flow
// lands, with the Reason in the query when it was refused. The SPA owns the route.
const signinSettingsPath = "/account/security"

// SiteIdentitiesList lists the session User's Linked identities (ADR 0048), with what proves
// the User for a link.
func (h *Handlers) SiteIdentitiesList(ctx context.Context) (*siteapi.SiteLinkedIdentities, error) {
	u, err := sessionUser(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := h.users.Identities(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteLinkedIdentities{
		Items: mapper.IdentitiesToResources(rows), PasswordSet: u.PasswordHash != "", IdentityProofFresh: sessionProof(ctx, "").StandsInFor(u, h.now()),
	}, nil
}

// SiteIdentitiesLink begins linking a provider account to the session User once they prove their
// identity (the password, or a fresh provider login or Passkey check; signup.Module.StartLink):
// the same flow as a sign-in, but the signed flow cookie also names the User the callback adds
// the subject to. The browser is sent on to the provider's URL.
func (h *Handlers) SiteIdentitiesLink(ctx context.Context, req *siteapi.SiteIdentityLinkInput) (*siteapi.SiteIdentityLinkStartedHeaders, error) {
	u, proof, err := h.identityProof(ctx, req.CurrentPassword)
	if err != nil {
		return nil, err
	}
	in := inputs.IdentityLinkStart(req)
	in.Proof = proof
	started, err := h.signUp.StartLink(ctx, u, in)
	if err != nil {
		return nil, err
	}
	sealed, err := h.signin.SealFlow(started.Flow)
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteIdentityLinkStartedHeaders{
		SetCookie: h.sessions.SigninFlow(sealed, signinprovider.FlowTTL).String(),
		Response:  siteapi.SiteIdentityLinkStarted{RedirectUrl: started.AuthURL},
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
