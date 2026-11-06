// Package contactintake is the use case that takes Contacts in from an import (ADR 0022, ADR
// 0102): each row upserts a Contact (contacts) and, when the row carries the evidence of SMS
// consent the phone gave outside sphericon, records its imported Confirmation (confirmations, ADR
// 0077) in the same transaction. The contacts module knows no Confirmation, so the operation
// across both lives here.
package contactintake

import (
	"context"
	"log/slog"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/domain/contacts/confirmations"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// UseCase takes Contacts in. Every call receives the Workspace-scoped client (ADR 0017) built by
// the entry point; the use case opens the transaction of each row, which the modules it calls join.
type UseCase struct {
	bus      *events.Bus
	contacts *contacts.Module
}

// New builds the Contact intake use case.
func New(bus *events.Bus, module *contacts.Module) *UseCase {
	return &UseCase{bus: bus, contacts: module}
}

// Input is one row of an import: the Contact it asserts and the SMS consent it carries.
type Input struct {
	contacts.UpsertInput

	// SMSConsent is the evidence of SMS consent the Contact's phone gave outside sphericon (an
	// import's consent columns): the row records an imported SMS Confirmation for the phone in
	// the same transaction (ADR 0077). nil records none.
	SMSConsent *confirmations.Evidence
}

// BatchOutcome is the result of one item of UpsertBatch: Result on success, Err (a
// domain error, or a storage error) otherwise.
type BatchOutcome struct {
	Result contacts.Result
	Err    error
}

// UpsertBatch upserts each item in its own transaction, in order, so a failing item
// neither rolls back nor blocks the others; each new Contact publishes contact.created
// with its own commit. The outcomes are parallel to items.
//
// A batch is an import (ADR 0022): its rows are written through an unaudited scope and
// the whole batch is recorded as ONE contact.import entry (counts only), not one entry
// per row. It brings earlier state rather than something happening now, so it records no
// contact.field_changed (#897). The rows commit one by one (a failing row must not poison the others), so
// the entry cannot share their transaction: if recording it fails the failure is
// logged and the committed outcomes are still returned, never a 500 for an import
// that happened.
func (u *UseCase) UpsertBatch(ctx context.Context, s *ent.Scoped, src contact.Source, items []Input) []BatchOutcome {
	rows := events.Unaudited(s)
	out := make([]BatchOutcome, len(items))
	var sum contacts.ImportSummary
	for i := range items {
		out[i].Result, out[i].Err = u.upsert(ctx, rows, src, items[i])
		switch {
		case out[i].Err != nil:
			sum.Failed++
		case out[i].Result.Created:
			sum.Created++
		default:
			sum.Updated++
		}
	}
	if err := u.contacts.RecordImport(ctx, s, sum); err != nil {
		slog.ErrorContext(ctx, "record contact.import audit entry", "workspace_id", s.WorkspaceID(), "error", err)
	}
	return out
}

// upsert is one row in its own transaction: the Contact, then its SMS consent.
func (u *UseCase) upsert(ctx context.Context, s *ent.Scoped, src contact.Source, in Input) (contacts.Result, error) {
	var res contacts.Result
	err := u.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		var err error
		res, err = contacts.ImportIn(ctx, ts, pub, src, in.UpsertInput)
		if err != nil {
			return err
		}
		return confirmSMS(ctx, ts, pub, res, in.SMSConsent)
	})
	if err != nil {
		return contacts.Result{}, err
	}
	return res, nil
}

// confirmSMS records the SMS consent a row carries for the phone it names, or the Contact's
// own when it names none: the evidence was collected for that number. Consent without a phone is
// refused, so an import row never drops it silently.
func confirmSMS(ctx context.Context, s *ent.Scoped, pub events.Publisher, res contacts.Result, ev *confirmations.Evidence) error {
	if ev == nil {
		return nil
	}
	number := lo.CoalesceOrEmpty(lo.FromPtr(res.Phone), lo.FromPtr(res.Contact.Phone))
	if number == "" {
		return confirmations.ErrNoPhone
	}
	_, err := confirmations.ConfirmSMSIn(ctx, s, pub, number, *ev)
	return err
}
