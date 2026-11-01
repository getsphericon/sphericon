package trackinghostlog_test

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/trackingdomain"
	"github.com/getsphericon/sphericon/ent/trackinghostchange"
	"github.com/getsphericon/sphericon/internal/cleanup"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/trackinghostlog"
)

// logged is what the log holds for a host, in log order.
func logged(t *testing.T, env *testhelper.TestEnv, host string) []trackinghostchange.State {
	t.Helper()
	rows, err := env.DB.TrackingHostChange.Query().
		Where(trackinghostchange.Host(host)).
		Order(ent.Asc(trackinghostchange.FieldID)).
		All(t.Context())
	require.NoError(t, err)
	states := make([]trackinghostchange.State, len(rows))
	for i, r := range rows {
		states[i] = r.State
	}
	return states
}

// A host that leaves the verified states is reported as removed, so the edge tears it down.
func TestAHostThatFallsBackToPendingIsRemoved(t *testing.T) {
	env := testhelper.Setup(t)

	// test writes: the hook fires on this write
	require.NoError(t, env.FeedDB.Scoped(fixtures.UmbrellaID).TrackingDomain().
		UpdateOneID(fixtures.TrackingDomainServingID).
		SetState(trackingdomain.StatePending).Exec(t.Context()))

	assert.Equal(t, []trackinghostchange.State{trackinghostchange.StateRemoved},
		logged(t, env, fixtures.TrackingDomainServingHost))
}

// A deleted host that was verified is removed; one that never was has nothing to take back.
func TestDeletingAHostLogsItOnlyWhenItWasVerified(t *testing.T) {
	env := testhelper.Setup(t)
	umbrella := env.FeedDB.Scoped(fixtures.UmbrellaID)

	require.NoError(t, umbrella.TrackingDomain().DeleteOneID(fixtures.TrackingDomainRetiredID).Exec(t.Context()))
	require.NoError(t, umbrella.TrackingDomain().DeleteOneID(fixtures.TrackingDomainReplacementID).Exec(t.Context()))

	assert.Equal(t, []trackinghostchange.State{trackinghostchange.StateRemoved},
		logged(t, env, fixtures.TrackingDomainRetiredHost))
	assert.Empty(t, logged(t, env, fixtures.TrackingDomainReplacementHost))
}

func TestACreatedHostIsLoggedOnlyWhenItIsAlreadyVerified(t *testing.T) {
	env := testhelper.Setup(t)
	create := func(ws int64, host string) *ent.TrackingDomainScopedCreate {
		// test writes: the hook fires on this write
		return env.FeedDB.Scoped(ws).TrackingDomain().Create().SetHost(host).SetRole(trackingdomain.RoleRetired)
	}

	// test writes: the hook fires on this write
	require.NoError(t, env.FeedDB.Scoped(fixtures.KramericaID).TrackingDomain().Create().
		SetHost("new-pending.kramerica.test").SetRole(trackingdomain.RoleCurrent).Exec(t.Context()))
	require.NoError(t, create(fixtures.KramericaID, "new-dns.kramerica.test").
		SetState(trackingdomain.StateDNSVerified).Exec(t.Context()))

	assert.Empty(t, logged(t, env, "new-pending.kramerica.test"))
	assert.Equal(t, []trackinghostchange.State{trackinghostchange.StateDNSVerified},
		logged(t, env, "new-dns.kramerica.test"))
}

// A change of role keeps the host served: a retired host still serves what was sent on it.
func TestRetiringAServingHostLogsNothing(t *testing.T) {
	env := testhelper.Setup(t)

	// test writes: the hook fires on this write
	require.NoError(t, env.FeedDB.Scoped(fixtures.UmbrellaID).TrackingDomain().
		UpdateOneID(fixtures.TrackingDomainServingID).
		SetRole(trackingdomain.RoleRetired).Exec(t.Context()))

	assert.Empty(t, logged(t, env, fixtures.TrackingDomainServingHost))
}

// A log row written outside the transaction of its change is a separate statement: a
// crash between the two loses the change. The plain client refuses such a change.
func TestPlainClientRefusesAChangeToAVerifiedHost(t *testing.T) {
	testhelper.Setup(t) // loads the schema into the real database once
	cfg, err := config.Load("test")
	require.NoError(t, err)
	pool, err := sql.Open("pgx", cfg.DatabaseURL)
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Close(pool, "pool") })
	client := db.NewEntClient(pool)

	err = client.TrackingDomain.UpdateOneID(fixtures.TrackingDomainServingID).
		SetState(trackingdomain.StatePending).Exec(t.Context())
	require.ErrorIs(t, err, trackinghostlog.ErrNeedsTransaction)

	err = client.TrackingDomain.DeleteOneID(fixtures.TrackingDomainRetiredID).Exec(t.Context())
	require.ErrorIs(t, err, trackinghostlog.ErrNeedsTransaction)
}

// Serving is the query the host gate and the recheck job use: a verified state serves in any
// role, retired included, and a pending one does not.
func TestServingSelectsVerifiedHostsInAnyRole(t *testing.T) {
	env := testhelper.Setup(t)

	ids, err := env.DB.TrackingDomain.Query().Where(trackinghostlog.Serving()).IDs(t.Context())
	require.NoError(t, err)

	assert.Contains(t, ids, int64(fixtures.TrackingDomainServingID))
	assert.Contains(t, ids, int64(fixtures.TrackingDomainDNSVerifiedID))
	assert.Contains(t, ids, int64(fixtures.TrackingDomainRetiredID))
	assert.NotContains(t, ids, int64(fixtures.TrackingDomainReplacementID))
	assert.NotContains(t, ids, int64(fixtures.TrackingDomainClaimID))
}
