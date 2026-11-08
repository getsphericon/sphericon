package broadcasts_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/domain/sending/holds"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
)

// runQueued runs the plan job queued for a Broadcast (at the schedule given, nil for a send now)
// and returns its outcome with what the scheduler saw.
func (d *delivery) runQueued(id int64, scheduledAt *time.Time) (broadcasts.Outcome, *recordingScheduler) {
	sched := &recordingScheduler{}
	out := d.del.RunQueued(d.t.Context(), d.s, broadcasts.SendArgs{BroadcastID: id, ScheduledAt: scheduledAt}, sched)
	return out, sched
}

// firstAttempt and lastAttempt are a recipient job's runs under a budget of three.
var (
	firstAttempt = broadcasts.Attempt{Number: 1, Limit: 3}
	lastAttempt  = broadcasts.Attempt{Number: 3, Limit: 3}
)

// deliver runs one attempt at a recipient's send job.
func (d *delivery) deliver(recipientID int64, attempt broadcasts.Attempt) broadcasts.Outcome {
	return d.del.DeliverRecipient(d.t.Context(), d.s, recipientID, attempt)
}

func (d *delivery) status(id int64) broadcast.Status {
	return d.env.DB.Broadcast.GetX(d.t.Context(), id).Status
}

// A queued job runs only a Broadcast that was claimed for it (scheduled or sending). One that an
// unschedule returned to draft after the job was queued stays a draft: the job is done with
// nothing planned.
func TestAQueuedPlanOfADraftIsDoneWithNothingPlanned(t *testing.T) {
	d := newDelivery(t, time.Now)

	out, sched := d.runQueued(fixtures.BroadcastDraftID, nil)

	assert.Equal(t, broadcasts.Outcome{}, out, "draft: unscheduled after the job was queued")
	assert.Empty(t, sched.sends)
	assert.Equal(t, broadcast.StatusDraft, d.status(fixtures.BroadcastDraftID))
	assert.Zero(t, d.recipientCount(fixtures.BroadcastDraftID))
}

func TestAQueuedPlanPlansAScheduledBroadcast(t *testing.T) {
	d := newDelivery(t, time.Now)

	out, sched := d.runQueued(fixtures.BroadcastScheduledID, nil)

	assert.Equal(t, broadcasts.Outcome{}, out)
	assert.NotEmpty(t, sched.sends)
	assert.Equal(t, broadcast.StatusSending, d.status(fixtures.BroadcastScheduledID))
}

// A delayed job is bound to the schedule it was enqueued for: after a reschedule (or unschedule
// + schedule) the superseded job must not send the Broadcast early.
func TestAQueuedPlanOfASupersededScheduleIsDoneWithNothingPlanned(t *testing.T) {
	d := newDelivery(t, time.Now)
	oldAt := d.env.DB.Broadcast.GetX(d.t.Context(), fixtures.BroadcastScheduledID).ScheduledAt
	require.NotNil(t, oldAt)
	newAt := oldAt.Add(48 * time.Hour)
	d.env.DB.Broadcast.UpdateOneID(fixtures.BroadcastScheduledID).SetScheduledAt(newAt).ExecX(d.t.Context())

	superseded, sched := d.runQueued(fixtures.BroadcastScheduledID, oldAt)

	assert.Equal(t, broadcasts.Outcome{}, superseded)
	assert.Empty(t, sched.sends, "job for the old schedule is superseded")
	assert.Equal(t, broadcast.StatusScheduled, d.status(fixtures.BroadcastScheduledID))

	current, sched := d.runQueued(fixtures.BroadcastScheduledID, &newAt)

	assert.Equal(t, broadcasts.Outcome{}, current)
	assert.NotEmpty(t, sched.sends, "job for the current schedule runs")
}

// A delayed job is also superseded by an immediate send (which clears scheduled_at), while a
// retry of the job that legitimately started the send (sending, same scheduled_at) runs.
func TestAQueuedPlanOfADelayedJobAcrossSendAndRetry(t *testing.T) {
	d := newDelivery(t, time.Now)
	at := *d.env.DB.Broadcast.GetX(d.t.Context(), fixtures.BroadcastScheduledID).ScheduledAt
	d.env.DB.Broadcast.UpdateOneID(fixtures.BroadcastScheduledID).SetStatus(broadcast.StatusSending).ExecX(d.t.Context())

	retry, sched := d.runQueued(fixtures.BroadcastScheduledID, &at)

	assert.Equal(t, broadcasts.Outcome{}, retry)
	assert.NotEmpty(t, sched.sends, "retry of the job that started the send")

	d.env.DB.Broadcast.UpdateOneID(fixtures.BroadcastScheduledID).ClearScheduledAt().ExecX(d.t.Context())
	sentNow, sched := d.runQueued(fixtures.BroadcastScheduledID, &at)

	assert.Equal(t, broadcasts.Outcome{}, sentNow)
	assert.Empty(t, sched.sends, "sent now: the delayed job is superseded")
}

// A queued plan job that fires after its Broadcast has finished is stale: it is done (so the
// queue does not retry it) and leaves the status alone.
func TestAQueuedPlanOfAFinishedBroadcastIsDone(t *testing.T) {
	d := newDelivery(t, time.Now)

	sent, sentSched := d.runQueued(fixtures.BroadcastSentID, nil)
	failed, failedSched := d.runQueued(fixtures.BroadcastFailedID, nil)

	assert.Equal(t, broadcasts.Outcome{}, sent)
	assert.Equal(t, broadcasts.Outcome{}, failed)
	assert.Empty(t, sentSched.sends)
	assert.Empty(t, failedSched.sends)
	assert.Equal(t, broadcast.StatusSent, d.status(fixtures.BroadcastSentID))
	assert.Equal(t, broadcast.StatusFailed, d.status(fixtures.BroadcastFailedID))
}

// A held plan is no failure: the job waits the hold's backoff without spending an attempt.
func TestAQueuedPlanOfAHeldBroadcastWaitsTheHoldBackoff(t *testing.T) {
	d := newDelivery(t, time.Now)
	d.env.SuspendWorkspace(d.t.Context(), fixtures.AcmeID, fixedNow)

	out, sched := d.runQueued(fixtures.BroadcastScheduledID, nil)

	assert.Equal(t, outboundmessages.HoldBackoff, out.Wait, "a hold asks again after the hold backoff")
	var held *broadcasts.HeldError
	require.ErrorAs(t, out.Err, &held)
	assert.Equal(t, holds.ReasonSuspended, held.Reason)
	assert.Empty(t, sched.sends)
}

// A fault of the plan, such as a provider that cannot be resolved, spends an attempt: it is no
// wait.
func TestAQueuedPlanFaultSpendsAnAttempt(t *testing.T) {
	d := newDelivery(t, time.Now)
	d.res.err = errors.New("no integration")

	out, _ := d.runQueued(fixtures.BroadcastScheduledID, nil)

	require.Error(t, out.Err)
	assert.Zero(t, out.Wait, "a failure is not a wait")
	assert.False(t, out.GaveUp)
}

// A failed send before the last attempt spends the attempt: the recipient stays pending for the
// next one.
func TestAFailedSendBeforeTheLastAttemptLeavesTheRecipientPending(t *testing.T) {
	d := newDelivery(t, time.Now)
	d.box.err = errors.New("smtp unavailable")
	id := d.recipient(fixtures.ContactAliceID)

	out := d.deliver(id, firstAttempt)

	require.ErrorContains(t, out.Err, "smtp unavailable")
	assert.Zero(t, out.Wait, "a failure spends an attempt: it is not a wait")
	assert.False(t, out.GaveUp)
	assert.True(t, d.pending(id))
	assert.Equal(t, broadcast.StatusSending, d.broadcastRow().Status)
}

// On the last attempt a failed send gives the recipient up with its cause, and the Broadcast
// settles instead of hanging in sending; the failure still ends the job.
func TestAFailedLastAttemptGivesTheRecipientUpAndSettlesTheBroadcast(t *testing.T) {
	d := newDelivery(t, time.Now)
	d.box.err = errors.New("smtp unavailable")
	id := d.recipient(fixtures.ContactAliceID)

	out := d.deliver(id, lastAttempt)

	require.ErrorContains(t, out.Err, "smtp unavailable")
	assert.True(t, out.GaveUp)
	assert.Zero(t, out.Wait)
	assert.False(t, d.pending(id))
	assert.Equal(t, broadcast.StatusSent, d.broadcastRow().Status)
	assert.Equal(t, broadcasts.Stats{Recipients: 1, Failed: 1}, d.stats())
	msg := d.messageOf(id)
	assert.Equal(t, outboundmessage.StatusFailed, msg.Status)
	require.NotNil(t, msg.Reason)
	assert.Contains(t, *msg.Reason, "smtp unavailable")
}

// A recipient whose send another attempt is making waits that attempt out, even on its last
// attempt: nothing is sent or failed meanwhile.
func TestARecipientWithALiveClaimWaits(t *testing.T) {
	d := newDelivery(t, time.Now)
	id := d.recipient(fixtures.ContactBobID)
	d.claim(id, 0)

	out := d.deliver(id, lastAttempt)

	require.ErrorIs(t, out.Err, outboundmessages.ErrInProgress)
	assert.Positive(t, out.Wait)
	assert.False(t, out.GaveUp)
	assert.True(t, d.pending(id))
	assert.Equal(t, outboundmessage.StatusPending, d.messageOf(id).Status)
	assert.Equal(t, broadcast.StatusSending, d.broadcastRow().Status)
	assert.Empty(t, d.box.sent)
}

// takeover fails the send after another attempt took over the claim, as one that outlived its
// lease would find: the failure cannot release a claim that is no longer its own.
type takeover struct{ db *ent.Client }

func (takeover) DefaultFrom() (address, name string) { return "hello@codebasics.dev", "CodeBasics" }

func (s takeover) Send(ctx context.Context, _ messaging.EmailMessage) (messaging.Receipt, error) {
	s.db.OutboundMessage.Update().
		Where(outboundmessage.StatusEQ(outboundmessage.StatusPending)).
		SetClaimedAt(time.Now()).ExecX(ctx)
	return messaging.Receipt{}, errors.New("smtp unavailable")
}

// On the last attempt a recipient whose claim another attempt took over is not given up: that
// attempt may yet reach the provider, so the job waits (which spends no attempt) and gives up once
// the lease is out.
func TestALastAttemptWaitsInsteadOfGivingUpOnALiveClaim(t *testing.T) {
	d := newDelivery(t, time.Now)
	d.res.sender = takeover{db: d.env.DB}
	id := d.recipient(fixtures.ContactAliceID)

	out := d.deliver(id, lastAttempt)

	require.ErrorIs(t, out.Err, outboundmessages.ErrInProgress)
	assert.Positive(t, out.Wait)
	assert.False(t, out.GaveUp)
	assert.True(t, d.pending(id), "nothing is failed while the claim is live")
	assert.Equal(t, outboundmessage.StatusPending, d.messageOf(id).Status)
	assert.Equal(t, broadcast.StatusSending, d.broadcastRow().Status)
}

// A claim past its lease is no other attempt's any more: the last attempt takes it over and, its
// send failing, gives the recipient up.
func TestALastAttemptGivesUpOverAnExpiredClaim(t *testing.T) {
	d := newDelivery(t, time.Now)
	d.box.err = errors.New("smtp unavailable")
	id := d.recipient(fixtures.ContactAliceID)
	first := d.deliver(id, firstAttempt) // leaves the recipient's message pending
	require.Error(t, first.Err)
	d.claim(id, time.Hour)

	out := d.deliver(id, lastAttempt)

	assert.True(t, out.GaveUp)
	assert.False(t, d.pending(id))
	assert.Equal(t, outboundmessage.StatusFailed, d.messageOf(id).Status)
}

// A send that reached the provider but whose job did not finish is sent, whatever the attempts
// that followed found: the recipient follows its message, and a last attempt that would fail it
// leaves it sent.
func TestALateLastAttemptAfterTheMailLeftKeepsTheRecipientSent(t *testing.T) {
	d := newDelivery(t, time.Now)
	id := d.recipient(fixtures.ContactAliceID)
	b := d.broadcastRow()
	rec := d.recipientRow(id)
	c := d.env.DB.Contact.GetX(d.t.Context(), fixtures.ContactAliceID)
	req, err := outboundmessages.BroadcastRecipient(b, rec, c)
	require.NoError(d.t, err)
	res, err := d.out.Send(d.t.Context(), d.s, req)
	require.NoError(t, err)
	require.Equal(t, outboundmessages.Sent, res.Outcome)
	d.box.err = errors.New("late failure")

	out := d.deliver(id, lastAttempt)

	assert.Equal(t, broadcasts.Outcome{}, out)
	assert.False(t, d.pending(id))
	assert.Equal(t, outboundmessage.StatusSent, d.messageOf(id).Status)
	assert.Len(t, d.box.sent, 1)
}

// A recipient that is already decided keeps its outcome: a late attempt neither fails it nor
// rewrites its message.
func TestALateLastAttemptAfterTheSendKeepsItsOutcome(t *testing.T) {
	d := newDelivery(t, time.Now)
	id := d.recipient(fixtures.ContactAliceID)
	sent := d.deliver(id, firstAttempt)
	require.Equal(t, broadcasts.Outcome{}, sent)
	d.box.err = errors.New("late")

	out := d.deliver(id, lastAttempt)

	assert.Equal(t, broadcasts.Outcome{}, out)
	assert.Equal(t, outboundmessage.StatusSent, d.messageOf(id).Status)
	assert.Equal(t, broadcasts.Stats{Recipients: 1, Sent: 1}, d.stats())
}

// A held recipient waits the hold backoff and stays pending, even on its last attempt.
func TestAHeldRecipientWaitsTheHoldBackoff(t *testing.T) {
	d := newDelivery(t, time.Now)
	id := d.recipient(fixtures.ContactAliceID)
	d.env.SuspendWorkspace(d.t.Context(), fixtures.AcmeID, fixedNow)

	out := d.deliver(id, lastAttempt)

	assert.Equal(t, outboundmessages.HoldBackoff, out.Wait)
	var held *broadcasts.HeldError
	require.ErrorAs(t, out.Err, &held)
	assert.False(t, out.GaveUp, "a hold is no failure, even on the last attempt")
	assert.True(t, d.pending(id))
}

// An erased recipient has nothing to send: its job is done.
func TestAnUnknownRecipientsJobIsDone(t *testing.T) {
	d := newDelivery(t, time.Now)

	out := d.deliver(424242, lastAttempt)

	assert.Equal(t, broadcasts.Outcome{}, out)
}
