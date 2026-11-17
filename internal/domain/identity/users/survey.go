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

// surveyDueAt is the marker a public sign-up sets: `at` when the Instance asks the Sign-up survey,
// nil when it does not.
func surveyDueAt(due bool, at time.Time) *time.Time {
	if !due {
		return nil
	}
	return &at
}

// clearSurveyDue removes the marker of a User who is admitted rather than signed up in public.
func clearSurveyDue(ctx context.Context, tx *ent.Client, userID int64) error {
	return tx.User.UpdateOneID(userID).ClearSignupSurveyDueAt().Exec(ctx)
}

// ClearSurveyDueIn removes the marker of the User userID inside tx while it is set, the step of a
// submitted Sign-up survey (internal/domain/identity/signupsurveyresponses): of two racing submits one wins. It returns how
// many rows it changed, 0 when the User owes no survey.
func (a *Module) ClearSurveyDueIn(ctx context.Context, tx *ent.Client, userID int64) (int, error) {
	return tx.User.Update().
		Where(user.ID(userID), user.SignupSurveyDueAtNotNil()).
		ClearSignupSurveyDueAt().
		Save(ctx)
}

// Feed is the users module's write of the service feed position of Users (ADR 0062), for
// internal/domain/identity/usertombstones's hook: an ent hook holds no module, so it builds this
// role, which needs no dependency, with NewFeed.
type Feed struct{}

// NewFeed builds the write of the service feed position of Users.
func NewFeed() *Feed { return &Feed{} }

// SetPosition gives the Users which selects the service feed position, on the client c of the
// hook's transaction.
func (*Feed) SetPosition(ctx context.Context, c *ent.Client, position int64, which predicate.User) error {
	return c.User.Update().Where(which).SetFeedPosition(position).Exec(ctx)
}
