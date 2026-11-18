package auditlogretention_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/domain/insights/auditentries"
	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ee/usecase/auditlogretention"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/auditentry"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The fixtures hold one entry per Workspace, on 2026-01-01 (Acme) and 2026-01-02 (Globex).
var febFirst = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

// retention is the use case under lic, on the clock now, over the env's Audit log.
func retention(env *testhelper.TestEnv, lic *licensekey.License, now func() time.Time) *auditlogretention.UseCase {
	return auditlogretention.New(env.DB, auditentries.NewLog(env.DB, lic, env.Bus), lic, now)
}

// pruneEach is what the job's fan-out does with Prune: one run per Workspace Due lists, summed.
func pruneEach(t *testing.T, u *auditlogretention.UseCase) int {
	t.Helper()
	due, err := u.Due(t.Context())
	require.NoError(t, err)
	var total int
	for _, ws := range due {
		n, err := u.Prune(t.Context(), ws)
		require.NoError(t, err)
		total += n
	}
	return total
}

func countEntries(t *testing.T, env *testhelper.TestEnv, ws int64) int {
	t.Helper()
	n, err := env.DB.Scoped(ws).AuditEntry().Query().Count(t.Context())
	require.NoError(t, err)
	return n
}

func TestPruneRemovesEntriesOlderThanTheWorkspaceRetention(t *testing.T) {
	env := testhelper.Setup(t)
	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetRetentionDays(7).ExecX(t.Context())
	lic := testhelper.License(t, licensekey.FeatureRetention)

	u := retention(env, lic, func() time.Time { return febFirst })

	acme, err := u.Prune(t.Context(), fixtures.AcmeID)
	require.NoError(t, err)
	globex, err := u.Prune(t.Context(), fixtures.GlobexID)
	require.NoError(t, err)

	assert.Equal(t, 1, acme)
	assert.Zero(t, countEntries(t, env, fixtures.AcmeID))
	assert.Zero(t, globex)
	assert.Equal(t, 5, countEntries(t, env, fixtures.GlobexID), "a Workspace with no retention keeps its log")
}

func TestPruneKeepsEntriesInsideTheWindow(t *testing.T) {
	env := testhelper.Setup(t)
	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetRetentionDays(60).ExecX(t.Context())

	n, err := retention(env, testhelper.License(t, licensekey.FeatureRetention), func() time.Time { return febFirst }).Prune(t.Context(), fixtures.AcmeID)

	require.NoError(t, err)
	assert.Zero(t, n)
	assert.Equal(t, 1, countEntries(t, env, fixtures.AcmeID))
}

func TestPrunesNothingWithoutTheRetentionLicense(t *testing.T) {
	env := testhelper.Setup(t)
	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetRetentionDays(7).ExecX(t.Context())

	for _, lic := range []*licensekey.License{nil, testhelper.License(t), testhelper.License(t, licensekey.FeatureAudit)} {
		assert.Zero(t, pruneEach(t, retention(env, lic, func() time.Time { return febFirst })))
	}
	assert.Equal(t, 1, countEntries(t, env, fixtures.AcmeID))
}

func TestPruneSkipsAWorkspaceInItsDeletionWindow(t *testing.T) {
	env := testhelper.Setup(t)
	env.DB.Workspace.UpdateOneID(fixtures.BluthID).SetRetentionDays(7).ExecX(t.Context())
	before := countEntries(t, env, fixtures.BluthID)
	require.Positive(t, before, "BluthAuditEntry is older than the window")

	n, err := retention(env, testhelper.License(t, licensekey.FeatureRetention), func() time.Time { return febFirst }).Prune(t.Context(), fixtures.BluthID)

	require.NoError(t, err)
	assert.Zero(t, n)
	assert.Equal(t, before, countEntries(t, env, fixtures.BluthID))
}

// An entry older than the client window loses the client's address and user agent and keeps the
// rest; a younger one keeps them. It needs no license: the entries were written under one.
func TestForgetClientsClearsTheAddressOfEntriesOlderThanTheWindow(t *testing.T) {
	env := testhelper.Setup(t)
	// 2026-06-01 less 90 days is 2026-03-03: the February and March entries are older.
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	n, err := retention(env, nil, func() time.Time { return now }).ForgetClients(t.Context(), fixtures.GlobexID)

	require.NoError(t, err)
	assert.Equal(t, 2, n)
	log := env.DB.Scoped(fixtures.GlobexID).AuditEntry()
	old := log.Query().Where(auditentry.ID(fixtures.GlobexAuditIntegrationUpdateID)).OnlyX(t.Context())
	assert.Nil(t, old.IP)
	assert.Equal(t, "req-a", *old.RequestID, "the rest of the entry stays")
	young := log.Query().Where(auditentry.ID(fixtures.GlobexAuditIntegrationDeleteID)).OnlyX(t.Context())
	assert.Equal(t, "10.0.0.1", *young.IP)
}

// The job fans out over every Workspace: client details are cleared without the license and in a
// Workspace in its deletion window too, so none is left out.
func TestDueListsEveryWorkspace(t *testing.T) {
	env := testhelper.Setup(t)

	due, err := retention(env, nil, time.Now).Due(t.Context())

	require.NoError(t, err)
	assert.Subset(t, due, []int64{fixtures.AcmeID, fixtures.GlobexID, fixtures.BluthID, fixtures.MonarchID},
		"a Workspace without a window, one in its deletion window and a suspended one too")
}

// A Prune that fails does not keep the same Workspace's client details from being cleared: the
// sweep of a Workspace runs both and reports the failure.
func TestASweepWhosePruneFailsStillForgetsTheClients(t *testing.T) {
	env := testhelper.Setup(t)
	env.DB.Workspace.UpdateOneID(fixtures.GlobexID).SetRetentionDays(7).ExecX(t.Context())
	boom := errors.New("boom")
	env.DB.AuditEntry.Use(failing(ent.OpDelete|ent.OpDeleteOne, boom))
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	err := retention(env, testhelper.License(t, licensekey.FeatureRetention), func() time.Time { return now }).Sweep(t.Context(), fixtures.GlobexID)

	require.ErrorIs(t, err, boom)
	old := env.DB.Scoped(fixtures.GlobexID).AuditEntry().Query().Where(auditentry.ID(fixtures.GlobexAuditIntegrationUpdateID)).OnlyX(t.Context())
	assert.Nil(t, old.IP, "the entry the prune failed to remove still loses its client")
}

// A ForgetClients that fails is reported by the sweep after its Prune did its work.
func TestASweepWhoseForgetClientsFailsStillPrunes(t *testing.T) {
	env := testhelper.Setup(t)
	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetRetentionDays(7).ExecX(t.Context())
	boom := errors.New("boom")
	env.DB.AuditEntry.Use(failing(ent.OpUpdate|ent.OpUpdateOne, boom))

	err := retention(env, testhelper.License(t, licensekey.FeatureRetention), func() time.Time { return febFirst }).Sweep(t.Context(), fixtures.AcmeID)

	require.ErrorIs(t, err, boom)
	assert.Zero(t, countEntries(t, env, fixtures.AcmeID))
}

// failing is an ent hook that fails every mutation of the ops with err.
func failing(ops ent.Op, err error) ent.Hook {
	return func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			if m.Op().Is(ops) {
				return nil, err
			}
			return next.Mutate(ctx, m)
		})
	}
}
