package broadcasts_test

import (
	"testing"

	entgo "entgo.io/ent"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/schema"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/holds"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The Broadcast's hold reason declares exactly the reasons Outbound send holds a send for: the
// Workspace-level ones of the holds package and the source-level ones of Outbound send.
func TestTheBroadcastHoldReasonIsEveryReasonASendIsHeldFor(t *testing.T) {
	field, ok := lo.Find(schema.Broadcast{}.Fields(), func(f entgo.Field) bool { return f.Descriptor().Name == broadcast.FieldHoldReason })
	require.True(t, ok)
	declared := lo.Map(field.Descriptor().Enums, func(e struct{ N, V string }, _ int) string { return e.V })

	assert.ElementsMatch(t, []string{
		holds.ReasonSuspended, holds.ReasonPendingDeletion, holds.ReasonStaff,
		outbound.HoldNoIntegration, outbound.HoldUnverifiedDomain, outbound.HoldProviderStopped,
	}, declared)
}

func TestABroadcastHoldReasonOutsideTheDeclaredOnesIsRefused(t *testing.T) {
	env := testhelper.Setup(t)

	err := env.DB.Broadcast.UpdateOneID(fixtures.BroadcastSendingID).SetHoldReason("kept").Exec(t.Context())

	require.Error(t, err)
	assert.True(t, ent.IsValidationError(err), "got %v", err)
}

func TestTheDatabaseRefusesAFailedBroadcastWithoutWhenItFailed(t *testing.T) {
	env := testhelper.Setup(t)

	err := env.DB.Broadcast.UpdateOneID(fixtures.BroadcastSendingID).SetStatus(broadcast.StatusFailed).Exec(t.Context())

	require.Error(t, err)
	assert.True(t, ent.IsConstraintError(err), "got %v", err)
}
