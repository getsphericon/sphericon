package external_test

import (
	"net/http"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An Event Trigger's Entry policy and cooldown round-trip through /api: written on create, read
// back on the version, and a cooldown under the contract's hour is a 400 (ADR 0127).
func TestExternalEntryPolicyAndCooldownRoundTrip(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	c := env.ExternalScoped(t, apitokens.ScopeAutomationsRead, apitokens.ScopeAutomationsWrite)
	trigger := func(cooldown int32) externalapi.AutomationTriggerInput {
		return externalapi.AutomationTriggerInput{OneOf: externalapi.NewEventTriggerInputAutomationTriggerInputSum(externalapi.EventTriggerInput{
			Kind: externalapi.EventTriggerInputKindEvent, Action: "order.completed",
			EntryPolicy: externalapi.NewOptEventTriggerInputEntryPolicy(externalapi.EventTriggerInputEntryPolicyAgain),
			Cooldown:    externalapi.NewOptInt32(cooldown),
		})}
	}

	created, err := c.AutomationsCreate(ctx, &externalapi.CreateAutomationInput{Name: "Post-purchase", Trigger: trigger(30 * 24 * 3600)})
	require.NoError(t, err)
	read, err := c.AutomationsGet(ctx, externalapi.AutomationsGetParams{ID: created.ID})
	require.NoError(t, err)
	_, refused := c.AutomationsCreate(ctx, &externalapi.CreateAutomationInput{Name: "Too often", Trigger: trigger(60)})

	assert.JSONEq(t, `{"kind":"event","action":"order.completed","entryPolicy":"again","cooldown":2592000}`,
		testhelper.JSONOf(t, &read.Draft.Value.Trigger))
	problem := testhelper.ExternalRefusal(t, refused, http.StatusBadRequest)
	assert.Contains(t, problem.Detail.Value, "cooldown")
}

// A Contact's Enrollments in an Automation that enrolled her twice are each listed with their
// ordinal, and one is read with its own (fixture Repeat purchase: Alice's two).
func TestExternalEnrollmentsShowTheirOrdinal(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	c := env.ExternalScoped(t, apitokens.ScopeEnrollmentsRead)

	listed, err := c.EnrollmentsList(ctx, externalapi.EnrollmentsListParams{AutomationId: entityIDString(fixtures.AutomationRepeatPurchaseID)})
	require.NoError(t, err)
	second, err := c.EnrollmentsGet(ctx, externalapi.EnrollmentsGetParams{
		AutomationId: entityIDString(fixtures.AutomationRepeatPurchaseID),
		EnrollmentId: entityIDString(fixtures.AutomationRunRepeatPurchaseSecondID),
	})
	require.NoError(t, err)

	ordinals := lo.SliceToMap(listed.Items, func(e externalapi.EnrollmentResource) (externalapi.EntityId, int32) { return e.ID, e.Ordinal })
	assert.Equal(t, map[externalapi.EntityId]int32{
		entityIDString(fixtures.AutomationRunRepeatPurchaseFirstID):  1,
		entityIDString(fixtures.AutomationRunRepeatPurchaseSecondID): 2,
	}, ordinals)
	assert.EqualValues(t, 2, second.Ordinal)
	assert.Equal(t, externalapi.EntityId(fixtures.ContactAliceID), second.ContactId)
}
