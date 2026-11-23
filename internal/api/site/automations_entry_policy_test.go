package site_test

import (
	"net/http"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// againTrigger is an Event Trigger under `again` with the cooldown in seconds (ADR 0127).
func againTrigger(action string, cooldown int32) siteapi.AutomationTriggerInput {
	return siteapi.AutomationTriggerInput{OneOf: siteapi.NewEventTriggerInputAutomationTriggerInputSum(siteapi.EventTriggerInput{
		Kind: siteapi.EventTriggerInputKindEvent, Action: action,
		EntryPolicy: siteapi.NewOptEventTriggerInputEntryPolicy(siteapi.EventTriggerInputEntryPolicyAgain),
		Cooldown:    siteapi.NewOptInt32(cooldown),
	})}
}

// An Event Trigger's Entry policy and cooldown are stored as sent and read back on the version, on
// create and on update; a Trigger written without them takes `once` and an hour.
func TestSiteEntryPolicyAndCooldownRoundTrip(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	created, err := c.SiteAutomationsCreate(ctx, &siteapi.CreateAutomationInput{Name: "Post-purchase", Trigger: againTrigger("order.completed", 7200)},
		siteapi.SiteAutomationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	read, err := c.SiteAutomationsGet(ctx, siteapi.SiteAutomationsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: created.ID})
	require.NoError(t, err)
	reset, err := c.SiteAutomationsUpdate(ctx, &siteapi.UpdateAutomationInput{Revision: read.Revision,
		Trigger: siteapi.NewOptAutomationTriggerInput(testhelper.SiteTrigger("order.completed"))},
		siteapi.SiteAutomationsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: created.ID})
	require.NoError(t, err)

	want := `{"kind":"event","action":"order.completed","entryPolicy":"again","cooldown":7200}`
	assert.JSONEq(t, want, testhelper.JSONOf(t, &read.Draft.Value.Trigger))
	assert.JSONEq(t, `{"kind":"event","action":"order.completed","entryPolicy":"once","cooldown":3600}`,
		testhelper.JSONOf(t, &reset.Draft.Value.Trigger), "a Trigger replaces its Entry policy with it")
}

// A cooldown under an hour is refused on save: the contract bounds it, so the body is a 400 naming
// the field.
func TestSiteRefusesACooldownUnderAnHour(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	_, err := c.SiteAutomationsCreate(t.Context(), &siteapi.CreateAutomationInput{Name: "Too often", Trigger: againTrigger("order.completed", 1800)},
		siteapi.SiteAutomationsCreateParams{WorkspaceId: fixtures.AcmePublicID})

	problem := testhelper.SiteRefusal(t, err, http.StatusBadRequest)
	assert.Contains(t, problem.Detail.Value, "cooldown")
}

// A Contact's Enrollments in an Automation that enrolled her twice are each listed with their
// ordinal (fixture Repeat purchase: Alice's two).
func TestSiteEnrollmentsShowTheirOrdinal(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	listed, err := c.SiteEnrollmentsList(t.Context(), siteapi.SiteEnrollmentsListParams{
		WorkspaceId: fixtures.AcmePublicID, AutomationId: siteapi.EntityId(fixtures.AutomationRepeatPurchaseID),
	})
	require.NoError(t, err)

	ordinals := lo.SliceToMap(listed.Items, func(e siteapi.EnrollmentResource) (siteapi.EntityId, int32) { return e.ID, e.Ordinal })
	assert.Equal(t, map[siteapi.EntityId]int32{
		siteapi.EntityId(fixtures.AutomationRunRepeatPurchaseFirstID):  1,
		siteapi.EntityId(fixtures.AutomationRunRepeatPurchaseSecondID): 2,
	}, ordinals)
}
