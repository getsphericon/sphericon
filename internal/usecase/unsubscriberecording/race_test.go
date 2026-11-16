package unsubscriberecording_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/internal/domain/contacts/consentstamps"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/contacts/unsubscribes"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/linktarget"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/unsubscriberecording"
)

// A click that a concurrent one beat to the opt-out finds it written when its insert goes ahead
// (ON CONFLICT DO NOTHING): it succeeds and records nothing more, no second email.unsubscribed.
func TestAClickThatLosesTheRaceToTheOptOutRecordsNothingMore(t *testing.T) {
	env := testhelper.Setup(t, testhelper.Committed())
	scope := env.DB.Scoped(fixtures.AcmeID)
	ctx := context.Background()
	dest := fixtures.ContactAliceEmail

	release := env.HoldOpen(t, events.Ingest(scope), firstOptOut(ctx, dest))

	errs := env.QueueBehind(t, release,
		testhelper.Writer{Name: "second click", Run: func() error {
			return testhelper.Module[*unsubscriberecording.UseCase](env).RecordLink(ctx, scope, linktarget.Unsubscribe{
				WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, Channel: channels.Email,
				Destination: dest, Source: eligibility.SourceEverything,
			})
		}})

	require.NoError(t, errs["second click"])
	client := env.DB
	rows, err := client.Unsubscribe.Query().Where(
		unsubscribe.WorkspaceID(fixtures.AcmeID), unsubscribe.DestinationEQ(dest),
		unsubscribe.SendingSourceEQ(eligibility.SourceEverything),
	).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, rows)
	logged, err := client.Event.Query().Where(
		event.WorkspaceID(fixtures.AcmeID), event.Action(systemevent.EmailUnsubscribed), event.Email(dest),
	).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, logged, "the click that found the opt-out written publishes nothing")
}

// firstOptOut is the write of the click that wins the race: the opt-out of dest from everything,
// which fails when it was written already, since the race needs this click to write it.
func firstOptOut(ctx context.Context, dest string) func(ts *ent.Scoped, _ events.Publisher) error {
	return func(ts *ent.Scoped, _ events.Publisher) error {
		added, err := unsubscribes.Add(ctx, ts, unsubscribes.OptOut{
			Channel: channels.Email, Destination: dest, Source: eligibility.SourceEverything,
			At: consentstamps.At(time.Now()),
		})
		if err == nil && added.ID == 0 {
			return errors.New("the first click found the opt-out already written")
		}
		return err
	}
}
