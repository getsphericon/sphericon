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
	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/contacts/unsubscribes"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/linktarget"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/unsubscriberecording"
)

// A click that a concurrent one beat to the opt-out finds it written when its insert goes ahead
// (ON CONFLICT DO NOTHING): it succeeds and records nothing more, no second email.unsubscribed.
func TestAClickThatLosesTheRaceToTheOptOutRecordsNothingMore(t *testing.T) {
	race := testhelper.NewWorkspaceRace(t, fixtures.AcmeID)
	ctx := context.Background()
	dest := fixtures.ContactAliceEmail

	release := race.HoldOpen(t, events.Ingest(race.Scope), firstOptOut(ctx, dest))

	errs := testhelper.QueuedBehind(t, race.DB, release,
		testhelper.Writer{Name: "second click", Run: func() error {
			return unsubscriberecording.New(race.Bus, time.Now).RecordLink(ctx, race.Scope, linktarget.Unsubscribe{
				WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, Channel: channels.Email,
				Destination: dest, Source: eligibility.SourceEverything,
			})
		}})

	require.NoError(t, errs["second click"])
	client := entclient.Core.New(race.DB)
	rows, err := client.Unsubscribe.Query().Where(
		unsubscribe.WorkspaceID(fixtures.AcmeID), unsubscribe.DestinationEQ(dest),
		unsubscribe.SendingSourceEQ(eligibility.SourceEverything),
	).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, rows)
	logged, err := client.Event.Query().Where(
		event.WorkspaceID(fixtures.AcmeID), event.Action(events.NameEmailUnsubscribed), event.Email(dest),
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
		})
		if err == nil && added == 0 {
			return errors.New("the first click found the opt-out already written")
		}
		return err
	}
}
