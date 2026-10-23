package trackinghostlog_test

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/sendingdomain"
	"github.com/getsphericon/sphericon/ent/trackinghostchange"
	"github.com/getsphericon/sphericon/internal/cleanup"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/trackinghostlog"
)

// logged is what the log holds for a domain, in log order.
func logged(t *testing.T, env *testhelper.TestEnv, domain string) []trackinghostchange.State {
	t.Helper()
	rows, err := env.DB.TrackingHostChange.Query().
		Where(trackinghostchange.Domain(domain)).
		Order(ent.Asc(trackinghostchange.FieldID)).
		All(t.Context())
	require.NoError(t, err)
	states := make([]trackinghostchange.State, len(rows))
	for i, r := range rows {
		states[i] = r.State
	}
	return states
}

// Renaming a verified host takes the old name out of service and puts the new one in,
// in that order, so the edge never serves a name the Workspace no longer owns.
func TestRenamingAVerifiedHostRemovesTheOldNameAndAddsTheNewOne(t *testing.T) {
	env := testhelper.Setup(t)
	umbrella := env.DB.Scoped(fixtures.UmbrellaID)

	require.NoError(t, umbrella.SendingDomain().UpdateOneID(fixtures.SendingDomainTrackingServingID).
		SetDomain("renamed.umbrella.test").Exec(t.Context()))

	assert.Equal(t, []trackinghostchange.State{trackinghostchange.StateRemoved},
		logged(t, env, fixtures.SendingDomainTrackingServingDomain))
	assert.Equal(t, []trackinghostchange.State{trackinghostchange.StateServing},
		logged(t, env, "renamed.umbrella.test"))
}

// A host that was never verified has nothing to take back when it is renamed.
func TestRenamingAPendingHostLogsNothing(t *testing.T) {
	env := testhelper.Setup(t)

	require.NoError(t, env.DB.Scoped(fixtures.UmbrellaID).SendingDomain().
		UpdateOneID(fixtures.SendingDomainTrackingPendingID).
		SetDomain("renamed-pending.umbrella.test").Exec(t.Context()))

	assert.Empty(t, logged(t, env, fixtures.SendingDomainTrackingPendingDomain))
	assert.Empty(t, logged(t, env, "renamed-pending.umbrella.test"))
}

func TestACreatedHostIsLoggedOnlyWhenItIsAlreadyVerified(t *testing.T) {
	env := testhelper.Setup(t)
	umbrella := env.DB.Scoped(fixtures.UmbrellaID)
	create := func(domain string) *ent.SendingDomainScopedCreate {
		// test writes: the hook fires on this write
		return umbrella.SendingDomain().Create().
			SetDomain(domain).
			SetDkimSelector("sel").
			SetDkimPrivateKeyEncrypted("sealed").
			SetDkimPublicKey("v=DKIM1")
	}

	require.NoError(t, create("new-pending.umbrella.test").Exec(t.Context()))
	require.NoError(t, create("new-dns.umbrella.test").
		SetTrackingState(sendingdomain.TrackingStateDNSVerified).Exec(t.Context()))

	assert.Empty(t, logged(t, env, "new-pending.umbrella.test"))
	assert.Equal(t, []trackinghostchange.State{trackinghostchange.StateDNSVerified},
		logged(t, env, "new-dns.umbrella.test"))
}

// A change to a column the host does not depend on is not a change of the host.
func TestEditingAnUnrelatedColumnOfAServingHostLogsNothing(t *testing.T) {
	env := testhelper.Setup(t)

	require.NoError(t, env.DB.Scoped(fixtures.UmbrellaID).SendingDomain().
		UpdateOneID(fixtures.SendingDomainTrackingServingID).
		SetDkimSelector("rotated").Exec(t.Context()))

	assert.Empty(t, logged(t, env, fixtures.SendingDomainTrackingServingDomain))
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

	err = client.SendingDomain.UpdateOneID(fixtures.SendingDomainTrackingServingID).
		SetTrackingState(sendingdomain.TrackingStatePending).Exec(t.Context())
	require.ErrorIs(t, err, trackinghostlog.ErrNeedsTransaction)

	err = client.SendingDomain.UpdateOneID(fixtures.SendingDomainTrackingServingID).
		SetDkimSelector("rotated").Exec(t.Context())
	require.NoError(t, err, "a column the log does not depend on goes through")
}

// Serving is the query the host gate and the recheck job use: a verified state serves,
// a pending one does not, and archiving a verified one takes it out, which is the same
// answer the change log gives for the row (an archive logs the host as removed).
func TestServingSelectsVerifiedHostsThatAreNotArchived(t *testing.T) {
	env := testhelper.Setup(t)
	umbrella := env.DB.Scoped(fixtures.UmbrellaID)
	ids := func() []int64 {
		got, err := env.DB.SendingDomain.Query().Where(trackinghostlog.Serving()).IDs(t.Context())
		require.NoError(t, err)
		return got
	}

	before := ids()
	assert.Contains(t, before, int64(fixtures.SendingDomainTrackingDNSVerifiedID))
	assert.Contains(t, before, int64(fixtures.SendingDomainTrackingServingID))
	assert.NotContains(t, before, int64(fixtures.SendingDomainTrackingPendingID))

	require.NoError(t, umbrella.SendingDomain().Discard(t.Context(), fixtures.SendingDomainTrackingServingID))

	after := ids()
	assert.NotContains(t, after, int64(fixtures.SendingDomainTrackingServingID))
	assert.Equal(t, []trackinghostchange.State{trackinghostchange.StateRemoved},
		logged(t, env, fixtures.SendingDomainTrackingServingDomain))
}
