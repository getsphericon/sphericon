// Package signupsurvey is the Sign-up survey: four questions a User who signed up in public
// answers once, stored on the Instance and read only by a Staff member. Whether the Instance asks it
// (the saas Mode, which also needs the `staff` license) is decided once, by Enabled.
// A public sign-up marks the User as owing the survey (users.signup_survey_due_at, set by
// internal/accounts when Enabled is true); Submit stores the answers and clears the marker in
// one transaction.
//
// The package holds the raw *ent.Client: a SignupSurveyResponse belongs to a User, not to a
// Workspace, so no scoped wrapper exists for it, and the marker it clears is on the User row.
package signupsurvey

import (
	"context"
	"log/slog"
	"strings"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/signupsurveyresponse"
	"github.com/getsphericon/sphericon/ent/user"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/validation"
)

// ErrNotDue is the refusal of a submit by a User who does not owe the survey: it was answered
// already, or the User was never asked (the Instance has it off, or the User joined by an
// Invitation).
var ErrNotDue = validation.Conflict("signupsurvey: the user does not owe the survey", "errors.signup_survey_not_due")

// otherText is the refusal of an `other` text sent with a choice that is not `other` (blank
// false), or of an `other` choice whose text is blank, on field, the contract field that carried
// the text (`roleOther`).
func otherText(field string, blank bool) *validation.Error {
	key := "errors.signup_survey_other_text"
	if blank {
		key = "errors.signup_survey_other_blank"
	}
	return &validation.Error{Field: field, Message: i18n.T(key, nil)}
}

// Enabled is whether the Instance asks the survey: its Mode asks for it (saas) and the `staff`
// license is held, since only the Staff console reads the answers. A request without the license
// is logged once, here, and is not a boot error.
func Enabled(requested, licensed bool, log *slog.Logger) bool {
	if requested && !licensed {
		log.Warn("the Mode asks for the Sign-up survey but the staff license is missing: the survey stays off")
	}
	return requested && licensed
}

// Answers are the four answers of one submit. The `…Other` texts are set only with the `other`
// choice of their question; HeardFrom is nil when the optional question was skipped.
type Answers struct {
	Role             signupsurveyresponse.Role
	RoleOther        *string
	CompanySize      signupsurveyresponse.CompanySize
	CompanySizeOther *string
	MainGoal         signupsurveyresponse.MainGoal
	MainGoalOther    *string
	HeardFrom        *signupsurveyresponse.HeardFrom
	HeardFromOther   *string
}

// Module is the Sign-up survey of an Instance.
type Module struct {
	bus     *events.Bus
	enabled bool
}

// New builds the module; enabled is the result of Enabled.
func New(bus *events.Bus, enabled bool) *Module {
	return &Module{bus: bus, enabled: enabled}
}

// Enabled is whether a public sign-up marks its User as owing the survey.
func (m *Module) Enabled() bool { return m != nil && m.enabled }

// Submit stores the User's answers and clears the "survey due" marker in one transaction, so a
// response row exists exactly when the marker is gone. ErrNotDue when the Instance no longer asks
// the survey or the User does not owe it (the clear is a conditional write: of two racing submits one wins), and an
// *validation.Error on the field of an `other` text without its `other` choice; nothing is stored then.
func (m *Module) Submit(ctx context.Context, userID int64, a Answers) error {
	if !m.Enabled() {
		return ErrNotDue
	}
	if err := a.check(); err != nil {
		return err
	}
	return m.bus.WithinTx(ctx, func(tx *ent.Client, _ events.Publisher) error {
		cleared, err := tx.User.Update().
			Where(user.ID(userID), user.SignupSurveyDueAtNotNil()).
			ClearSignupSurveyDueAt().
			Save(ctx)
		if err != nil {
			return err
		}
		if cleared == 0 {
			return ErrNotDue
		}
		create := tx.SignupSurveyResponse.Create().
			SetUserID(userID).
			SetRole(a.Role).
			SetNillableRoleOtherText(a.RoleOther).
			SetCompanySize(a.CompanySize).
			SetNillableCompanySizeOtherText(a.CompanySizeOther).
			SetMainGoal(a.MainGoal).
			SetNillableMainGoalOtherText(a.MainGoalOther).
			SetNillableHeardFrom(a.HeardFrom).
			SetNillableHeardFromOtherText(a.HeardFromOther)
		return create.Exec(ctx)
	})
}

// check refuses an `other` text whose question did not choose `other`, and an `other` choice
// whose text is blank once trimmed. An `other` choice with no text at all is allowed.
func (a Answers) check() error {
	heardOther := a.HeardFrom != nil && *a.HeardFrom == signupsurveyresponse.HeardFromOther
	for _, q := range []struct {
		field string
		other bool
		text  *string
	}{
		{"roleOther", a.Role == signupsurveyresponse.RoleOther, a.RoleOther},
		{"companySizeOther", a.CompanySize == signupsurveyresponse.CompanySizeOther, a.CompanySizeOther},
		{"mainGoalOther", a.MainGoal == signupsurveyresponse.MainGoalOther, a.MainGoalOther},
		{"heardFromOther", heardOther, a.HeardFromOther},
	} {
		switch {
		case q.text != nil && !q.other:
			return otherText(q.field, false)
		case q.text != nil && strings.TrimSpace(*q.text) == "":
			return otherText(q.field, true)
		}
	}
	return nil
}
