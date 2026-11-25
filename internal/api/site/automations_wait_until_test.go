package site_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const waitUntilStepID = "6c1f3a2e-8d4b-4e5f-9a7c-1b2d3e4f5a11"

// createWaitUntil creates an Automation in Acme whose root path holds the Wait until written as
// doc.
func createWaitUntil(t *testing.T, c *siteapi.Client, doc string) (*siteapi.SiteAutomationResource, error) {
	t.Helper()
	return c.SiteAutomationsCreate(t.Context(), &siteapi.CreateAutomationInput{
		Name: "Mornings", Trigger: testhelper.SiteTrigger("signup"),
		Steps: testhelper.SitePathEdit(testhelper.SiteWaitUntil(t, doc)),
	}, siteapi.SiteAutomationsCreateParams{WorkspaceId: siteapi.WorkspaceId(fixtures.AcmePublicID)})
}

// A Wait until round-trips through /site (ADR 0126): read, sent back as read and read again, it is
// the Step that was written.
func TestSiteAutomationKeepsAWaitUntilThroughASaveOfWhatWasRead(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := t.Context()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)
	written := `{"id":"` + waitUntilStepID + `","type":"wait_until",` +
		`"until":{"form":"time_of_day","localTime":"10:00","weekdays":["tuesday"]}}`
	created, err := createWaitUntil(t, c, written)
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
	require.Equal(t, siteapi.WaitUntilStepAutomationStepSum, steps[0].OneOf.Type)
	assert.JSONEq(t, written, testhelper.JSONOf(t, &steps[0].OneOf.WaitUntilStep))
}

// A local time not written HH:MM is refused by the contract (400); a repeated weekday by the save
// (422), naming the Step.
func TestSiteAutomationRefusesAMalformedWaitUntil(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	_, malformed := createWaitUntil(t, c, `{"type":"wait_until","until":{"form":"time_of_day","localTime":"9:00"}}`)
	_, repeated := createWaitUntil(t, c, `{"id":"`+waitUntilStepID+`","type":"wait_until",`+
		`"until":{"form":"time_of_day","localTime":"09:00","weekdays":["friday","friday"]}}`)

	testhelper.SiteRefusal(t, malformed, http.StatusBadRequest)
	refused := testhelper.SiteRefusal(t, repeated, http.StatusUnprocessableEntity)
	detail := refused.Detail.Or("")
	assert.Contains(t, detail, waitUntilStepID)
	assert.Contains(t, detail, "(wait_until)")
}
