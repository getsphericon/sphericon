package needsattentionreport_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/holdreason"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/needsattentionreport"
)

// withSendCap is the test env with Capton's Send cap set to perDay messages per 24 hours, its bucket
// full, and its two Broadcasts, each to the same three Contacts, placed to start in 2 and 3 hours
// from now (the fixtures' own times are as of when the template database was loaded).
func withSendCap(t *testing.T, perDay int) *testhelper.TestEnv {
	t.Helper()
	env := testhelper.Setup(t)
	env.DB.Workspace.UpdateOneID(fixtures.CaptonID).SetSendCap(perDay).ExecX(t.Context())
	capton := env.DB.Scoped(fixtures.CaptonID)
	now := time.Now()
	require.NoError(t, capton.Broadcast().UpdateOneID(fixtures.BroadcastCaptonSoonerID).SetScheduledAt(now.Add(2*time.Hour)).Exec(t.Context()))
	require.NoError(t, capton.Broadcast().UpdateOneID(fixtures.BroadcastCaptonLaterID).SetScheduledAt(now.Add(3*time.Hour)).Exec(t.Context()))
	return env
}

// startOf is when the Capton Broadcast id is scheduled to start.
func startOf(t *testing.T, env *testhelper.TestEnv, id int64) time.Time {
	t.Helper()
	b, err := env.DB.Scoped(fixtures.CaptonID).Broadcast().Get(t.Context(), id)
	require.NoError(t, err)
	return *b.ScheduledAt
}

// An audience of 3 under a Send cap of 2: the full bucket lets 2 out at the start, and the third
// waits for half of the bucket to refill, 12 hours at 2 per 24 hours.
func TestAScheduledBroadcastLargerThanTheSendCapIsAnItemWithItsFinish(t *testing.T) {
	env := withSendCap(t, 2)
	finish := startOf(t, env, fixtures.BroadcastCaptonSoonerID).Add(12 * time.Hour)

	items := itemsOf(t, env, fixtures.CaptonID)

	item, ok := itemAbout(items, needsattentionreport.KindScheduledBroadcastOverSendCap, fixtures.BroadcastCaptonSoonerID)
	require.True(t, ok)
	assert.Equal(t, needsattentionreport.SeverityDegraded, item.Severity)
	assert.Equal(t, fixtures.BroadcastCaptonSoonerName, item.Subject.Name)
	assert.Equal(t, 3, item.Count, "the estimated audience")
	require.NotNil(t, item.EstimatedCompletionAt)
	assert.True(t, finish.Equal(*item.EstimatedCompletionAt), "expected to finish at %v, got %v", finish, *item.EstimatedCompletionAt)
}

// Under a Send cap of 6 both Broadcasts, 6 Contacts in all, go out from the full bucket.
func TestScheduledBroadcastsTheSendCapLetsOutAreNoItem(t *testing.T) {
	env := withSendCap(t, 6)

	items := itemsOf(t, env, fixtures.CaptonID)

	assert.Empty(t, items)
}

// Under a Send cap of 4 each Broadcast fits alone, but they draw from the same bucket: the later one
// waits for the earlier one's audience too, 2 of 6 beyond the bucket, 12 hours at 4 per 24 hours.
func TestScheduledBroadcastsAreJudgedTogetherInTheOrderTheyStart(t *testing.T) {
	env := withSendCap(t, 4)
	finish := startOf(t, env, fixtures.BroadcastCaptonLaterID).Add(12 * time.Hour)

	items := itemsOf(t, env, fixtures.CaptonID)

	require.Len(t, items, 1)
	later := items[0]
	assert.Equal(t, needsattentionreport.KindScheduledBroadcastOverSendCap, later.Kind)
	assert.Equal(t, int64(fixtures.BroadcastCaptonLaterID), later.Subject.ID)
	assert.Equal(t, 3, later.Count, "its own audience, not the sum")
	require.NotNil(t, later.EstimatedCompletionAt)
	assert.True(t, finish.Equal(*later.EstimatedCompletionAt), "expected to finish at %v, got %v", finish, *later.EstimatedCompletionAt)
}

// A Broadcast already sending draws its remaining recipients from the bucket before any that has yet
// to start: under a Send cap of 6, its 3 and the sooner one's 3 fill the bucket, and the later one's 3
// wait for half of it to refill, 12 hours at 6 per 24 hours.
func TestTheRecipientsLeftToASendingBroadcastComeBeforeTheScheduledOnes(t *testing.T) {
	env := withSendCap(t, 6)
	lift := env.DB.Scoped(fixtures.CaptonID).Broadcast().UpdateOneID(fixtures.BroadcastCaptonSendingID).ClearHoldReason()
	require.NoError(t, lift.Exec(t.Context()))
	finish := startOf(t, env, fixtures.BroadcastCaptonLaterID).Add(12 * time.Hour)

	items := itemsOf(t, env, fixtures.CaptonID)

	require.Len(t, items, 1)
	later := items[0]
	assert.Equal(t, int64(fixtures.BroadcastCaptonLaterID), later.Subject.ID)
	require.NotNil(t, later.EstimatedCompletionAt)
	assert.True(t, finish.Equal(*later.EstimatedCompletionAt), "expected to finish at %v, got %v", finish, *later.EstimatedCompletionAt)
}

func TestABroadcastScheduledAfterTheNext24HoursIsNoItem(t *testing.T) {
	env := withSendCap(t, 2)
	capton := env.DB.Scoped(fixtures.CaptonID)
	later := time.Now().Add(30 * time.Hour)
	require.NoError(t, capton.Broadcast().UpdateOneID(fixtures.BroadcastCaptonSoonerID).SetScheduledAt(later).Exec(t.Context()))
	require.NoError(t, capton.Broadcast().UpdateOneID(fixtures.BroadcastCaptonLaterID).SetScheduledAt(later).Exec(t.Context()))

	items := itemsOf(t, env, fixtures.CaptonID)

	assert.Empty(t, items)
}

// A held Broadcast is a blocked item of its own and takes nothing from the bucket while it is held:
// the later one, alone, fits a Send cap of 4.
func TestAHeldScheduledBroadcastIsNoSendCapItemAndTakesNothingFromTheOthers(t *testing.T) {
	env := withSendCap(t, 4)
	hold := env.DB.Scoped(fixtures.CaptonID).Broadcast().UpdateOneID(fixtures.BroadcastCaptonSoonerID).SetHoldReason(holdreason.NoIntegration)
	require.NoError(t, hold.Exec(t.Context()))

	items := itemsOf(t, env, fixtures.CaptonID)

	assert.Equal(t, []needsattentionreport.Kind{needsattentionreport.KindBroadcastHeld}, kinds(items))
}

func TestWithoutASendCapNoScheduledBroadcastIsAnItem(t *testing.T) {
	env := testhelper.Setup(t)

	items := itemsOf(t, env, fixtures.CaptonID)

	assert.Empty(t, items)
}
