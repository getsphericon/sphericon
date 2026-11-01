package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/api/auth"
)

// SiteSignupSurveySubmit stores the authenticated User's answers to the Sign-up survey and
// clears the "survey due" marker. The survey belongs to the User, so no Workspace is read.
func (h *Handlers) SiteSignupSurveySubmit(ctx context.Context, req *siteapi.SiteSignupSurveyInput) (siteapi.SiteSignupSurveySubmitRes, error) {
	a := auth.GetSiteAuth(ctx)
	if a == nil {
		return nil, auth.ErrUnauthorized
	}
	if err := h.signupSurvey.Submit(ctx, a.UserID, inputs.SignupSurvey(req)); err != nil {
		return nil, err
	}
	return &siteapi.SiteSignupSurveySubmitNoContent{}, nil
}
