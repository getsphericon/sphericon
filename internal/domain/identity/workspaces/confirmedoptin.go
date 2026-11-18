package workspaces

import (
	"context"
	"strings"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/kernel/fieldname"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// ConfirmationEmail is the Workspace's confirmation email as it goes out (ADR 0013): the settings
// it stores, each unset one read as the Instance's default naming the Workspace. The frame around
// them (the Confirm button, the line that nothing happens if the person did not ask, the postal
// address) is fixed by sphericon.
type ConfirmationEmail struct {
	// FromEmail is "" while the email is not set up.
	FromEmail string
	FromName  string
	Subject   string
	Intro     string
}

// ConfirmationEmailDefaults are the texts the confirmation email goes out with where the
// Workspace sets none: the Instance's, naming the Workspace.
type ConfirmationEmailDefaults struct {
	FromName string
	Subject  string
	Intro    string
}

// ConfirmationEmailSettings is the confirmation email as a person edits it: what the Workspace
// stores, nil where it keeps the default, beside the defaults and the Workspace's revision.
type ConfirmationEmailSettings struct {
	FromEmail *string
	FromName  *string
	Subject   *string
	Intro     *string
	Defaults  ConfirmationEmailDefaults
	Revision  int64
}

// ConfirmationEmailSettingsOf is the confirmation email's settings of ws, its defaults in the
// Instance's language read now, so a rename of the Workspace carries into them.
func ConfirmationEmailSettingsOf(ws *ent.Workspace) ConfirmationEmailSettings {
	named := map[string]any{"Workspace": ws.Name}
	return ConfirmationEmailSettings{
		FromEmail: ws.ConfirmationFromEmail,
		FromName:  ws.ConfirmationFromName,
		Subject:   ws.ConfirmationSubject,
		Intro:     ws.ConfirmationIntro,
		Defaults: ConfirmationEmailDefaults{
			FromName: ws.Name,
			Subject:  i18n.T(i18n.EmailConfirmationRequestSubject, named),
			Intro:    i18n.T(i18n.EmailConfirmationRequestIntro, named),
		},
		Revision: ws.Revision,
	}
}

// ConfirmationEmailOf is the confirmation email of ws as it goes out: each text the Workspace
// sets, or its default.
func ConfirmationEmailOf(ws *ent.Workspace) ConfirmationEmail {
	set := ConfirmationEmailSettingsOf(ws)
	return ConfirmationEmail{
		FromEmail: lo.FromPtr(set.FromEmail),
		FromName:  lo.FromPtrOr(set.FromName, set.Defaults.FromName),
		Subject:   lo.FromPtrOr(set.Subject, set.Defaults.Subject),
		Intro:     lo.FromPtrOr(set.Intro, set.Defaults.Intro),
	}
}

// HasConfirmationEmail reports whether the Workspace has its confirmation email set up: it has a
// From, which was on a verified Sending domain when it was saved. Without it confirmed opt-in
// cannot be turned on and no confirmation email is requested (ErrConfirmationEmailRequired).
func HasConfirmationEmail(ws *ent.Workspace) bool {
	return ws.ConfirmationFromEmail != nil
}

var (
	// ErrConfirmationEmailRequired refuses confirmed opt-in, or a Confirmation request, while the
	// Workspace has no confirmation email set up (ADR 0013).
	ErrConfirmationEmailRequired = validation.ConfirmationEmailRequired(
		"workspaces: no confirmation email set up", i18n.ErrorsConfirmationEmailRequired)
	// ErrConfirmationFromNotVerified refuses a confirmation email From whose domain is not a
	// verified Sending domain of the Workspace; the refusal names the domain.
	ErrConfirmationFromNotVerified = validation.RefuseField(fieldname.FromEmail,
		"workspaces: the confirmation email's from must be on a verified sending domain", i18n.ErrorsConfirmationFromNotVerified)
)

// SetConfirmationEmailInput is the confirmation email's settings, saved on Revision (ADR 0119).
// FromEmail is required; a FromName, Subject or Intro that is nil or blank goes back to the
// default.
type SetConfirmationEmailInput struct {
	Revision  int64
	FromEmail string
	FromName  *string
	Subject   *string
	Intro     *string
}

// SetConfirmationEmail saves the confirmation email's settings (ADR 0013), each text trimmed. A
// From whose domain is not a verified Sending domain of the Workspace is refused with
// ErrConfirmationFromNotVerified, checked on every save as the send gate checks it, so the email
// is set up only with a From it can send from. The change is the Workspace's `workspace.update`
// Audit entry.
func (m *Module) SetConfirmationEmail(ctx context.Context, s *ent.Scoped, in SetConfirmationEmailInput) (*ent.Workspace, error) {
	from := strings.TrimSpace(in.FromEmail)
	verified, err := messaging.HasVerifiedSendingDomain(ctx, s, from)
	if err != nil {
		return nil, err
	}
	if !verified {
		return nil, ErrConfirmationFromNotVerified.With(messaging.DomainOf(from))
	}
	upd := edit(s, in.Revision).SetConfirmationFromEmail(from)
	setOrClear(in.FromName, upd.SetConfirmationFromName, upd.ClearConfirmationFromName)
	setOrClear(in.Subject, upd.SetConfirmationSubject, upd.ClearConfirmationSubject)
	setOrClear(in.Intro, upd.SetConfirmationIntro, upd.ClearConfirmationIntro)
	return upd.Save(ctx)
}

// setOrClear sets a text setting to v trimmed, or clears it to its default when v is nil or blank.
func setOrClear(v *string, set func(string) *ent.WorkspaceScopedUpdateOne, reset func() *ent.WorkspaceScopedUpdateOne) {
	if v != nil && strings.TrimSpace(*v) != "" {
		set(strings.TrimSpace(*v))
		return
	}
	reset()
}

// DiffConfirmationsRecorded is the key of the switch of confirmed opt-in's Audit diff that holds
// how many Confirmations the switch recorded (ADR 0013): the grandfathered ones turning it on
// asserts, none when it is turned off.
const DiffConfirmationsRecorded = "confirmations_recorded"

// SwitchConfirmedOptInInput turns confirmed opt-in On or off, made on Revision (ADR 0119), when
// Recorded Confirmations were recorded for it.
type SwitchConfirmedOptInInput struct {
	Revision int64
	On       bool
	Recorded int
}

// ConfirmedOptInSwitch decides a switch of ws's confirmed opt-in to on, made on revision (ADR
// 0013, ADR 0119), before anything is written for it: whether it changes the Workspace (a switch to
// the state it is in changes nothing), refused with ErrRevisionChanged on a revision the Workspace
// has moved past, and turning it on with ErrConfirmationEmailRequired while the Workspace has no
// confirmation email set up.
func ConfirmedOptInSwitch(ws *ent.Workspace, revision int64, on bool) (changes bool, err error) {
	switch {
	case ws.RequireConfirmedOptIn == on:
		return false, nil
	case ws.Revision != revision:
		return false, ErrRevisionChanged
	case on && !HasConfirmationEmail(ws):
		return false, ErrConfirmationEmailRequired
	}
	return true, nil
}

// SwitchConfirmedOptInIn turns the scoped Workspace's confirmed opt-in on or off in the
// transaction s is bound to, refused as ConfirmedOptInSwitch refuses it, and records its one
// `workspace.update` Audit entry with the number of Confirmations in.Recorded, which a change of a
// column alone cannot carry: the write itself goes through an unaudited scope. A switch to the
// state the Workspace is in writes nothing and records nothing.
func (m *Module) SwitchConfirmedOptInIn(ctx context.Context, s *ent.Scoped, in SwitchConfirmedOptInInput) (*ent.Workspace, error) {
	before, err := s.Workspace(ctx)
	if err != nil {
		return nil, err
	}
	changes, err := ConfirmedOptInSwitch(before, in.Revision, in.On)
	if err != nil || !changes {
		return before, err
	}
	after, err := edit(events.Unaudited(s), in.Revision).SetRequireConfirmedOptIn(in.On).Save(ctx)
	if err != nil {
		return nil, err
	}
	diff := map[string]any{
		workspace.FieldRequireConfirmedOptIn: events.Change(before.RequireConfirmedOptIn, in.On),
		DiffConfirmationsRecorded:            in.Recorded,
	}
	return after, m.bus.RecordAuditIn(ctx, s, events.WorkspaceAudit(after, s.Actor(), events.ActionWorkspaceUpdate, diff))
}
