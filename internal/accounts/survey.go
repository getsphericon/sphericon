package accounts

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
)

// Who owes the Sign-up survey is decided here and nowhere else in the package: a User owes it
// while users.signup_survey_due_at is set. A public sign-up (a password Sign-up or a
// Sign-in provider creating the User) sets the marker through surveyDueAt when the Instance asks
// the survey (the caller passes signupsurvey.Module.Enabled); a User who is admitted by an
// Invitation or a Join domain is not a public sign-up and has it cleared by clearSurveyDue.

// surveyDueAt is the marker a public sign-up sets: now when the Instance asks the Sign-up survey,
// nil when it does not.
func surveyDueAt(due bool) *time.Time {
	if !due {
		return nil
	}
	now := time.Now()
	return &now
}

// clearSurveyDue removes the marker of a User who is admitted rather than signed up in public.
func clearSurveyDue(ctx context.Context, tx *ent.Client, userID int64) error {
	return tx.User.UpdateOneID(userID).ClearSignupSurveyDueAt().Exec(ctx)
}
