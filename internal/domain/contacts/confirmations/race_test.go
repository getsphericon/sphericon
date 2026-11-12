package confirmations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/domain/contacts/confirmations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/linktarget"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

var evidence = confirmations.Evidence{Wording: "Text me offers", Source: "checkout", Brand: "Acme"}

// A Confirmation that a concurrent request beat to the number finds it written when its insert
// goes ahead (ON CONFLICT DO NOTHING): it answers with the Confirmation that stands and publishes
// no second sms.confirmed.
func TestAConfirmationThatLosesTheRaceAnswersWithTheOneThatStands(t *testing.T) {
	race := testhelper.NewWorkspaceRace(t, fixtures.AcmeID)
	ctx := context.Background()
	const number = "+14155550199"

	var first *ent.Confirmation
	release := race.HoldOpen(t, race.Scope, func(ts *ent.Scoped, pub events.Publisher) error {
		var err error
		first, err = confirmations.ConfirmSMSIn(ctx, ts, pub, number, evidence)
		return err
	})

	var second *ent.Confirmation
	errs := testhelper.QueuedBehind(t, race.DB, release,
		testhelper.Writer{Name: "second request", Run: func() error {
			var err error
			second, err = confirmations.Confirm(ctx, race.Bus, race.Scope, channels.SMS, number, evidence)
			return err
		}})

	require.NoError(t, errs["second request"])
	require.NotNil(t, second)
	assert.Equal(t, first.ID, second.ID, "the second request answers with the Confirmation that stands")
	logged, err := entclient.Core.New(race.DB).Event.Query().Where(
		event.WorkspaceID(fixtures.AcmeID), event.Action(systemevent.SmsConfirmed), event.Phone(number),
	).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, logged, "only the request that wrote the Confirmation publishes its proof")
}

// A double opt-in link clicked after its Contact was erased records the Confirmation by its
// address, unlinked, rather than failing on the Contact's foreign key.
func TestAConfirmationOfAnErasedContactIsRecordedUnlinked(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	const address = "gone@example.com"

	err := confirmations.RecordConfirmation(ctx, env.Bus, env.DB.Scoped(fixtures.AcmeID), linktarget.Confirmation{
		WorkspaceID: fixtures.AcmeID, ContactID: 999999, Destination: address,
	}, "")

	require.NoError(t, err)
	got, err := env.DB.Scoped(fixtures.AcmeID).Confirmation().Query().
		Where(confirmation.ChannelEQ(channels.Email), confirmation.DestinationEQ(address)).Only(ctx)
	require.NoError(t, err)
	assert.Nil(t, got.ContactID)
}
