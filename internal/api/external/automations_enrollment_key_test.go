package external_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An Event Trigger's Enrollment key round-trips through /api, and an Enrollment of a keyed
// Automation is read with its key value (ADR 0165; fixture Order series: Alice's order B).
func TestExternalEnrollmentKeyAndKeyValueRoundTrip(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	c := env.ExternalScoped(t, apitokens.ScopeAutomationsRead, apitokens.ScopeAutomationsWrite, apitokens.ScopeEnrollmentsRead)
	trigger := externalapi.AutomationTriggerInput{OneOf: externalapi.NewEventTriggerInputAutomationTriggerInputSum(externalapi.EventTriggerInput{
		Kind: externalapi.EventTriggerInputKindEvent, Action: "order.completed", EnrollmentKey: externalapi.NewOptString("order_id"),
	})}

	created, err := c.AutomationsCreate(ctx, &externalapi.CreateAutomationInput{Name: "Post-purchase", Trigger: trigger})
	require.NoError(t, err)
	read, err := c.AutomationsGet(ctx, externalapi.AutomationsGetParams{ID: created.ID})
	require.NoError(t, err)
	b, err := c.EnrollmentsGet(ctx, externalapi.EnrollmentsGetParams{
		AutomationId: entityIDString(fixtures.AutomationOrderSeriesID),
		EnrollmentId: entityIDString(fixtures.AutomationRunOrderSeriesBID),
	})
	require.NoError(t, err)

	assert.JSONEq(t, `{"kind":"event","action":"order.completed","entryPolicy":"once","cooldown":3600,"enrollmentKey":"order_id"}`,
		testhelper.JSONOf(t, &read.Draft.Value.Trigger))
	assert.Equal(t, externalapi.NewOptString("B"), b.KeyValue)
}
