package testhelper

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationversion"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
)

// LatestSteps are the stored steps of the Automation's newest version.
func LatestSteps(ctx context.Context, t *testing.T, db *ent.Client, automationID int64) automationstep.Path {
	t.Helper()
	v, err := db.AutomationVersion.Query().
		Where(automationversion.AutomationID(automationID)).
		Order(ent.Desc(automationversion.FieldVersion)).
		First(ctx)
	require.NoError(t, err)
	return v.Steps
}

// StepIDAt is the id of the Step at pos of the version the Enrollment runID began on, 0 for the
// first; "" past its last Step.
func (env *TestEnv) StepIDAt(t *testing.T, runID int64, pos int) automationstep.ID {
	t.Helper()
	run, err := env.DB.AutomationRun.Get(t.Context(), runID)
	require.NoError(t, err)
	v, err := env.DB.AutomationVersion.Get(t.Context(), run.VersionID)
	require.NoError(t, err)
	for i, st := range automationstep.All(v.Steps) {
		if i == pos {
			return st.ID
		}
	}
	return ""
}

// CurrentStepID is the id of the Step the Enrollment runID stands on, "" for none (a version of no
// Steps).
func (env *TestEnv) CurrentStepID(t *testing.T, runID int64) automationstep.ID {
	t.Helper()
	run, err := env.DB.AutomationRun.Get(t.Context(), runID)
	require.NoError(t, err)
	return lo.FromPtr(run.CurrentStepID)
}
