package contacts

import (
	"context"
	"log/slog"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/domain/contacts/confirmations"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/batch"
)

// IntakeInput is one row of a Contact intake (GLOSSARY, ADR 0022): the Contact it asserts and the
// SMS consent it carries.
type IntakeInput struct {
	UpsertInput

	// SMSConsent is the evidence of SMS consent the Contact's phone gave outside sphericon (an
	// import's consent columns): the row records an imported SMS Confirmation for the phone in
	// the same transaction (ADR 0077). nil records none.
	SMSConsent *confirmations.Evidence
}

// UpsertBatch is the Contact intake: it upserts each item in its own transaction, in order, so a
// failing item neither rolls back nor blocks the others; each new Contact publishes
// contact.created with its own commit, and a row that carries SMS consent records its imported
// Confirmation (confirmations, ADR 0077) in the row's transaction. The outcomes are in the order
// of items: the Contact's Result, or the domain or storage error of its row.
//
// A batch is an import (ADR 0022): its rows are written through an unaudited scope and
// the whole batch is recorded as ONE contact.import entry (counts only), not one entry
// per row. It brings earlier state rather than something happening now, so it records no
// contact.field_changed (#897). The rows commit one by one (a failing row must not poison the others), so
// the entry cannot share their transaction: if recording it fails the failure is
// logged and the committed outcomes are still returned, never a 500 for an import
// that happened.
func (m *Module) UpsertBatch(ctx context.Context, s *ent.Scoped, src contact.Source, items []IntakeInput) batch.Result[Result] {
	rows := events.Unaudited(s)
	out := batch.Each(items, func(in IntakeInput) (Result, error) { return m.intake(ctx, rows, src, in) })
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
	return out
}

// intake is one row in its own transaction: the Contact, then its SMS consent.
func (m *Module) intake(ctx context.Context, s *ent.Scoped, src contact.Source, in IntakeInput) (Result, error) {
	var res Result
	err := m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		var err error
		res, err = importIn(ctx, ts, pub, src, in.UpsertInput)
		if err != nil {
			return err
		}
		return confirmSMS(ctx, ts, pub, res, in.SMSConsent)
	})
	if err != nil {
		return Result{}, err
	}
	return res, nil
}

// confirmSMS records the SMS consent a row carries for the phone it names, or the Contact's
// own when it names none: the evidence was collected for that number. Consent without a phone is
// refused, so an import row never drops it silently.
func confirmSMS(ctx context.Context, s *ent.Scoped, pub events.Publisher, res Result, ev *confirmations.Evidence) error {
	if ev == nil {
		return nil
	}
	number := lo.CoalesceOrEmpty(lo.FromPtr(res.Phone), lo.FromPtr(res.Contact.Phone))
	_, err := confirmations.ConfirmContactSMSIn(ctx, s, pub, number, *ev)
	return err
}
