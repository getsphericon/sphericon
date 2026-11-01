package automations_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func enrolled(t *testing.T, env *testhelper.TestEnv) int64 {
	t.Helper()
	runID, ok, err := automations.Enroll(context.Background(), env.DB.Scoped(fixtures.AcmeID), env.Publisher(), fixtures.AutomationHoldDemoID, fixtures.ContactAliceID)
	require.NoError(t, err)
	require.True(t, ok)
	return runID
}

func TestFenceRunsTheEffectOnAnActiveEnrollment(t *testing.T) {
	env := testhelper.Setup(t)
	runID := enrolled(t, env)
	ran := false

	err := automations.FenceOn(env.DB)(context.Background(), env.DB.Scoped(fixtures.AcmeID), runID, func(context.Context) error {
		ran = true
		return nil
	})

	require.NoError(t, err)
	assert.True(t, ran)
}

func TestFenceRunsNothingOnAnExitedEnrollment(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	runID := enrolled(t, env)
	require.NoError(t, automations.ExitRun(context.Background(), s, env.Publisher(), runID, time.Now()))
	ran := false

	err := automations.FenceOn(env.DB)(context.Background(), s, runID, func(context.Context) error {
		ran = true
		return nil
	})

	require.True(t, automations.Stale(err))
	assert.False(t, ran)
}
