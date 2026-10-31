package staff

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	staffeeapi "github.com/getsphericon/sphericon/gen/staffee"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/narrow"
	"github.com/getsphericon/sphericon/internal/signupsurvey"
)

// SignupSurveyCounts is the Sign-up survey answers counted per question, read through
// internal/signupsurvey (the answers belong to Users, so this package holds no query for them).
// ErrNotLicensed without the `staff` license.
func (m *Module) SignupSurveyCounts(ctx context.Context) (signupsurvey.Counts, error) {
	if err := m.licensed(); err != nil {
		return signupsurvey.Counts{}, err
	}
	return m.survey.Counts(ctx)
}

// SignupSurveyResponse is one User's answers; ent.IsNotFound when the User is unknown or has not
// answered. ErrNotLicensed without the `staff` license.
func (m *Module) SignupSurveyResponse(ctx context.Context, userID int64) (*ent.SignupSurveyResponse, error) {
	if err := m.licensed(); err != nil {
		return nil, err
	}
	return m.survey.ForUser(ctx, userID)
}

// StaffSignupSurveyCounts is the Sign-up survey answers counted per question. The aggregate has
// no single source row, so it is built here from the counts.
func (h *Handlers) StaffSignupSurveyCounts(ctx context.Context) (staffeeapi.StaffSignupSurveyCountsRes, error) {
	c, err := h.module.SignupSurveyCounts(ctx)
	if err != nil {
		return nil, err
	}
	return &staffeeapi.StaffSignupSurveyCounts{
		Responses: narrow.Int32(c.Responses),
		Role: choiceCounts(c.Role, func(choice string, n int32) staffeeapi.StaffSignupRoleCount {
			return staffeeapi.StaffSignupRoleCount{Choice: staffeeapi.StaffSignupRole(choice), Count: n}
		}),
		RoleOtherTexts: otherTexts(c.Role),
		CompanySize: choiceCounts(c.CompanySize, func(choice string, n int32) staffeeapi.StaffSignupCompanySizeCount {
			return staffeeapi.StaffSignupCompanySizeCount{Choice: staffeeapi.StaffSignupCompanySize(choice), Count: n}
		}),
		CompanySizeOtherTexts: otherTexts(c.CompanySize),
		MainGoal: choiceCounts(c.MainGoal, func(choice string, n int32) staffeeapi.StaffSignupGoalCount {
			return staffeeapi.StaffSignupGoalCount{Choice: staffeeapi.StaffSignupGoal(choice), Count: n}
		}),
		MainGoalOtherTexts: otherTexts(c.MainGoal),
		HeardFrom: choiceCounts(c.HeardFrom, func(choice string, n int32) staffeeapi.StaffSignupSourceCount {
			return staffeeapi.StaffSignupSourceCount{Choice: staffeeapi.StaffSignupSource(choice), Count: n}
		}),
		HeardFromOtherTexts: otherTexts(c.HeardFrom),
	}, nil
}

func choiceCounts[T any](t signupsurvey.Tally, build func(choice string, count int32) T) []T {
	out := make([]T, len(t.Choices))
	for i, c := range t.Choices {
		out[i] = build(c.Choice, narrow.Int32(c.Count))
	}
	return out
}

func otherTexts(t signupsurvey.Tally) []staffeeapi.StaffSignupOtherTextCount {
	out := make([]staffeeapi.StaffSignupOtherTextCount, len(t.Others))
	for i, o := range t.Others {
		out[i] = staffeeapi.StaffSignupOtherTextCount{Text: o.Text, Count: narrow.Int32(o.Count)}
	}
	return out
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
