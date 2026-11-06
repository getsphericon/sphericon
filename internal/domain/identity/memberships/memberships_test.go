package memberships_test

import (
	"context"
	"sync"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/auditentry"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestChangeMembershipRoleInvariants(t *testing.T) {
	env := testhelper.Setup(t)
	ms := memberships.New(env.DB, env.Bus)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.InitechID)
	actor := events.Actor{Kind: events.ActorUser}

	roleOf := func(id int64) membership.Role {
		return env.DB.Membership.GetX(ctx, id).Role
	}

	t.Run("an admin cannot grant the owner Role", func(t *testing.T) {
		_, err := ms.ChangeRole(ctx, s, actor, membership.RoleAdmin, fixtures.InitechMemberMembershipID, membership.RoleOwner)
		require.ErrorIs(t, err, memberships.Require(membership.RoleAdmin, memberships.PermOwnersManage))
		assert.Equal(t, membership.RoleMember, roleOf(fixtures.InitechMemberMembershipID))
	})

	t.Run("an admin cannot alter an owner", func(t *testing.T) {
		_, err := ms.ChangeRole(ctx, s, actor, membership.RoleAdmin, fixtures.InitechCoOwnerMembershipID, membership.RoleMember)
		require.ErrorIs(t, err, memberships.Require(membership.RoleAdmin, memberships.PermOwnersManage))
		assert.Equal(t, membership.RoleOwner, roleOf(fixtures.InitechCoOwnerMembershipID))
	})

	t.Run("an owner grants the owner Role", func(t *testing.T) {
		m, err := ms.ChangeRole(ctx, s, actor, membership.RoleOwner, fixtures.InitechAdminMembershipID, membership.RoleOwner)
		require.NoError(t, err)
		assert.Equal(t, membership.RoleOwner, m.Role)
		require.NotNil(t, m.Edges.User, "the User edge comes loaded for the response")
		assert.Equal(t, fixtures.InitechAdminAdaName, m.Edges.User.Name)
	})

	t.Run("a co-owner can be demoted while another owner remains", func(t *testing.T) {
		m, err := ms.ChangeRole(ctx, s, actor, membership.RoleOwner, fixtures.InitechCoOwnerMembershipID, membership.RoleAdmin)
		require.NoError(t, err)
		assert.Equal(t, membership.RoleAdmin, m.Role)
	})

	t.Run("the last owner cannot be demoted", func(t *testing.T) {
		// The admin promoted above and the owner Olga remain; demote the former, then Olga is last.
		_, err := ms.ChangeRole(ctx, s, actor, membership.RoleOwner, fixtures.InitechAdminMembershipID, membership.RoleMember)
		require.NoError(t, err)
		_, err = ms.ChangeRole(ctx, s, actor, membership.RoleOwner, fixtures.InitechOwnerMembershipID, membership.RoleAdmin)
		require.ErrorIs(t, err, memberships.ErrDemoteLastOwner)
		assert.Equal(t, membership.RoleOwner, roleOf(fixtures.InitechOwnerMembershipID))
	})

	t.Run("a Membership of another Workspace is not found", func(t *testing.T) {
		_, err := ms.ChangeRole(ctx, s, actor, membership.RoleOwner, fixtures.GlobexOwnerMembershipID, membership.RoleMember)
		require.ErrorIs(t, err, memberships.ErrMemberNotFound)
	})
}

// membershipUpdateEntries works the Edition's jobs and counts the stored
// `membership.update` entries.
func membershipUpdateEntries(t *testing.T, env *testhelper.TestEnv) int {
	t.Helper()
	env.WorkEditionJobs(t)
	return env.DB.AuditEntry.Query().Where(auditentry.Action(events.ActionMembershipUpdate)).CountX(t.Context())
}

func TestChangeMembershipRoleAuditsInTheSameTransaction(t *testing.T) {
	env := testhelper.Setup(t)
	ms := memberships.New(env.DB, env.Bus)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.InitechID)
	actor := events.Actor{Kind: events.ActorUser}
	updates := func() int { return membershipUpdateEntries(t, env) }

	before := updates()
	_, err := ms.ChangeRole(ctx, s, actor, membership.RoleOwner, fixtures.InitechCoOwnerMembershipID, membership.RoleAdmin)
	require.NoError(t, err)
	assert.Equal(t, before+1, updates(), "a committed change leaves its entry")

	// Olga is now the last owner: the refused change rolls back and records nothing.
	_, err = ms.ChangeRole(ctx, s, actor, membership.RoleOwner, fixtures.InitechOwnerMembershipID, membership.RoleAdmin)
	require.ErrorIs(t, err, memberships.ErrDemoteLastOwner)
	assert.Equal(t, before+1, updates(), "a refused change leaves no entry")
}

func TestRemoveMembershipInvariants(t *testing.T) {
	env := testhelper.Setup(t)
	ms := memberships.New(env.DB, env.Bus)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.InitechID)
	actor := events.Actor{Kind: events.ActorUser}

	exists := func(id int64) bool {
		ok, err := env.DB.Membership.Query().Where(membership.ID(id)).Exist(ctx)
		require.NoError(t, err)
		return ok
	}

	t.Run("an admin cannot remove an owner", func(t *testing.T) {
		err := ms.Remove(ctx, s, actor, membership.RoleAdmin, fixtures.InitechCoOwnerMembershipID)
		require.ErrorIs(t, err, memberships.Require(membership.RoleAdmin, memberships.PermOwnersManage))
		assert.True(t, exists(fixtures.InitechCoOwnerMembershipID))
	})

	t.Run("an admin removes a member", func(t *testing.T) {
		require.NoError(t, ms.Remove(ctx, s, actor, membership.RoleAdmin, fixtures.InitechMemberMembershipID))
		assert.False(t, exists(fixtures.InitechMemberMembershipID))
	})

	t.Run("an owner removes a co-owner, then the last owner cannot be removed", func(t *testing.T) {
		require.NoError(t, ms.Remove(ctx, s, actor, membership.RoleOwner, fixtures.InitechCoOwnerMembershipID))
		err := ms.Remove(ctx, s, actor, membership.RoleOwner, fixtures.InitechOwnerMembershipID)
		require.ErrorIs(t, err, memberships.ErrRemoveLastOwner)
		assert.True(t, exists(fixtures.InitechOwnerMembershipID))
	})

	t.Run("a Membership of another Workspace is not found", func(t *testing.T) {
		err := ms.Remove(ctx, s, actor, membership.RoleOwner, fixtures.GlobexOwnerMembershipID)
		require.ErrorIs(t, err, memberships.ErrMemberNotFound)
		assert.True(t, exists(fixtures.GlobexOwnerMembershipID))
	})
}

func TestLeaveRemovesTheCallersOwnMembershipWhateverTheRole(t *testing.T) {
	env := testhelper.Setup(t)
	ms := memberships.New(env.DB, env.Bus)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.InitechID)

	exists := func(id int64) bool {
		ok, err := env.DB.Membership.Query().Where(membership.ID(id)).Exist(ctx)
		require.NoError(t, err)
		return ok
	}

	require.NoError(t, ms.Leave(ctx, s, fixtures.InitechMemberMaxID))
	assert.False(t, exists(fixtures.InitechMemberMembershipID))

	require.NoError(t, ms.Leave(ctx, s, fixtures.InitechAdminAdaID))
	assert.False(t, exists(fixtures.InitechAdminMembershipID))

	require.NoError(t, ms.Leave(ctx, s, fixtures.InitechCoOwnerOwenID))
	assert.False(t, exists(fixtures.InitechCoOwnerMembershipID))
}

func TestLeaveRefusesTheOnlyOwner(t *testing.T) {
	env := testhelper.Setup(t)
	ms := memberships.New(env.DB, env.Bus)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.InitechID)

	require.NoError(t, ms.Leave(ctx, s, fixtures.InitechCoOwnerOwenID))
	err := ms.Leave(ctx, s, fixtures.InitechOwnerOlgaID)
	require.ErrorIs(t, err, memberships.ErrOnlyOwnerLeaves)

	ok, err := env.DB.Membership.Query().Where(membership.ID(fixtures.InitechOwnerMembershipID)).Exist(ctx)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestLeaveByAUserWithoutMembershipIsNotFound(t *testing.T) {
	env := testhelper.Setup(t)
	ms := memberships.New(env.DB, env.Bus)

	err := ms.Leave(t.Context(), env.DB.Scoped(fixtures.InitechID), fixtures.OwnerJohnID)
	assert.True(t, ent.IsNotFound(err))
}

func TestLeaveAuditsTheUserAsActor(t *testing.T) {
	env := testhelper.Setup(t)
	ms := memberships.New(env.DB, env.Bus)
	ctx := t.Context()

	require.NoError(t, ms.Leave(ctx, env.DB.Scoped(fixtures.InitechID), fixtures.InitechMemberMaxID))
	env.WorkEditionJobs(t)

	entry, err := env.DB.AuditEntry.Query().Where(auditentry.Action(events.ActionMembershipDelete)).Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, events.ActorUser, entry.ActorKind)
	assert.Equal(t, fixtures.InitechMemberMaxName, lo.FromPtr(entry.ActorName))
	assert.Equal(t, fixtures.InitechMemberMaxName, lo.FromPtr(entry.TargetName))
}

// Two co-Owners leaving at the same moment serialize on the Workspace's Owner rows: the
// second reads the first's committed outcome. The other tests ride one go-txdb transaction,
// which serializes statements, so this one runs on a database of its own.
func TestTwoCoOwnersLeavingAtOnceLeaveOneOwner(t *testing.T) {
	sqlDB := testhelper.RealDB(t)
	sqlDB.SetMaxOpenConns(8)
	client := entclient.New(sqlDB)
	bus := events.New(sqlDB, entclient.NewTx)
	ms := memberships.New(client, bus)
	s := client.Scoped(fixtures.InitechID)

	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i, userID := range []int64{fixtures.InitechOwnerOlgaID, fixtures.InitechCoOwnerOwenID} {
		wg.Go(func() { errs[i] = ms.Leave(t.Context(), s, userID) })
	}
	wg.Wait()

	refused := lo.Filter(errs, func(err error, _ int) bool { return err != nil })
	require.Len(t, refused, 1)
	require.ErrorIs(t, refused[0], memberships.ErrOnlyOwnerLeaves)
	owners, err := s.Membership().Query().Where(membership.RoleEQ(membership.RoleOwner)).Count(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, owners)
}
