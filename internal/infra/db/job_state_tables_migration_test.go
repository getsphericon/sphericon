package db_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/integrationproviderstate"
	"github.com/getsphericon/sphericon/ent/webhookendpointhealth"
	"github.com/getsphericon/sphericon/ent/workspacestanding"
	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/webhook"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The state jobs write moves from the edited rows to tables of its own (ADR 0168): every owner gets
// its row, with what it held, and an owner that held nothing gets an empty one.
func TestTheJobStateMigrationKeepsEveryOwnersState(t *testing.T) {
	sqlDB := testhelper.SchemaBefore(t, "job_state_tables", filepath.Join("testdata", "job_state_tables"))
	_, err := db.Migrate(t.Context(), sqlDB)
	require.NoError(t, err)
	client := entclient.Core.New(sqlDB)
	ctx := t.Context()
	at := func(day int) *time.Time { return new(time.Date(2026, 1, day, 0, 0, 0, 0, time.UTC)) }
	same := func(t *testing.T, want, got *time.Time) {
		t.Helper()
		require.NotNil(t, got)
		assert.True(t, want.Equal(*got), "%v, not %v", want, got)
	}

	read := client.WorkspaceStanding.Query().Where(workspacestanding.WorkspaceID(1)).OnlyX(ctx)
	assert.EqualValues(t, 7, read.FeedPosition)
	same(t, at(2), read.SetupCompletedAt)
	same(t, at(3), read.AbuseWarningNotifiedAt)
	same(t, at(4), read.AbuseFreezeNotifiedAt)
	plain := client.WorkspaceStanding.Query().Where(workspacestanding.WorkspaceID(2)).OnlyX(ctx)
	assert.Zero(t, plain.FeedPosition)
	assert.Nil(t, plain.SetupCompletedAt)
	assert.Nil(t, plain.AbuseWarningNotifiedAt)
	assert.Nil(t, plain.AbuseFreezeNotifiedAt)

	state := client.IntegrationProviderState.Query().Where(integrationproviderstate.IntegrationID(1)).OnlyX(ctx)
	assert.Equal(t, new(14), state.ProviderMaxPerSecond)
	assert.Equal(t, new(50000), state.ProviderMaxPerDay)
	assert.Equal(t, new(providers.EnforcementProbation), state.ProviderEnforcement)
	assert.True(t, state.ProviderSandbox)
	same(t, at(5), state.ProviderRefusedAt)
	same(t, at(6), state.ProviderQuotaCheckedAt)
	same(t, at(7), state.ProviderQuotaFailedAt)
	assert.EqualValues(t, 1, state.WorkspaceID)
	unread := client.IntegrationProviderState.Query().Where(integrationproviderstate.IntegrationID(2)).OnlyX(ctx)
	assert.Nil(t, unread.ProviderMaxPerSecond)
	assert.Nil(t, unread.ProviderEnforcement)
	assert.False(t, unread.ProviderSandbox)
	assert.EqualValues(t, 2, unread.WorkspaceID)

	failing := client.WebhookEndpointHealth.Query().Where(webhookendpointhealth.WebhookEndpointID(1)).OnlyX(ctx)
	assert.Equal(t, new(7), failing.ConsecutiveFailures)
	same(t, at(8), failing.FailingSince)
	assert.Equal(t, new(webhook.KindHTTPStatus), failing.LastErrorKind)
	assert.Equal(t, new(503), failing.LastErrorStatus)
	same(t, at(9), failing.LastFailureAt)
	healthy := client.WebhookEndpointHealth.Query().Where(webhookendpointhealth.WebhookEndpointID(2)).OnlyX(ctx)
	assert.Nil(t, healthy.ConsecutiveFailures)
	assert.Nil(t, healthy.FailingSince)
	assert.EqualValues(t, 2, healthy.WorkspaceID)
}
