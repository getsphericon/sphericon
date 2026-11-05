package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/api/auth"
)

// SiteSignupSurveySubmit stores the authenticated User's answers to the Sign-up survey and
// clears the "survey due" marker. The survey belongs to the User, so no Workspace is read.
func (h *Handlers) SiteSignupSurveySubmit(ctx context.Context, req *siteapi.SiteSignupSurveyInput) error {
	a := auth.GetSiteAuth(ctx)
	if a == nil {
		return auth.ErrUnauthorized
	}
	return h.signupSurvey.Submit(ctx, a.UserID, inputs.SignupSurvey(req))
}
