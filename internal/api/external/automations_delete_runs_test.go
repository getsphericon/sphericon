package external_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationrun"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Deleting an automation takes its enrollments with it: the fixture Welcome series
// has runs, and its deletion must succeed rather than trip the foreign key.
func TestExternalAutomationsDeleteRemovesItsRuns(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "automations:write")
	ctx := context.Background()

	before, err := env.DB.AutomationRun.Query().Where(automationrun.AutomationID(fixtures.AutomationWelcomeSeriesID)).Count(ctx)
	require.NoError(t, err)
	require.Positive(t, before, "the fixture automation has enrollments")

	res, err := c.AutomationsDelete(ctx, externalapi.AutomationsDeleteParams{ID: entityIDString(fixtures.AutomationWelcomeSeriesID)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.AutomationsDeleteNoContent{}, res)

	after, err := env.DB.AutomationRun.Query().Where(automationrun.AutomationID(fixtures.AutomationWelcomeSeriesID)).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, after, "its enrollments are deleted with it")
}
