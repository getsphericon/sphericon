package site

import (
	"context"
	"errors"
	"net/http"

	"github.com/getsphericon/sphericon/ent/signupsurveyresponse"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/signupsurvey"
)

// SiteSignupSurveySubmit stores the authenticated User's answers to the Sign-up survey and
// clears the "survey due" marker. The survey belongs to the User, so no Workspace is read.
func (h *Handlers) SiteSignupSurveySubmit(ctx context.Context, req *siteapi.SiteSignupSurveyInput) (siteapi.SiteSignupSurveySubmitRes, error) {
	a := auth.GetSiteAuth(ctx)
	if a == nil {
		return nil, auth.ErrUnauthorized
	}
	err := h.signupSurvey.Submit(ctx, a.UserID, answersOf(req))
	var otherText *signupsurvey.OtherTextError
	switch {
	case err == nil:
		return &siteapi.SiteSignupSurveySubmitNoContent{}, nil
	case errors.Is(err, signupsurvey.ErrNotDue):
		v := siteapi.SiteSignupSurveySubmitConflict(problem(http.StatusConflict, "the sign-up survey is not due"))
		return &v, nil
	case errors.As(err, &otherText):
		v := siteapi.SiteSignupSurveySubmitUnprocessableEntity(problemWithErrors(http.StatusUnprocessableEntity,
			otherText.Error(), map[string][]string{otherText.Field: {otherText.Error()}}))
		return &v, nil
	}
	return nil, err
}

// answersOf maps the request to the module's answers; every value was checked against the
// contract's enums when the request was decoded.
func answersOf(req *siteapi.SiteSignupSurveyInput) signupsurvey.Answers {
	heardFrom, heard := req.HeardFrom.Get()
	answers := signupsurvey.Answers{
		Role:             signupsurveyresponse.Role(req.Role),
		RoleOther:        optionalText(req.RoleOther),
		CompanySize:      signupsurveyresponse.CompanySize(req.CompanySize),
		CompanySizeOther: optionalText(req.CompanySizeOther),
		MainGoal:         signupsurveyresponse.MainGoal(req.MainGoal),
		MainGoalOther:    optionalText(req.MainGoalOther),
		HeardFromOther:   optionalText(req.HeardFromOther),
	}
	if heard {
		answers.HeardFrom = new(signupsurveyresponse.HeardFrom(heardFrom))
	}
	return answers
}

// optionalText is the text of an optional field, nil when it was not sent.
func optionalText(o siteapi.OptString) *string {
	v, ok := o.Get()
	if !ok {
		return nil
	}
	return &v
}
