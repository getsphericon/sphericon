package testhelper

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationversion"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
)

// LatestSteps are the stored steps of the Automation's newest version.
func LatestSteps(ctx context.Context, t *testing.T, db *ent.Client, automationID int64) []automationstep.Step {
	t.Helper()
	v, err := db.AutomationVersion.Query().
		Where(automationversion.AutomationID(automationID)).
		Order(ent.Desc(automationversion.FieldVersion)).
		First(ctx)
	require.NoError(t, err)
	return v.Steps
}
