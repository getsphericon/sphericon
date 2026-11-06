package users

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/user"
)

// Who owes the Sign-up survey is decided here and nowhere else in the package: a User owes it
// while users.signup_survey_due_at is set. A public sign-up (a password Sign-up or a
// Sign-in provider creating the User) sets the marker through surveyDueAt when the Instance asks
// the survey (the caller passes signupsurveyresponses.Module.Enabled); a User who is admitted by an
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

// ClearSurveyDueIn removes the marker of the User userID inside tx while it is set, the step of a
// submitted Sign-up survey (internal/domain/identity/signupsurveyresponses): of two racing submits one wins. It returns how
// many rows it changed, 0 when the User owes no survey.
func ClearSurveyDueIn(ctx context.Context, tx *ent.Client, userID int64) (int, error) {
	return tx.User.Update().
		Where(user.ID(userID), user.SignupSurveyDueAtNotNil()).
		ClearSignupSurveyDueAt().
		Save(ctx)
}

// SetFeedPosition gives the Users which selects the service feed position (ADR 0062): the write of
// internal/domain/identity/usertombstones's hook, which takes the position.
func SetFeedPosition(ctx context.Context, client *ent.Client, position int64, which predicate.User) error {
	return client.User.Update().Where(which).SetFeedPosition(position).Exec(ctx)
}
