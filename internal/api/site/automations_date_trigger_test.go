package site_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A date Trigger (ADR 0126) round-trips through /site: written on create and read back on the
// draft, by its kind.
func TestSiteDateTriggerRoundTrips(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	trigger := `{"kind":"date","field":{"kind":"birthday"},"recurrence":"yearly","offsetDays":0,"localTime":"10:00"}`

	created, err := c.SiteAutomationsCreate(ctx, &siteapi.CreateAutomationInput{Name: "Birthday",
		Trigger: testhelper.ConditionInput[siteapi.AutomationTriggerInput](t, trigger)},
		siteapi.SiteAutomationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)

	assert.JSONEq(t, trigger, testhelper.JSONOf(t, &created.Draft.Value.Trigger))
}
