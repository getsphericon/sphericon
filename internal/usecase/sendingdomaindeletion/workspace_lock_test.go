package sendingdomaindeletion_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/liveuse"
	"github.com/getsphericon/sphericon/internal/domain/sending/sendingdomains"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/sendingdomaindeletion"
)

// Deleting a Sending domain and scheduling a draft that sends from an address on it take the
// Workspace's lock (ADR 0099), so the delete queued behind the schedule reads the armed Broadcast
// and is refused naming it: the domain is never deleted under a Broadcast the check found a draft.
func TestAScheduleQueuedFirstRefusesTheDeleteOfTheDomainItSendsFrom(t *testing.T) {
	ctx := t.Context()
	env := testhelper.Setup(t, testhelper.Committed())
	s := env.DB.Scoped(fixtures.AcmeID)
	sends := testhelper.Module[*broadcasts.Module](env)
	domains := testhelper.Module[*sendingdomains.Module](env)
	deletion := testhelper.Module[*sendingdomaindeletion.UseCase](env)
	from := broadcasts.EmailContentPatch{FromEmail: patch.Set("news@" + fixtures.SendingDomainVerifiedDomain)}
	_, err := sends.Update(ctx, s, fixtures.BroadcastDraftID, broadcasts.UpdateInput{Revision: testhelper.BroadcastRevisionIn(ctx, t, s, fixtures.BroadcastDraftID), Content: from})
	require.NoError(t, err)
	schedule := testhelper.Writer{Name: "schedule", Run: func() error {
		_, err := sends.Schedule(ctx, s, fixtures.BroadcastDraftID, broadcasts.ScheduleInput{Revision: testhelper.BroadcastRevisionIn(ctx, t, s, fixtures.BroadcastDraftID), ScheduledAt: time.Now().Add(time.Hour)})
		return err
	}}
	remove := testhelper.Writer{Name: "delete", Run: func() error { return deletion.Delete(ctx, s, fixtures.SendingDomainVerifiedID) }}

	errs := env.QueueBehindWorkspaceLock(t, s, schedule, remove)

	require.NoError(t, errs["schedule"])
	b, err := sends.Get(ctx, s, fixtures.BroadcastDraftID)
	require.NoError(t, err)
	var used *liveuse.Error
	require.ErrorAs(t, errs["delete"], &used)
	assert.Equal(t, []liveuse.User{{Kind: liveuse.Broadcast, ID: b.ID, Name: b.Name}}, used.Users)
	kept, err := domains.Get(ctx, s, fixtures.SendingDomainVerifiedID)
	require.NoError(t, err)
	assert.Nil(t, kept.DiscardedAt, "the domain the Broadcast sends from is kept")
}
