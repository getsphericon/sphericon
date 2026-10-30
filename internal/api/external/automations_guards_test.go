package external_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automation"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const missingAutomation = externalapi.EntityId(999999)

// Every id-addressed automation operation answers 404 for an id that does not
// exist, and the same for another workspace's automation, which stays untouched.
func TestExternalAutomationsAreWorkspaceScoped(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "automations:read", "automations:write", "automations:publish")

	foreign := env.DB.Automation.GetX(ctx, fixtures.AutomationGlobexID)

	for name, id := range map[string]externalapi.EntityId{
		"missing": missingAutomation,
		"foreign": entityIDString(foreign.ID),
	} {
		t.Run(name, func(t *testing.T) {
			get, err := c.AutomationsGet(ctx, externalapi.AutomationsGetParams{ID: id})
			require.NoError(t, err)
			assert.IsType(t, &externalapi.AutomationsGetNotFound{}, get)

			upd, err := c.AutomationsUpdate(ctx, &externalapi.UpdateAutomationInput{Name: externalapi.NewOptString("hijack")}, externalapi.AutomationsUpdateParams{ID: id})
			require.NoError(t, err)
			assert.IsType(t, &externalapi.AutomationsUpdateNotFound{}, upd)

			act, err := c.AutomationsPublish(ctx, externalapi.AutomationsPublishParams{ID: id})
			require.NoError(t, err)
			assert.IsType(t, &externalapi.AutomationsPublishNotFound{}, act)

			deact, err := c.AutomationsDeactivate(ctx, externalapi.AutomationsDeactivateParams{ID: id})
			require.NoError(t, err)
			assert.IsType(t, &externalapi.AutomationsDeactivateNotFound{}, deact)

			del, err := c.AutomationsDelete(ctx, externalapi.AutomationsDeleteParams{ID: id})
			require.NoError(t, err)
			assert.IsType(t, &externalapi.AutomationsDeleteNotFound{}, del)
		})
	}

	stored, err := env.DB.Automation.Get(ctx, foreign.ID)
	require.NoError(t, err, "the foreign automation survives")
	assert.Equal(t, fixtures.AutomationGlobexName, stored.Name)
	assert.Equal(t, automation.StatusInactive, stored.Status, "never published across tenants")
}
