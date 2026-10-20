package jobs_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An automation's apply_tag step creating a new Tag is a change made by the `system`
// actor (ADR 0022); the Contact edge itself is not an audited field change.
func TestAutomationApplyTagIsAuditedUnderSystem(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	c, err := env.DB.Contact.Create().SetWorkspaceID(fixtures.AcmeID).SetEmail("sys@test.dev").Save(ctx)
	require.NoError(t, err)
	_, err = env.DB.Automation.Create().SetWorkspaceID(fixtures.AcmeID).
		SetName("Tagger").SetTriggerEvent("contact.created").SetStatus(automation.StatusActive).
		SetDefinition(`[{"type":"apply_tag","tag":"system-made"}]`).Save(ctx)
	require.NoError(t, err)
	runIDs, err := jobs.EvaluateTrigger(ctx, env.DB.Scoped(fixtures.AcmeID), c.ID, "contact.created")
	require.NoError(t, err)
	require.Len(t, runIDs, 1)

	_, err = jobs.RunStep(ctx, env.DB, noFence, newMod(env, resolvingTo(&fakeSender{})), runIDs[0])
	require.NoError(t, err)

	got := env.OutboxEvents(t, events.NameAuditEntry)
	require.Len(t, got, 1)
	e := got[0].(*events.AuditEntry)
	assert.Equal(t, "tag.create", e.Action)
	assert.Equal(t, events.ActorSystem, e.Actor.Kind)
	assert.Equal(t, "system-made", e.TargetName)
}
