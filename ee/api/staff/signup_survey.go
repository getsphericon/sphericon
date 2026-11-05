package staff

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	staffeeapi "github.com/getsphericon/sphericon/gen/staffee"
	"github.com/getsphericon/sphericon/internal/i18n"
)

// StaffSignupSurveyCounts is the Sign-up survey answers counted per question.
func (h *Handlers) StaffSignupSurveyCounts(ctx context.Context) (staffeeapi.StaffSignupSurveyCountsRes, error) {
	c, err := h.module.SignupSurveyCounts(ctx)
	if err != nil {
		return nil, err
	}
	res := staffMapper.SignupSurveyCountsToResource(c)
	return &res, nil
}

// StaffSignupSurveyGet is one User's Sign-up survey answers.
func (h *Handlers) StaffSignupSurveyGet(ctx context.Context, params staffeeapi.StaffSignupSurveyGetParams) (staffeeapi.StaffSignupSurveyGetRes, error) {
	row, err := h.module.SignupSurveyResponse(ctx, int64(params.UserId))
	if ent.IsNotFound(err) {
		p := notFoundProblem()
		p.Detail = staffeeapi.NewOptString(i18n.T(i18n.ErrorsSignupSurveyResponseNotFound, nil))
		v := staffeeapi.StaffSignupSurveyGetNotFound(p)
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	res := staffMapper.SignupSurveyResponseToResource(row)
	return &res, nil
}
