package jobs_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An automation's apply_tag step creating a new Tag is a change made by the `system`
// actor (ADR 0022); the Contact edge itself is not an audited field change.
func TestAutomationApplyTagIsAuditedUnderSystem(t *testing.T) {
	env := testhelper.Setup(t)
	enrollVia(t, env, fixtures.AutomationJobsSystemTagID, fixtures.ContactAliceID)

	require.True(t, env.RunNextStep(t, stepperOf(env, resolvingTo(&fakeSender{}))))

	got := env.OutboxEvents(t, events.NameAuditEntry)
	require.Len(t, got, 1)
	e := got[0].(*events.AuditEntry)
	assert.Equal(t, "tag.create", e.Action)
	assert.Equal(t, events.ActorSystem, e.Actor.Kind)
	assert.Equal(t, "system-made", e.TargetName)
}
