package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/api/auth"
)

// SiteOptionalConsentsList is the account settings' optional consents (ADR 0062): each one in
// effect with the authenticated User's current answer. They belong to the User, so no Workspace is
// read.
func (h *Handlers) SiteOptionalConsentsList(ctx context.Context) (*siteapi.SiteOptionalConsentList, error) {
	a := auth.GetSiteAuth(ctx)
	if a == nil {
		return nil, auth.ErrUnauthorized
	}
	consents, err := h.consentRecords.OptionalConsents(ctx, a.UserID)
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteOptionalConsentList{Items: mapper.OptionalConsentsToSite(consents)}, nil
}

// SiteOptionalConsentsSet gives or withdraws one of the authenticated User's optional consents.
func (h *Handlers) SiteOptionalConsentsSet(ctx context.Context, req *siteapi.SiteOptionalConsentChange, params siteapi.SiteOptionalConsentsSetParams) (siteapi.SiteOptionalConsentsSetRes, error) {
	a := auth.GetSiteAuth(ctx)
	if a == nil {
		return nil, auth.ErrUnauthorized
	}
	consent, err := h.consentRecords.SetOptionalConsent(ctx, a.UserID, string(params.Key), string(req.Version), req.Accepted)
	if err != nil {
		return nil, err
	}
	return new(mapper.OptionalConsentToSite(consent)), nil
}
