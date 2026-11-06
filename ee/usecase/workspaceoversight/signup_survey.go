package workspaceoversight

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/identity/signupsurveyresponses"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// ErrSignupSurveyResponseNotFound is a User who is unknown or has not answered the Sign-up survey.
var ErrSignupSurveyResponseNotFound = validation.NotFound("staff: no sign-up survey answers for this user", i18n.ErrorsSignupSurveyResponseNotFound)

// SignupSurveyCounts is the Sign-up survey answers counted per question, read through
// internal/domain/identity/signupsurveyresponses (the answers belong to Users, so this package holds no query for them).
// ErrNotLicensed without the `staff` license.
func (m *UseCase) SignupSurveyCounts(ctx context.Context) (signupsurveyresponses.Counts, error) {
	if err := m.licensed(); err != nil {
		return signupsurveyresponses.Counts{}, err
	}
	return m.survey.Counts(ctx)
}

// SignupSurveyResponse is one User's answers; ErrSignupSurveyResponseNotFound when the User is
// unknown or has not answered. ErrNotLicensed without the `staff` license.
func (m *UseCase) SignupSurveyResponse(ctx context.Context, userID int64) (*ent.SignupSurveyResponse, error) {
	if err := m.licensed(); err != nil {
		return nil, err
	}
	row, err := m.survey.ForUser(ctx, userID)
	if ent.IsNotFound(err) {
		return nil, ErrSignupSurveyResponseNotFound
	}
	return row, err
}
