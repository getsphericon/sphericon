package contacts

import (
	"context"
	"log/slog"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/domain/contacts/confirmations"
	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/addresscheck"
	"github.com/getsphericon/sphericon/internal/kernel/batch"
	"github.com/getsphericon/sphericon/internal/kernel/bounds"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/emailcase"
	"github.com/getsphericon/sphericon/internal/kernel/fieldname"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// IntakeInput is one row of a Contact intake (GLOSSARY, ADR 0022): the Contact it asserts, the Tags
// it adds and the email, SMS and Engagement tracking consent it carries.
type IntakeInput struct {
	UpsertInput

	// Tags are the names of Tags the row adds to the Contact, each created on first use (ADR 0138).
	// A Tag the Contact has stays, and none is removed.
	Tags []string

	// EmailConsent is the evidence of email consent the Contact's address gave outside sphericon
	// (an import's consent columns): the row records an imported email Confirmation for the
	// address in the same transaction (ADR 0138). nil records none.
	EmailConsent *confirmations.Evidence

	// SMSConsent is the evidence of SMS consent the Contact's phone gave outside sphericon (an
	// import's consent columns): the row records an imported SMS Confirmation for the phone in
	// the same transaction (ADR 0077). nil records none.
	SMSConsent *confirmations.Evidence

	// EngagementTracking is the Engagement tracking consent the Contact's Destinations gave
	// outside sphericon (ADR 0120): the row records it as imported in the same transaction.
	EngagementTracking engagementtracking.ContactObservations
}

// Warning is a trait of an intake row worth the importer's attention; its Contact is stored all the
// same. The values are the contract's ContactBatchWarning.
type Warning string

const (
	// WarningDisposableEmail: the row's address is at a throwaway mailbox provider (ADR 0117).
	WarningDisposableEmail Warning = "disposable_email"
	// WarningRoleEmail: the row's address is a function's rather than a person's (ADR 0117).
	WarningRoleEmail Warning = "role_email"
	// WarningConsentTimeInFuture: a consent of the row stated a time after now, which was dropped
	// and the consent recorded as given now (ADR 0138).
	WarningConsentTimeInFuture Warning = "consent_time_in_future"
	// WarningKeysNameTwoContacts: the row's alias keys named two Contacts, and it was written to the
	// one their rank picks without the keys of the other (ADR 0138). Only a CSV import's rows carry
	// it (Import); the API's batch upsert refuses such an item as a key taken.
	WarningKeysNameTwoContacts Warning = "keys_name_two_contacts"
)

// intakeMode is how a Contact intake writes its rows: onlyNew leaves a Contact the Workspace has as
// it is but for its refusals, rankKeys writes a row whose keys name two Contacts to the one their
// rank picks (the CSV import's, ADR 0138).
type intakeMode struct {
	onlyNew, rankKeys bool
}

// UpsertBatch is the Contact intake of the API's batch upsert: it upserts each item in its own
// transaction, in order, so a failing item neither rolls back nor blocks the others; each new
// Contact publishes contact.created with its own commit, and a row's Tags and consent are written
// in the row's transaction. The outcomes are in the order of items: the Contact's Result, or the
// domain or storage error of its row (intake says what a row does and refuses).
//
// A batch is an import (ADR 0022): its rows are written through an unaudited scope and
// the whole batch is recorded as ONE contact.import entry (counts only), not one entry
// per row. It brings earlier state rather than something happening now, so it records no
// contact.field_changed (#897). The rows commit one by one (a failing row must not poison the others), so
// the entry cannot share their transaction: if recording it fails the failure is
// logged and the committed outcomes are still returned, never a 500 for an import
// that happened. A batch of fewer or more items than the contract's ContactBatch is refused whole,
// before any row.
func (m *Module) UpsertBatch(ctx context.Context, s *ent.Scoped, src contact.Source, items []IntakeInput) (batch.Result[Result], error) {
	out, err := m.rows(ctx, s, src, items, intakeMode{})
	if err != nil {
		return batch.Result[Result]{}, err
	}
	var sum importSummary
	for _, o := range out.Items {
		switch {
		case o.Err != nil:
			sum.Failed++
		case o.Value.Created:
			sum.Created++
		default:
			sum.Updated++
		}
	}
	if err := m.recordImport(ctx, s, sum); err != nil {
		slog.ErrorContext(ctx, "record contact.import audit entry", "workspace_id", s.WorkspaceID(), "error", err)
	}
	return out, nil
}

// ImportInput is one batch of a CSV import's job (ADR 0138): its rows, typed already, the job's
// cause and the "only add new Contacts" choice.
type ImportInput struct {
	Rows []IntakeInput
	// Cause names the job on every Event the rows publish: events.Import, history that starts
	// nothing, or events.ImportAsNew when its Member chose to treat them as new sign-ups.
	Cause events.Cause
	// OnlyAddNew leaves a Contact the Workspace has as it is but for the refusals its row carries,
	// which narrow consent and are always recorded (Result.Kept).
	OnlyAddNew bool
}

// Import is the Contact intake's entry for a CSV import's job (ADR 0138): the rows of
// UpsertBatch, written through the scope s, which acts as the Member who ran the import, under the
// job's cause, with no Audit entry of its own: the job records one contact.import for the whole
// import. A Contact it creates has the source `import`. A row whose keys name two Contacts is
// written to the one their rank picks, with WarningKeysNameTwoContacts.
func (m *Module) Import(ctx context.Context, s *ent.Scoped, in ImportInput) (batch.Result[Result], error) {
	return m.rows(events.WithCause(ctx, in.Cause), s, contact.SourceImport, in.Rows, intakeMode{onlyNew: in.OnlyAddNew, rankKeys: true})
}

// rows writes each item through an unaudited scope, in order, each in a transaction of its own
// (intake), after reading the mail verdicts of their domains once.
func (m *Module) rows(ctx context.Context, s *ent.Scoped, src contact.Source, items []IntakeInput, mode intakeMode) (batch.Result[Result], error) {
	if err := validation.Items(fieldname.Contacts, len(items), bounds.ContactBatchMinItems, bounds.ContactBatchMaxItems); err != nil {
		return batch.Result[Result]{}, err
	}
	rows := events.Unaudited(s)
	mail := m.mailVerdicts(ctx, items)
	return batch.Each(items, func(in IntakeInput) (Result, error) { return m.intake(ctx, rows, src, in, mail, mode) }), nil
}

// mailVerdicts reads whether the domain of each address the batch asserts accepts mail, one
// lookup per domain.
func (m *Module) mailVerdicts(ctx context.Context, items []IntakeInput) map[string]addresscheck.Verdict {
	domains := lo.Uniq(lo.FilterMap(items, func(in IntakeInput, _ int) (string, bool) {
		d := addresscheck.Domain(lo.FromPtr(in.Email))
		return d, d != ""
	}))
	return addresscheck.Domains(ctx, m.mail, domains)
}

// intake is one row in its own transaction: the address check, the type of each Custom field
// value the Workspace has a field of, the Contact, its Tags, then its email, SMS and Engagement
// tracking consent. Before any row it reads, once per domain and outside every transaction,
// whether the domains of the batch's addresses accept mail (ADR 0117): a row that brings an
// address new to the Workspace at a domain that cannot receive mail is refused
// (ErrEmailDomainNoMail), and a stored row's Result carries the Warnings of its address. A consent
// time after now is dropped and the row warned (WarningConsentTimeInFuture). With mode.onlyNew, a
// row that resolves to a Contact the Workspace has records only the refusals it carries.
func (m *Module) intake(ctx context.Context, s *ent.Scoped, src contact.Source, in IntakeInput, mail map[string]addresscheck.Verdict, mode intakeMode) (Result, error) {
	in, warnings := m.dated(in)
	var res Result
	err := m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		if err := admitAddress(ctx, ts, in.Email, mail); err != nil {
			return err
		}
		if err := checkFieldTypes(ctx, ts, in.CustomFields); err != nil {
			return err
		}
		var err error
		res, err = m.importIn(ctx, ts, src, in.UpsertInput, mode)
		if err != nil {
			return err
		}
		if res.Kept {
			in.EngagementTracking = engagementtracking.RefusalsOf(in.EngagementTracking)
			return m.recordTracking(ctx, ts, res, in)
		}
		for _, name := range in.Tags {
			if _, err := m.ApplyTagIn(ctx, ts, res.Contact.ID, name); err != nil {
				return err
			}
		}
		if err := m.confirm(ctx, ts, res, in); err != nil {
			return err
		}
		return m.recordTracking(ctx, ts, res, in)
	})
	if err != nil {
		return Result{}, err
	}
	if in.Email != nil {
		res.Warnings = append(addressWarnings(emailcase.Of(*in.Email)), res.Warnings...)
	}
	res.Warnings = append(res.Warnings, warnings...)
	return res, nil
}

// dated is in with each consent time after now dropped, so the consent is recorded as given now
// (confirmations, ADR 0154), and the warning that says so.
func (m *Module) dated(in IntakeInput) (IntakeInput, []Warning) {
	now := m.now()
	future := false
	for _, ev := range []**confirmations.Evidence{&in.EmailConsent, &in.SMSConsent} {
		if *ev == nil || !(*ev).OccurredAt.After(now) {
			continue
		}
		undated := **ev
		undated.OccurredAt = time.Time{}
		*ev, future = &undated, true
	}
	if !future {
		return in, nil
	}
	return in, []Warning{WarningConsentTimeInFuture}
}

// addressWarnings are the traits of address worth the importer's attention (ADR 0117), as Warnings.
func addressWarnings(address string) []Warning {
	return lo.FilterMap(addresscheck.Flags(address), func(f addresscheck.Flag, _ int) (Warning, bool) {
		w, ok := flagWarnings[f]
		return w, ok
	})
}

// flagWarnings are the Warnings of the address check's flags.
var flagWarnings = map[addresscheck.Flag]Warning{
	addresscheck.Disposable: WarningDisposableEmail,
	addresscheck.Role:       WarningRoleEmail,
}

// admitAddress refuses an address new to the Workspace whose domain cannot receive mail (ADR
// 0117). An address a Contact already holds is past the check: the row sends again what the
// audience has, so the import stays repeatable whatever DNS answers now.
func admitAddress(ctx context.Context, s *ent.Scoped, email *string, mail map[string]addresscheck.Verdict) error {
	if email == nil {
		return nil
	}
	address := emailcase.Of(*email)
	if mail[addresscheck.Domain(address)] != addresscheck.NoMail {
		return nil
	}
	held, err := s.Contact().Query().Where(contact.EmailEQ(address)).Exist(ctx)
	if err != nil {
		return err
	}
	if held {
		return nil
	}
	return ErrEmailDomainNoMail
}

// confirm records the email and SMS consent a row carries (confirmations, ADR 0077, ADR 0138)
// for the address and the phone the row names, or the Contact's own when it names none: the
// evidence was collected for them. Consent for a Destination the Contact lacks is refused, so an
// import row never drops it silently.
func (m *Module) confirm(ctx context.Context, s *ent.Scoped, res Result, in IntakeInput) error {
	if ev := in.EmailConsent; ev != nil {
		err := m.confirmations.ImportIn(ctx, s, confirmations.ImportInput{
			Channel:     channels.Email,
			Destination: lo.CoalesceOrEmpty(emailcase.Of(lo.FromPtr(in.Email)), lo.FromPtr(res.Contact.Email)),
			Evidence:    *ev,
		})
		if err != nil {
			return err
		}
	}
	if ev := in.SMSConsent; ev != nil {
		return m.confirmations.ImportIn(ctx, s, confirmations.ImportInput{
			Channel:     channels.SMS,
			Destination: lo.CoalesceOrEmpty(lo.FromPtr(res.Phone), lo.FromPtr(res.Contact.Phone)),
			Evidence:    *ev,
		})
	}
	return nil
}

// recordTracking records the Engagement tracking consent a row carries for the Contact's email
// address and phone: the row's, else the ones the Contact has. Consent for a Destination the
// Contact lacks is refused. The consent was collected for the address and the number the row
// names, as SMS consent for its phone.
func (m *Module) recordTracking(ctx context.Context, s *ent.Scoped, res Result, in IntakeInput) error {
	return m.tracking.ImportIn(ctx, s, engagementtracking.ImportInput{
		Email:        lo.CoalesceOrEmpty(emailcase.Of(lo.FromPtr(in.Email)), lo.FromPtr(res.Contact.Email)),
		Phone:        lo.CoalesceOrEmpty(lo.FromPtr(res.Phone), lo.FromPtr(res.Contact.Phone)),
		Observations: in.EngagementTracking,
	})
}

// Verdict is what the Contact intake would make of a row, written or not: why it would refuse it
// (nil when it would write it) and the Warnings of its address.
type Verdict struct {
	Refusal  error
	Warnings []Warning
}

// Verdicts judge rows as the intake would, writing nothing (a CSV import's preview, ADR 0138): a
// row with no alias key, an address that is none or whose domain cannot receive mail while no
// Contact holds it (ADR 0117, the domains read once), a phone that cannot be read, or a Custom field
// value its field's type does not read is refused with the intake's own refusal; a row it would
// write carries the Warnings of its address. A refusal of a key another Contact holds is not
// judged: it needs the rows written before it.
func (m *Module) Verdicts(ctx context.Context, s *ent.Scoped, rows []IntakeInput) ([]Verdict, error) {
	mail := m.mailVerdicts(ctx, rows)
	out := make([]Verdict, len(rows))
	for i, row := range rows {
		v, err := verdict(ctx, s, row, mail)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

// verdict is the Verdict of row, or the error of a read.
func verdict(ctx context.Context, s *ent.Scoped, row IntakeInput, mail map[string]addresscheck.Verdict) (Verdict, error) {
	in := row.UpsertInput.normalized()
	if lo.FromPtr(in.SubjectID) == "" && in.Email == nil && in.Phone == nil {
		return Verdict{Refusal: ErrIdentityRequired}, nil
	}
	email, err := checked(in.Email, in.City.Ptr(), in.Country.Ptr(), in.Locale.Ptr(), in.Avatar.Ptr())
	if err != nil {
		return Verdict{Refusal: err}, nil
	}
	for _, check := range []func() error{
		func() error { return admitAddress(ctx, s, email, mail) },
		func() error { _, err := upsertedPhone(ctx, s, in); return err },
		func() error { return checkFieldTypes(ctx, s, in.CustomFields) },
	} {
		err := check()
		if _, refused := validation.Shown(err); refused {
			return Verdict{Refusal: err}, nil
		}
		if err != nil {
			return Verdict{}, err
		}
	}
	if email == nil {
		return Verdict{}, nil
	}
	return Verdict{Warnings: addressWarnings(*email)}, nil
}
