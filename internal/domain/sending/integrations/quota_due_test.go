package integrations_test

import (
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestIntegrationsDueForQuotaRefreshAreTheSESOnes(t *testing.T) {
	env := testhelper.Setup(t)
	due, err := integrations.DueForQuotaRefresh(t.Context(), env.DB, 100)
	require.NoError(t, err)
	ids := lo.Map(due, func(i *ent.Integration, _ int) int64 { return i.ID })
	assert.Contains(t, ids, int64(fixtures.IntegrationAcmeSesID))
	assert.NotContains(t, ids, int64(fixtures.IntegrationAcmeDefaultID))
}

// A deleted Integration is not kept working (ADR 0099): its credential sends nothing, so the
// provider is not asked for its quota.
func TestIntegrationsDueForQuotaRefreshLeaveADeletedIntegrationOut(t *testing.T) {
	env := testhelper.Setup(t)
	env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeSesID).SetDiscardedAt(time.Now()).ExecX(t.Context())

	due, err := integrations.DueForQuotaRefresh(t.Context(), env.DB, 100)

	require.NoError(t, err)
	ids := lo.Map(due, func(i *ent.Integration, _ int) int64 { return i.ID })
	assert.NotContains(t, ids, int64(fixtures.IntegrationAcmeSesID))
	assert.Contains(t, ids, int64(fixtures.IntegrationAcmeSesImpliedID))
}
