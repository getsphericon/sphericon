package holds_test

import (
	"context"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	enthold "github.com/getsphericon/sphericon/ent/hold"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/holds"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func keys(rows []*ent.Hold) []string {
	return lo.Map(rows, func(h *ent.Hold, _ int) string { return h.Key })
}

type world struct {
	env *testhelper.TestEnv
	ctx context.Context
	s   *ent.Scoped
	now time.Time
}

func newWorld(t *testing.T) world {
	t.Helper()
	env := testhelper.Setup(t)
	return world{
		env: env,
		ctx: context.Background(),
		s:   env.DB.Scoped(fixtures.AcmeID),
		now: time.Now().Truncate(time.Microsecond), // the precision the database stores
	}
}

func (w world) hold(t *testing.T, key string, expires time.Time) {
	t.Helper()
	_, err := w.s.Hold().Create().SetKey(key).SetReasonCode("review").SetMessage("held").
		SetSetBy("service-a").SetExpiresAt(expires).Save(w.ctx)
	require.NoError(t, err)
}

func (w world) standing(t *testing.T, staffHonoured bool) holds.Standing {
	t.Helper()
	st, err := holds.Read(w.ctx, w.s, w.now, staffHonoured)
	require.NoError(t, err)
	return st
}

func TestLiveWorkspaceWithOnlyLapsedHoldsIsNotFrozen(t *testing.T) {
	w := newWorld(t) // the fixture Workspace carries a lapsed hold

	st := w.standing(t, true)

	assert.Empty(t, st.Reason())
	assert.Empty(t, st.Holds())
}

func TestAHoldIsActiveOnlyWhileItsExpiryIsAfterNow(t *testing.T) {
	w := newWorld(t)
	w.hold(t, "future", w.now.Add(time.Hour))
	w.hold(t, "boundary", w.now)
	w.hold(t, "past", w.now.Add(-time.Hour))

	st := w.standing(t, true)
	staffKeys := keys(st.Staff)
	reason := st.Reason()

	assert.Equal(t, []string{"future"}, staffKeys)
	assert.Equal(t, holds.ReasonStaff, reason)
}

func TestASecondActiveHoldKeepsSendingFrozenAfterTheFirstIsCleared(t *testing.T) {
	w := newWorld(t)
	w.hold(t, "b", w.now.Add(time.Hour))
	w.hold(t, "a", w.now.Add(time.Hour))

	both := w.standing(t, true)
	bothKeys := keys(both.Staff)
	assert.Equal(t, []string{"a", "b"}, bothKeys, "ordered by key")

	_, err := w.s.Hold().Delete().Where(enthold.Key("a")).Exec(w.ctx)
	require.NoError(t, err)

	cleared := w.standing(t, true)
	clearedKeys := keys(cleared.Staff)
	clearedReason := cleared.Reason()
	assert.Equal(t, []string{"b"}, clearedKeys)
	assert.Equal(t, holds.ReasonStaff, clearedReason)
}

func TestSuspensionIsItsOwnSourceBesideStaffHolds(t *testing.T) {
	w := newWorld(t)
	w.env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSuspendedAt(w.now).ExecX(w.ctx)
	w.hold(t, "review", w.now.Add(time.Hour))

	st := w.standing(t, true)
	reason := st.Reason()
	staffKeys := keys(st.Staff)

	assert.Equal(t, holds.ReasonSuspended, reason, "suspension outranks Staff holds")
	require.NotNil(t, st.SuspendedAt)
	assert.Equal(t, []string{"review"}, staffKeys, "never merged: the hold stays in its own list")
}

func TestDeletionOutranksSuspensionAndStaffHolds(t *testing.T) {
	w := newWorld(t)
	w.env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSuspendedAt(w.now).SetDiscardedAt(w.now).ExecX(w.ctx)
	w.hold(t, "review", w.now.Add(time.Hour))

	st := w.standing(t, true)
	reason := st.Reason()
	listed := keys(st.Holds())

	assert.Equal(t, holds.ReasonPendingDeletion, reason)
	require.NotNil(t, st.Deletion)
	assert.Equal(t, holds.KeyPendingDeletion, st.Deletion.Key)
	assert.Equal(t, holds.Message(holds.ReasonPendingDeletion), st.Deletion.Message)
	assert.NotNil(t, st.SuspendedAt)
	assert.Equal(t, []string{holds.KeyPendingDeletion, "review"}, listed)
}

func TestWithoutTheStaffLicenseHoldsAreIgnoredButDeletionAndSuspensionCount(t *testing.T) {
	w := newWorld(t)
	w.hold(t, "review", w.now.Add(time.Hour))

	unlicensed := w.standing(t, false)
	unlicensedReason := unlicensed.Reason()
	assert.Empty(t, unlicensedReason)
	assert.Empty(t, unlicensed.Staff)

	w.env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSuspendedAt(w.now).ExecX(w.ctx)
	suspended := w.standing(t, false)
	suspendedReason := suspended.Reason()
	assert.Equal(t, holds.ReasonSuspended, suspendedReason)

	w.env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetDiscardedAt(w.now).ExecX(w.ctx)
	deleted := w.standing(t, false)
	deletedReason := deleted.Reason()
	assert.Equal(t, holds.ReasonPendingDeletion, deletedReason)
}

// The module's one function decides whether Staff holds count: with it off they are ignored
// while deletion and suspension still count; deletion is reported first.
func TestReasonHonoursTheLicenseFunctionAndReportsDeletionFirst(t *testing.T) {
	w := newWorld(t)
	w.hold(t, "review", w.now.Add(time.Hour))
	clock := func() time.Time { return w.now }
	on := holds.New(func() bool { return true }, clock)
	off := holds.New(func() bool { return false }, clock)
	reason := func(h *holds.Holds) string {
		t.Helper()
		got, err := h.Reason(w.ctx, w.s)
		require.NoError(t, err)
		return got
	}

	onReason := reason(on)
	offReason := reason(off)
	assert.Equal(t, holds.ReasonStaff, onReason)
	assert.Empty(t, offReason)

	w.env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSuspendedAt(w.now).ExecX(w.ctx)
	suspendedOff := reason(off)
	assert.Equal(t, holds.ReasonSuspended, suspendedOff)

	w.env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetDiscardedAt(w.now).ExecX(w.ctx)
	deletedOn := reason(on)
	deletedOff := reason(off)
	assert.Equal(t, holds.ReasonPendingDeletion, deletedOn)
	assert.Equal(t, holds.ReasonPendingDeletion, deletedOff)
}
