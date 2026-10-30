package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
)

// SiteLegalDocumentsSignupItems is what the sign-up form asks (ADR 0062): the Sign-up items in
// effect, each with the label of its current version. Public.
func (h *Handlers) SiteLegalDocumentsSignupItems(ctx context.Context) (*siteapi.SiteSignupItemList, error) {
	docs, err := h.signupItems.InEffect(ctx)
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteSignupItemList{Items: mapper.SignupItemsToSite(docs)}, nil
}

// SiteLegalDocumentsGet is the current version of a Sign-up item's Legal document, for its public
// page. Public.
func (h *Handlers) SiteLegalDocumentsGet(ctx context.Context, params siteapi.SiteLegalDocumentsGetParams) (siteapi.SiteLegalDocumentsGetRes, error) {
	doc, err := h.signupItems.Document(ctx, string(params.Key))
	if err != nil {
		return nil, err
	}
	return new(mapper.LegalDocumentToSite(doc)), nil
}
