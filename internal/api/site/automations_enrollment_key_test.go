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

// keyedTrigger is an Event Trigger keyed by the property (ADR 0165).
func keyedTrigger(action, key string) siteapi.AutomationTriggerInput {
	return siteapi.AutomationTriggerInput{OneOf: siteapi.NewEventTriggerInputAutomationTriggerInputSum(siteapi.EventTriggerInput{
		Kind: siteapi.EventTriggerInputKindEvent, Action: action, EnrollmentKey: siteapi.NewOptString(key),
	})}
}

// An Event Trigger's Enrollment key is stored as sent and read back on the version; a Trigger
// written without it removes it.
func TestSiteEnrollmentKeyRoundTrip(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	created, err := c.SiteAutomationsCreate(ctx, &siteapi.CreateAutomationInput{Name: "Post-purchase", Trigger: keyedTrigger("order.completed", "order_id")},
		siteapi.SiteAutomationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	read, err := c.SiteAutomationsGet(ctx, siteapi.SiteAutomationsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: created.ID})
	require.NoError(t, err)
	unkeyed, err := c.SiteAutomationsUpdate(ctx, &siteapi.UpdateAutomationInput{Revision: read.Revision,
		Trigger: siteapi.NewOptAutomationTriggerInput(testhelper.SiteTrigger("order.completed"))},
		siteapi.SiteAutomationsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: created.ID})
	require.NoError(t, err)

	assert.JSONEq(t, `{"kind":"event","action":"order.completed","entryPolicy":"once","cooldown":3600,"enrollmentKey":"order_id"}`,
		testhelper.JSONOf(t, &read.Draft.Value.Trigger))
	assert.JSONEq(t, `{"kind":"event","action":"order.completed","entryPolicy":"once","cooldown":3600}`,
		testhelper.JSONOf(t, &unkeyed.Draft.Value.Trigger), "a Trigger replaces its Enrollment key with it")
}

// An empty Enrollment key is no property name: the save refuses it.
func TestSiteRefusesAnEmptyEnrollmentKey(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	_, err := c.SiteAutomationsCreate(t.Context(), &siteapi.CreateAutomationInput{Name: "Keyless", Trigger: keyedTrigger("order.completed", "")},
		siteapi.SiteAutomationsCreateParams{WorkspaceId: fixtures.AcmePublicID})

	problem := testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Contains(t, problem.Detail.Value, "enrollment key")
}

// Each of a Contact's Enrollments in a keyed Automation shows its key value (fixture Order series:
// Alice's orders A and B).
func TestSiteEnrollmentsShowTheirKeyValue(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	listed, err := c.SiteEnrollmentsList(t.Context(), siteapi.SiteEnrollmentsListParams{
		WorkspaceId: fixtures.AcmePublicID, AutomationId: siteapi.EntityId(fixtures.AutomationOrderSeriesID),
	})
	require.NoError(t, err)

	keys := lo.SliceToMap(listed.Items, func(e siteapi.EnrollmentResource) (siteapi.EntityId, string) { return e.ID, e.KeyValue.Or("") })
	assert.Equal(t, map[siteapi.EntityId]string{
		siteapi.EntityId(fixtures.AutomationRunOrderSeriesAID): "A",
		siteapi.EntityId(fixtures.AutomationRunOrderSeriesBID): "B",
	}, keys)
}
