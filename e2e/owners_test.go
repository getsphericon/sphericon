//go:build e2e

package e2e

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/accounts"
)

// Two owners changed at the same moment: without the row lock both would count two
// owners and succeed, leaving the Workspace ownerless. These cannot run under the
// transaction-per-test harness, where both would share one transaction.
func TestConcurrentOwnerDemotionsKeepOneOwner(t *testing.T) {
	t.Parallel()
	w, owners := workspaceWithTwoOwners(t)
	assertOneRefused(t, w, w.DemoteConcurrently(owners...))
}

func TestConcurrentOwnerRemovalsKeepOneOwner(t *testing.T) {
	t.Parallel()
	w, owners := workspaceWithTwoOwners(t)
	assertOneRefused(t, w, w.RemoveConcurrently(owners...))
}

func TestConcurrentOwnerDemotionAndRemovalKeepOneOwner(t *testing.T) {
	t.Parallel()
	w, owners := workspaceWithTwoOwners(t)
	assertOneRefused(t, w, w.DemoteAndRemoveConcurrently(owners[0], owners[1]))
}

func workspaceWithTwoOwners(t *testing.T) (w *Workspace, ownerIDs []int64) {
	t.Helper()
	w = env.NewWorkspace(t)
	owners := w.AddOwner()
	require.Len(t, owners, 2)
	return w, owners
}

func assertOneRefused(t *testing.T, w *Workspace, errs []error) {
	t.Helper()
	failed := 0
	for _, err := range errs {
		if err != nil {
			require.ErrorIs(t, err, accounts.ErrLastOwner)
			failed++
		}
	}
	assert.Equal(t, 1, failed, "exactly one change is refused")
	assert.Len(t, w.OwnerMembershipIDs(), 1, "the Workspace keeps an owner")
}
