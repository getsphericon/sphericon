package site_test

import (
	"maps"
	"net/http"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A Wait for event round-trips through /site (ADR 0124): read, sent back as read and read again, it
// is the Step that was written, its name kept since it keeps its id.
func TestSiteAutomationKeepsAWaitForEventThroughASaveOfWhatWasRead(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := t.Context()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)
	written := `{"id":"7d2a4b3c-9e5f-4a6b-8c7d-2e3f4a5b6c11","type":"wait_for_event","name":"purchase","timeoutSeconds":259200,` +
		`"awaits":{"kind":"event","matchers":[{"action":"order.created","filter":{"kind":"group","combinator":"and","entries":[` +
		`{"kind":"property","key":"total","test":{"kind":"number","operator":"gt","value":100}}]}}],` +
		`"condition":{"kind":"group","combinator":"and","entries":[{"kind":"tag","name":"vip","operator":"has"}]}},` +
		`"arrived":{"id":"7d2a4b3c-9e5f-4a6b-8c7d-2e3f4a5b6c12","steps":[{"id":"7d2a4b3c-9e5f-4a6b-8c7d-2e3f4a5b6c13","type":"apply_tag","tag":"buyer"}]},` +
		`"timedOut":{"id":"7d2a4b3c-9e5f-4a6b-8c7d-2e3f4a5b6c14","steps":[]}}`
	created, err := c.SiteAutomationsCreate(ctx, &siteapi.CreateAutomationInput{
		Name: "Waits for a purchase", Trigger: testhelper.SiteTrigger("signup"),
		Steps: testhelper.SitePathEdit(testhelper.SiteWaitForEvent(t, written)),
	}, siteapi.SiteAutomationsCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)

	_, err = c.SiteAutomationsUpdate(ctx, &siteapi.UpdateAutomationInput{
		Revision: created.Revision,
		Steps:    siteapi.NewOptAutomationPathInput(testhelper.Resend[siteapi.AutomationPathInput](t, &created.Draft.Value.Steps)),
	}, siteapi.SiteAutomationsUpdateParams{WorkspaceId: workspaceID, ID: created.ID})
	require.NoError(t, err)
	got, err := c.SiteAutomationsGet(ctx, siteapi.SiteAutomationsGetParams{WorkspaceId: workspaceID, ID: created.ID})
	require.NoError(t, err)

	steps := got.Draft.Value.Steps.Steps
	require.Len(t, steps, 1)
	require.Equal(t, siteapi.WaitForEventStepAutomationStepSum, steps[0].OneOf.Type)
	assert.JSONEq(t, written, testhelper.JSONOf(t, &steps[0].OneOf.WaitForEventStep))
}

// A Wait for event's Condition holding the test of the Enrollment's own message, a Branch's alone
// (ADR 0181), is refused by /site with a 400 naming its path.
func TestSiteAutomationRefusesAWaitForEventConditionTestingTheEnrollmentsOwnMessage(t *testing.T) {
	env := testhelper.Setup(t)
	edited := env.SiteActorVia(t, fixtures.OwnerJohnEmail, testhelper.EditBody(func(body map[string]any) {
		step := body["steps"].(map[string]any)["steps"].([]any)[0].(map[string]any)
		step["awaits"].(map[string]any)["condition"] = testhelper.NestedCondition(t,
			`{"kind":"stepMessage","step":"7d2a4b3c-9e5f-4a6b-8c7d-2e3f4a5b6c13","engagement":"opened"}`)
	}))
	wait := `{"type":"wait_for_event","name":"purchase","timeoutSeconds":60,"awaits":{"kind":"event","matchers":[{"action":"order.created"}]},` +
		`"arrived":{"steps":[]},"timedOut":{"steps":[]}}`

	_, err := edited.SiteAutomationsCreate(t.Context(), &siteapi.CreateAutomationInput{
		Name: "Waits for a purchase", Trigger: testhelper.SiteTrigger("signup"),
		Steps: testhelper.SitePathEdit(testhelper.SiteWaitForEvent(t, wait)),
	}, siteapi.SiteAutomationsCreateParams{WorkspaceId: siteapi.WorkspaceId(fixtures.AcmePublicID)})

	refused := testhelper.SiteRefusal(t, err, http.StatusBadRequest)
	assert.Equal(t, []string{"steps.steps.0.awaits.condition.entries.1.entries.1"}, slices.Collect(maps.Keys(refused.Errors.Or(nil))))
}
