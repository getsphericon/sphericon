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
	"github.com/getsphericon/sphericon/ent/workspace"
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

	assert.Equal(t, []string{"future"}, keys(st.Staff))
	assert.Equal(t, holds.ReasonStaff, st.Reason())
}

func TestASecondActiveHoldKeepsSendingFrozenAfterTheFirstIsCleared(t *testing.T) {
	w := newWorld(t)
	w.hold(t, "b", w.now.Add(time.Hour))
	w.hold(t, "a", w.now.Add(time.Hour))

	assert.Equal(t, []string{"a", "b"}, keys(w.standing(t, true).Staff), "ordered by key")

	_, err := w.s.Hold().Delete().Where(enthold.Key("a")).Exec(w.ctx)
	require.NoError(t, err)

	cleared := w.standing(t, true)
	assert.Equal(t, []string{"b"}, keys(cleared.Staff))
	assert.Equal(t, holds.ReasonStaff, cleared.Reason())
}

func TestSuspensionIsItsOwnSourceBesideStaffHolds(t *testing.T) {
	w := newWorld(t)
	w.env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSuspendedAt(w.now).ExecX(w.ctx)
	w.hold(t, "review", w.now.Add(time.Hour))

	st := w.standing(t, true)

	assert.Equal(t, holds.ReasonSuspended, st.Reason(), "suspension outranks Staff holds")
	require.NotNil(t, st.SuspendedAt)
	assert.Equal(t, []string{"review"}, keys(st.Staff), "never merged: the hold stays in its own list")
}

func TestDeletionOutranksSuspensionAndStaffHolds(t *testing.T) {
	w := newWorld(t)
	w.env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSuspendedAt(w.now).SetDeletedAt(w.now).ExecX(w.ctx)
	w.hold(t, "review", w.now.Add(time.Hour))

	st := w.standing(t, true)

	assert.Equal(t, holds.ReasonPendingDeletion, st.Reason())
	require.NotNil(t, st.Deletion)
	assert.Equal(t, holds.KeyPendingDeletion, st.Deletion.Key)
	assert.Equal(t, holds.Message(holds.ReasonPendingDeletion), st.Deletion.Message)
	assert.NotNil(t, st.SuspendedAt)
	assert.Equal(t, []string{holds.KeyPendingDeletion, "review"}, keys(st.Holds()))
}

func TestWithoutTheStaffLicenseHoldsAreIgnoredButDeletionAndSuspensionCount(t *testing.T) {
	w := newWorld(t)
	w.hold(t, "review", w.now.Add(time.Hour))

	unlicensed := w.standing(t, false)
	assert.Empty(t, unlicensed.Reason())
	assert.Empty(t, unlicensed.Staff)

	w.env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSuspendedAt(w.now).ExecX(w.ctx)
	assert.Equal(t, holds.ReasonSuspended, w.standing(t, false).Reason())

	w.env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetDeletedAt(w.now).ExecX(w.ctx)
	assert.Equal(t, holds.ReasonPendingDeletion, w.standing(t, false).Reason())
}

// The site loads the Workspace with its holds edge under the active rule; the send path
// queries. For the same rows they stand alike.
func TestTheSiteViewAndTheSendPathAgree(t *testing.T) {
	w := newWorld(t)
	w.hold(t, "future", w.now.Add(time.Hour))
	w.hold(t, "boundary", w.now)
	w.hold(t, "past", w.now.Add(-time.Hour))

	ws, err := w.env.DB.Workspace.Query().Where(workspace.ID(fixtures.AcmeID)).WithHolds(holds.ActiveEdge(w.now)).Only(w.ctx)
	require.NoError(t, err)
	site := holds.Of(ws, true)
	send := w.standing(t, true)

	assert.Equal(t, send.Reason(), site.Reason())
	assert.Equal(t, keys(send.Holds()), keys(site.Holds()))
	assert.Equal(t, []string{"future"}, keys(site.Staff))
}

// The send path's reason comes from the constructor's one function: with it off, Staff holds
// are ignored while deletion and suspension still count; deletion is reported first.
func TestReasonHonoursTheLicenseFunctionAndReportsDeletionFirst(t *testing.T) {
	w := newWorld(t)
	w.hold(t, "review", w.now.Add(time.Hour))
	on := holds.New(holds.Always, func() time.Time { return w.now })
	off := holds.New(func() bool { return false }, func() time.Time { return w.now })
	reason := func(h *holds.Holds) string {
		t.Helper()
		got, err := h.Reason(w.ctx, w.s)
		require.NoError(t, err)
		return got
	}

	assert.Equal(t, holds.ReasonStaff, reason(on))
	assert.Empty(t, reason(off))

	w.env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSuspendedAt(w.now).ExecX(w.ctx)
	assert.Equal(t, holds.ReasonSuspended, reason(off))

	w.env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetDeletedAt(w.now).ExecX(w.ctx)
	assert.Equal(t, holds.ReasonPendingDeletion, reason(on))
	assert.Equal(t, holds.ReasonPendingDeletion, reason(off))
}
