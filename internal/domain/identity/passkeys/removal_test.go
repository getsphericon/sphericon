package passkeys_test

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/passkey"
	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/domain/identity/passkeys"
	"github.com/getsphericon/sphericon/internal/domain/identity/secondfactors"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A Passkey removal and a Second factor reset of one User both write the User row and the User's
// Passkeys. Each takes the User row first, so the reset waits for the removal instead of
// deadlocking with it: the removal commits, and the reset, which checked the User before the
// removal moved it, fails as stale.
func TestARemovalAndAResetOfOneUserQueueOnTheUserRow(t *testing.T) {
	r := newRace(t)
	ready, release := pauseFirst(t, r.bus, bumpsEpoch)
	removed := make(chan error, 1)
	go func() {
		pat := r.client.User.GetX(t.Context(), fixtures.PasskeyPatID)
		_, err := r.keys.Remove(t.Context(), pat, users.IdentityProof{Password: fixtures.PasskeyPatPassword}, fixtures.PasskeyPatLaptopID)
		removed <- err
	}()
	testhelper.Receive(t, ready, "the removal reached the bump of Pat's session epoch")
	reset := make(chan error, 1)
	go func() { reset <- r.factors.ResetByStaff(t.Context(), fixtures.PasskeyPatEmail, "cli") }()
	require.Eventually(t, func() bool {
		n, err := testhelper.LockWaiters(t.Context(), r.db)
		return err == nil && n > 0
	}, 10*time.Second, 5*time.Millisecond, "the reset waits for a lock the removal holds")
	release()

	removal := testhelper.Receive(t, removed, "the removal finished")
	stale := testhelper.Receive(t, reset, "the reset finished")

	require.NoError(t, removal)
	require.ErrorIs(t, stale, lifecycle.ErrInvalidTransition)
	held, err := r.client.Passkey.Query().Where(passkey.UserID(fixtures.PasskeyPatID)).Exist(t.Context())
	require.NoError(t, err)
	assert.False(t, held)
}

// A User without a password or a Linked identity keeps one Passkey: two removals of their last two
// take the User row in turn, so the second sees the first gone and is refused.
func TestTwoRemovalsCannotTakeAPasswordlessUsersLastTwoPasskeys(t *testing.T) {
	r := newRace(t)
	ready, release := pauseFirst(t, r.bus, deletesPasskey)
	remove := func(id int64) error {
		rue := r.client.User.GetX(t.Context(), fixtures.PasskeyPairRueID)
		_, err := r.keys.Remove(t.Context(), rue, users.IdentityProof{}, id)
		return err
	}
	first := make(chan error, 1)
	go func() { first <- remove(fixtures.PasskeyRuePhoneID) }()
	testhelper.Receive(t, ready, "the first removal checked Rue's other Passkey")

	errs := testhelper.QueuedBehind(t, r.db, release, testhelper.Writer{Name: "second removal", Run: func() error {
		return remove(fixtures.PasskeyRueKeyID)
	}})

	require.NoError(t, testhelper.Receive(t, first, "the first removal finished"))
	require.ErrorIs(t, errs["second removal"], passkeys.ErrLastSignIn)
	left, err := r.client.Passkey.Query().Where(passkey.UserID(fixtures.PasskeyPairRueID)).IDs(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []int64{fixtures.PasskeyRueKeyID}, left)
}

// race is the Passkeys and the Second factors over a database of the test's own, whose pool lets
// two transactions wait side by side, and a bus that takes every job and keeps none.
type race struct {
	db      *sql.DB
	client  *ent.Client
	bus     *events.Bus
	keys    *passkeys.Module
	factors *secondfactors.Module
}

func newRace(t *testing.T) race {
	t.Helper()
	db := testhelper.RealDB(t)
	db.SetMaxOpenConns(4)
	client := entclient.Core.New(db)
	clock := time.Now
	bus := events.New(db, entclient.Core.NewTx, clock)
	bus.UseJobs(testhelper.AcceptingQueue{})
	keys, err := passkeys.New(client, bus, testhelper.ProvenUsers{}, "https://app.example", "passkeys-test-ceremony-secret", clock)
	require.NoError(t, err)
	factors := secondfactors.New(client, bus, testhelper.Cipher(t), testhelper.ProvenUsers{}, clock)
	return race{db: db, client: client, bus: bus, keys: keys, factors: factors}
}

// pauseFirst holds the first write of the bus's transactions that matches selects, before it
// reaches the database, until unblock.
func pauseFirst(t *testing.T, bus *events.Bus, matches func(ent.Mutation) bool) (paused <-chan struct{}, unblock func()) {
	t.Helper()
	ready, resume := make(chan struct{}, 1), make(chan struct{})
	release := sync.OnceFunc(func() { close(resume) })
	t.Cleanup(release)
	var fired atomic.Bool
	bus.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
			if matches(mutation) && fired.CompareAndSwap(false, true) {
				ready <- struct{}{}
				select {
				case <-resume:
				case <-ctx.Done():
				}
			}
			return next.Mutate(ctx, mutation)
		})
	})
	return ready, release
}

// bumpsEpoch is the write that ends a User's sessions: the bump of their session epoch.
func bumpsEpoch(m ent.Mutation) bool {
	u, ok := m.(*ent.UserMutation)
	if !ok {
		return false
	}
	epoch, adds := u.AddedSessionEpoch()
	return adds && epoch > 0
}

// deletesPasskey is the delete of a Passkey row.
func deletesPasskey(m ent.Mutation) bool {
	_, ok := m.(*ent.PasskeyMutation)
	return ok && m.Op().Is(ent.OpDeleteOne|ent.OpDelete)
}
