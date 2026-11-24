package outboundmessages_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages/engagement"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A render concurrent with a refusal (ADR 0120): the render reads the Destination's consent while
// the refusal is still in flight and the message goes out tracked; the reach-back the refusal starts
// once it commits finds the message by its Destination and withdraws it, so an open of it records
// nothing.
func TestAMessageRenderedWhileARefusalIsInFlightIsWithdrawnByItsReachBack(t *testing.T) {
	env := testhelper.Setup(t, testhelper.Committed())
	ctx := context.Background()
	mod := newModule(env, time.Now)
	req := marketing(t, env, "render-meets-refusal", fixtures.ContactAliceID)
	req.Track = true
	s := env.DB.Scoped(fixtures.AcmeID)
	release := env.HoldOpen(t, s, func(ts *ent.Scoped, _ events.Publisher) error {
		_, err := testhelper.Module[*engagementtracking.Module](env).RecordIn(ctx, ts, engagementtracking.RecordInput{
			Channel: channels.Email, Destination: req.Destination,
			State: engagementtracking.Denied, Provenance: engagementtracking.Manual,
		})
		return err
	})

	res, err := mod.Send(ctx, s, req)
	require.NoError(t, err)
	release()
	env.PerformEnqueuedJobs(t, jobs.TrackingWithdrawalArgs{})
	token, err := env.Tracker.Token(env.Tracked(t, res.MessageID))
	require.NoError(t, err)
	hit := engagement.Hit{UserAgent: "Mozilla/5.0 (Macintosh) Safari/605.1.15", Address: "192.0.2.10", MayRecord: func() bool { return true }}
	require.NoError(t, testhelper.Module[*engagement.Recorder](env).Open(ctx, token, hit))

	sent := env.CustomerMail.Messages()
	require.Len(t, sent, 1)
	assert.Contains(t, sent[0].HTML, "/e/o/", "the render read the consent before the refusal committed")
	msg := env.DB.OutboundMessage.GetX(ctx, res.MessageID)
	assert.False(t, engagementtracking.IsTracked(msg), "the reach-back withdrew the message")
	opens := testhelper.Queued[*events.EmailEngagement](t, env, systemevent.EmailOpened)
	assert.Empty(t, opens, "an open of the withdrawn message records nothing")
}
