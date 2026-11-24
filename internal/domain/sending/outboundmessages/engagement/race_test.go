package engagement_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages/engagement"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The recorder checks a message under its row (ADR 0120): an open that arrives while the
// withdrawal of the message's tracking is being written waits for it, and once it commits finds the
// message withdrawn and records nothing.
func TestAnOpenThatMeetsAWithdrawalInFlightWaitsAndRecordsNothing(t *testing.T) {
	env := testhelper.Setup(t, testhelper.Committed())
	ctx := t.Context()
	const id = fixtures.OutboundMessageTrackingSampleID
	token := openToken(t, env, id)

	// The withdrawal as its writer makes it: an update of the message's row, held uncommitted.
	release := env.HoldOpen(t, env.DB.Scoped(fixtures.AcmeID), func(ts *ent.Scoped, _ events.Publisher) error {
		return ts.OutboundMessage().Update().
			Where(outboundmessage.ID(id), outboundmessage.TrackingWithdrawnAtIsNil()).
			SetTrackingWithdrawnAt(time.Now()).
			Exec(ctx)
	})
	errs := env.QueueBehind(t, release, testhelper.Writer{Name: "open", Run: func() error {
		return testhelper.Module[*engagement.Recorder](env).Open(ctx, token, person(0))
	}})

	require.NoError(t, errs["open"])
	opens := testhelper.Queued[*events.EmailEngagement](t, env, systemevent.EmailOpened)
	assert.Empty(t, opens, "the open came after the withdrawal and records nothing")
}
