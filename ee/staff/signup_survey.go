package staff

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
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
