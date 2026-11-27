package contactimports

import (
	"context"
	"fmt"

	"github.com/getsphericon/sphericon/ent"
	entbinding "github.com/getsphericon/sphericon/ent/binding"
	entsuppression "github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/contacts/suppressions"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/batch"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/csvimport"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// refuse records the refusals a row brings, after the intake wrote it or refused it: a refusal
// narrows consent, so it is recorded whatever became of the row, a Contact the Workspace has left
// as it is included (ADR 0138). Each is recorded as the Member who runs the import, unaudited
// as the intake's rows are, under the job's history mark, which holds back its Events as triggers
// and nothing of its effects. A Telegram refusal is recorded at the Bindings of the Contact the
// row wrote, none when the intake refused it. A refusal a writer refuses is reported; any other
// error ends the job.
func (b batchRun) refuse(ctx context.Context, t *tally, r readRecord, item batch.Item[contacts.Result]) ([]csvimport.Issue, error) {
	if len(r.read.refusals) == 0 {
		return nil, nil
	}
	ctx = events.WithCause(ctx, b.cause)
	s := events.Unaudited(b.member)
	var notRecorded []csvimport.Issue
	for _, f := range r.read.refusals {
		destinations := []string{f.destination}
		contactID := int64(0)
		if item.Err == nil && item.Value.Contact != nil {
			contactID = item.Value.Contact.ID
		}
		if f.channel == channels.Telegram {
			users, err := telegramUsersOf(ctx, s, contactID)
			if err != nil {
				return nil, err
			}
			destinations = users
		}
		for _, dest := range destinations {
			stands, err := b.record(ctx, s, f, dest, contactID)
			if err == nil {
				switch {
				case !stands:
				case f.refusal == csvimport.RefusalSuppression:
					t.suppressed++
				default:
					t.optedOut++
				}
				continue
			}
			shown, ok := validation.Shown(err)
			if !ok {
				return nil, fmt.Errorf("record a refusal of import row %d: %w", r.rec.row, err)
			}
			at := f.column
			notRecorded = append(notRecorded, csvimport.Issue{
				Column: &at, Reason: csvimport.ReasonRefusalNotRecorded,
				Message: i18n.T(reasonMessages[csvimport.ReasonRefusalNotRecorded], map[string]any{"Column": cellAt(b.rd.header, at), "Detail": shown}),
			})
		}
	}
	return notRecorded, nil
}

// record records one refusal at dest, and reports whether it stands: a Suppression of the manual
// reason, or an opt-out of its scope `via: imported` with its effects, which changes nothing when
// it is older than the Destination's return.
func (b batchRun) record(ctx context.Context, s *ent.Scoped, f refusalFact, dest string, contactID int64) (bool, error) {
	if f.refusal == csvimport.RefusalSuppression {
		err := b.m.suppressions.Add(ctx, s, suppressions.Entry{Channel: f.channel, Destination: dest, Reason: entsuppression.ReasonManual, ContactID: contactID})
		return err == nil, err
	}
	source := eligibility.SourceEverything
	if f.refusal == csvimport.RefusalBroadcasts {
		source = eligibility.SourceBroadcasts
	}
	return b.m.optOuts.OptOutImported(ctx, s, ImportedOptOut{Channel: f.channel, Destination: dest, Source: source, At: f.at})
}

// telegramUsersOf are the Telegram users of the Contact's own Bindings, its Destinations on
// Telegram; none for no Contact.
func telegramUsersOf(ctx context.Context, s *ent.Scoped, contactID int64) ([]string, error) {
	if contactID == 0 {
		return nil, nil
	}
	return s.Binding().Query().
		Where(entbinding.ChannelEQ(entbinding.ChannelTelegram), entbinding.ContactID(contactID)).
		Unique(true).
		Order(entbinding.ByDestination()).
		Select(entbinding.FieldDestination).
		Strings(ctx)
}
