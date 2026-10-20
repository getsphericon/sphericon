package testhelper

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationversion"
)

// AutomationWithSteps saves the Automation that create describes together with its version 1,
// which holds steps (the executor's JSON step list), and returns the Automation. The version is
// the Automation's steps (ADR 0049): an Automation made with create alone has none and enrolls
// nobody.
func AutomationWithSteps(ctx context.Context, db *ent.Client, create *ent.AutomationCreate, steps string) (*ent.Automation, error) {
	a, err := create.Save(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := db.AutomationVersion.Create().
		SetWorkspaceID(a.WorkspaceID).SetAutomationID(a.ID).SetVersion(1).SetSteps(steps).
		Save(ctx); err != nil {
		return nil, err
	}
	return a, nil
}

// LatestVersionID is the id of the Automation's newest version.
func LatestVersionID(ctx context.Context, db *ent.Client, automationID int64) (int64, error) {
	v, err := db.AutomationVersion.Query().
		Where(automationversion.AutomationID(automationID)).
		Order(ent.Desc(automationversion.FieldVersion)).
		First(ctx)
	if err != nil {
		return 0, err
	}
	return v.ID, nil
}

// LatestSteps is the stored JSON step list of the Automation's newest version.
func LatestSteps(ctx context.Context, t *testing.T, db *ent.Client, automationID int64) string {
	t.Helper()
	v, err := db.AutomationVersion.Query().
		Where(automationversion.AutomationID(automationID)).
		Order(ent.Desc(automationversion.FieldVersion)).
		First(ctx)
	require.NoError(t, err)
	return v.Steps
}
