package automations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestFenceRunsTheEffectOnAnActiveEnrollment(t *testing.T) {
	env := testhelper.Setup(t)
	ran := false

	err := automations.FenceOn(env.DB)(context.Background(), env.DB.Scoped(fixtures.AcmeID), fixtures.AutomationRunJobsPendingID, func(context.Context) error {
		ran = true
		return nil
	})

	require.NoError(t, err)
	assert.True(t, ran)
}

func TestFenceRunsNothingOnAnExitedEnrollment(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	ran := false

	err := automations.FenceOn(env.DB)(context.Background(), s, fixtures.AutomationRunJobsExitedOnEmailID, func(context.Context) error {
		ran = true
		return nil
	})

	require.ErrorIs(t, err, lifecycle.ErrInvalidTransition)
	assert.False(t, ran)
}
