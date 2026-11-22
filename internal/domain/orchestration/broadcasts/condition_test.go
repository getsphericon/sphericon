package broadcasts_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Scheduling a draft copies its Segment's Condition (ADR 0065): a Broadcast to all Contacts has
// none, and one to a Segment with no tests holds the empty group, which matches every Contact
// (ADR 0169).
func TestAnArmedBroadcastHoldsNoConditionForAllContactsAndTheEmptyGroupForASegmentWithNoTests(t *testing.T) {
	env := testhelper.Setup(t)
	m := testhelper.Module[*broadcasts.Module](env)
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	schedule := func(id int64) {
		t.Helper()
		_, err := m.Schedule(ctx, acme, id, broadcasts.ScheduleInput{Revision: testhelper.BroadcastRevisionIn(ctx, t, acme, id), ScheduledAt: time.Now().Add(time.Hour)})
		require.NoError(t, err)
	}

	require.Nil(t, env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID).SegmentID, "the fixture goes to all Contacts")
	schedule(fixtures.BroadcastDraftID)
	assert.Nil(t, env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID).Condition)

	_, err := m.Unschedule(ctx, acme, fixtures.BroadcastDraftID)
	require.NoError(t, err)
	env.DB.Broadcast.UpdateOneID(fixtures.BroadcastDraftID).SetSegmentID(fixtures.SegmentNewsletterID).ExecX(ctx)
	schedule(fixtures.BroadcastDraftID)
	assert.Equal(t, &conditiontree.Segment{Group: conditiontree.Empty()}, env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID).Condition)
}
