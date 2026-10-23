package staff

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	staffapi "github.com/getsphericon/sphericon/gen/staff"
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
func (h *Handlers) StaffSignupSurveyCounts(ctx context.Context) (staffapi.StaffSignupSurveyCountsRes, error) {
	c, err := h.module.SignupSurveyCounts(ctx)
	if err != nil {
		return nil, err
	}
	return &staffapi.StaffSignupSurveyCounts{
		Responses: narrow.Int32(c.Responses),
		Role: choiceCounts(c.Role, func(choice string, n int32) staffapi.StaffSignupRoleCount {
			return staffapi.StaffSignupRoleCount{Choice: staffapi.StaffSignupRole(choice), Count: n}
		}),
		RoleOtherTexts: otherTexts(c.Role),
		CompanySize: choiceCounts(c.CompanySize, func(choice string, n int32) staffapi.StaffSignupCompanySizeCount {
			return staffapi.StaffSignupCompanySizeCount{Choice: staffapi.StaffSignupCompanySize(choice), Count: n}
		}),
		CompanySizeOtherTexts: otherTexts(c.CompanySize),
		MainGoal: choiceCounts(c.MainGoal, func(choice string, n int32) staffapi.StaffSignupGoalCount {
			return staffapi.StaffSignupGoalCount{Choice: staffapi.StaffSignupGoal(choice), Count: n}
		}),
		MainGoalOtherTexts: otherTexts(c.MainGoal),
		HeardFrom: choiceCounts(c.HeardFrom, func(choice string, n int32) staffapi.StaffSignupSourceCount {
			return staffapi.StaffSignupSourceCount{Choice: staffapi.StaffSignupSource(choice), Count: n}
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

func otherTexts(t signupsurvey.Tally) []staffapi.StaffSignupOtherTextCount {
	out := make([]staffapi.StaffSignupOtherTextCount, len(t.Others))
	for i, o := range t.Others {
		out[i] = staffapi.StaffSignupOtherTextCount{Text: o.Text, Count: narrow.Int32(o.Count)}
	}
	return out
}

// StaffSignupSurveyGet is one User's Sign-up survey answers.
func (h *Handlers) StaffSignupSurveyGet(ctx context.Context, params staffapi.StaffSignupSurveyGetParams) (staffapi.StaffSignupSurveyGetRes, error) {
	row, err := h.module.SignupSurveyResponse(ctx, int64(params.UserId))
	if ent.IsNotFound(err) {
		p := notFoundProblem()
		p.Detail = staffapi.NewOptString(i18n.T("errors.signup_survey_response_not_found", nil))
		v := staffapi.StaffSignupSurveyGetNotFound(p)
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	res := staffMapper.SignupSurveyResponseToResource(row)
	return &res, nil
}
