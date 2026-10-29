package accounts_test

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
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestRoleDecisionsAreNamedAndSeparate(t *testing.T) {
	cases := []struct {
		role                   membership.Role
		members, tokens, erase bool
	}{
		{membership.RoleOwner, true, true, true},
		{membership.RoleAdmin, true, true, true},
		{membership.RoleMember, false, false, false},
	}
	for _, c := range cases {
		assert.Equal(t, c.members, accounts.CanManageMembers(c.role), "manage members: %s", c.role)
		assert.Equal(t, c.tokens, accounts.CanManageTokens(c.role), "manage tokens: %s", c.role)
		assert.Equal(t, c.erase, accounts.CanErase(c.role), "erase: %s", c.role)
	}
}

func TestChangeMembershipRoleInvariants(t *testing.T) {
	env := testhelper.Setup(t)
	acc := accounts.New(env.DB, env.Bus, testhelper.Passwords())
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.InitechID)
	actor := events.Actor{Kind: events.ActorUser}

	roleOf := func(id int64) membership.Role {
		return env.DB.Membership.GetX(ctx, id).Role
	}

	t.Run("an admin cannot grant the owner Role", func(t *testing.T) {
		_, err := acc.ChangeMembershipRole(ctx, s, actor, membership.RoleAdmin, fixtures.InitechMemberMembershipID, membership.RoleOwner)
		require.ErrorIs(t, err, accounts.ErrOwnerOnly)
		assert.Equal(t, membership.RoleMember, roleOf(fixtures.InitechMemberMembershipID))
	})

	t.Run("an admin cannot alter an owner", func(t *testing.T) {
		_, err := acc.ChangeMembershipRole(ctx, s, actor, membership.RoleAdmin, fixtures.InitechCoOwnerMembershipID, membership.RoleMember)
		require.ErrorIs(t, err, accounts.ErrOwnerOnly)
		assert.Equal(t, membership.RoleOwner, roleOf(fixtures.InitechCoOwnerMembershipID))
	})

	t.Run("an owner grants the owner Role", func(t *testing.T) {
		m, err := acc.ChangeMembershipRole(ctx, s, actor, membership.RoleOwner, fixtures.InitechAdminMembershipID, membership.RoleOwner)
		require.NoError(t, err)
		assert.Equal(t, membership.RoleOwner, m.Role)
		require.NotNil(t, m.Edges.User, "the User edge comes loaded for the response")
		assert.Equal(t, fixtures.InitechAdminAdaName, m.Edges.User.Name)
	})

	t.Run("a co-owner can be demoted while another owner remains", func(t *testing.T) {
		m, err := acc.ChangeMembershipRole(ctx, s, actor, membership.RoleOwner, fixtures.InitechCoOwnerMembershipID, membership.RoleAdmin)
		require.NoError(t, err)
		assert.Equal(t, membership.RoleAdmin, m.Role)
	})

	t.Run("the last owner cannot be demoted", func(t *testing.T) {
		// The admin promoted above and the owner Olga remain; demote the former, then Olga is last.
		_, err := acc.ChangeMembershipRole(ctx, s, actor, membership.RoleOwner, fixtures.InitechAdminMembershipID, membership.RoleMember)
		require.NoError(t, err)
		_, err = acc.ChangeMembershipRole(ctx, s, actor, membership.RoleOwner, fixtures.InitechOwnerMembershipID, membership.RoleAdmin)
		require.ErrorIs(t, err, accounts.ErrLastOwner)
		assert.Equal(t, membership.RoleOwner, roleOf(fixtures.InitechOwnerMembershipID))
	})

	t.Run("a Membership of another Workspace is not found", func(t *testing.T) {
		_, err := acc.ChangeMembershipRole(ctx, s, actor, membership.RoleOwner, fixtures.GlobexOwnerMembershipID, membership.RoleMember)
		require.Error(t, err)
		assert.True(t, ent.IsNotFound(err))
	})
}

// membershipUpdateEntries hands the outbox to the audit subscriber and counts the stored
// `membership.update` entries.
func membershipUpdateEntries(t *testing.T, env *testhelper.TestEnv) int {
	t.Helper()
	env.DeliverToEE(t)
	return env.DB.AuditEntry.Query().Where(auditentry.Action(events.ActionMembershipUpdate)).CountX(t.Context())
}

func TestChangeMembershipRoleAuditsInTheSameTransaction(t *testing.T) {
	env := testhelper.Setup(t)
	acc := accounts.New(env.DB, env.Bus, testhelper.Passwords())
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.InitechID)
	actor := events.Actor{Kind: events.ActorUser}
	updates := func() int { return membershipUpdateEntries(t, env) }

	before := updates()
	_, err := acc.ChangeMembershipRole(ctx, s, actor, membership.RoleOwner, fixtures.InitechCoOwnerMembershipID, membership.RoleAdmin)
	require.NoError(t, err)
	assert.Equal(t, before+1, updates(), "a committed change leaves its entry")

	// Olga is now the last owner: the refused change rolls back and records nothing.
	_, err = acc.ChangeMembershipRole(ctx, s, actor, membership.RoleOwner, fixtures.InitechOwnerMembershipID, membership.RoleAdmin)
	require.ErrorIs(t, err, accounts.ErrLastOwner)
	assert.Equal(t, before+1, updates(), "a refused change leaves no entry")
}

func TestRemoveMembershipInvariants(t *testing.T) {
	env := testhelper.Setup(t)
	acc := accounts.New(env.DB, env.Bus, testhelper.Passwords())
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.InitechID)
	actor := events.Actor{Kind: events.ActorUser}

	exists := func(id int64) bool {
		ok, err := env.DB.Membership.Query().Where(membership.ID(id)).Exist(ctx)
		require.NoError(t, err)
		return ok
	}

	t.Run("an admin cannot remove an owner", func(t *testing.T) {
		err := acc.RemoveMembership(ctx, s, actor, membership.RoleAdmin, fixtures.InitechCoOwnerMembershipID)
		require.ErrorIs(t, err, accounts.ErrOwnerOnly)
		assert.True(t, exists(fixtures.InitechCoOwnerMembershipID))
	})

	t.Run("an admin removes a member", func(t *testing.T) {
		require.NoError(t, acc.RemoveMembership(ctx, s, actor, membership.RoleAdmin, fixtures.InitechMemberMembershipID))
		assert.False(t, exists(fixtures.InitechMemberMembershipID))
	})

	t.Run("an owner removes a co-owner, then the last owner cannot be removed", func(t *testing.T) {
		require.NoError(t, acc.RemoveMembership(ctx, s, actor, membership.RoleOwner, fixtures.InitechCoOwnerMembershipID))
		err := acc.RemoveMembership(ctx, s, actor, membership.RoleOwner, fixtures.InitechOwnerMembershipID)
		require.ErrorIs(t, err, accounts.ErrLastOwner)
		assert.True(t, exists(fixtures.InitechOwnerMembershipID))
	})

	t.Run("a Membership of another Workspace is not found", func(t *testing.T) {
		err := acc.RemoveMembership(ctx, s, actor, membership.RoleOwner, fixtures.GlobexOwnerMembershipID)
		require.Error(t, err)
		assert.True(t, ent.IsNotFound(err))
		assert.True(t, exists(fixtures.GlobexOwnerMembershipID))
	})
}

func TestLeaveRemovesTheCallersOwnMembershipWhateverTheRole(t *testing.T) {
	env := testhelper.Setup(t)
	acc := accounts.New(env.DB, env.Bus, testhelper.Passwords())
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.InitechID)

	exists := func(id int64) bool {
		ok, err := env.DB.Membership.Query().Where(membership.ID(id)).Exist(ctx)
		require.NoError(t, err)
		return ok
	}

	require.NoError(t, acc.Leave(ctx, s, fixtures.InitechMemberMaxID))
	assert.False(t, exists(fixtures.InitechMemberMembershipID))

	require.NoError(t, acc.Leave(ctx, s, fixtures.InitechAdminAdaID))
	assert.False(t, exists(fixtures.InitechAdminMembershipID))

	require.NoError(t, acc.Leave(ctx, s, fixtures.InitechCoOwnerOwenID))
	assert.False(t, exists(fixtures.InitechCoOwnerMembershipID))
}

func TestLeaveRefusesTheOnlyOwner(t *testing.T) {
	env := testhelper.Setup(t)
	acc := accounts.New(env.DB, env.Bus, testhelper.Passwords())
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.InitechID)

	require.NoError(t, acc.Leave(ctx, s, fixtures.InitechCoOwnerOwenID))
	err := acc.Leave(ctx, s, fixtures.InitechOwnerOlgaID)
	require.ErrorIs(t, err, accounts.ErrLastOwner)

	ok, err := env.DB.Membership.Query().Where(membership.ID(fixtures.InitechOwnerMembershipID)).Exist(ctx)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestLeaveByAUserWithoutMembershipIsNotFound(t *testing.T) {
	env := testhelper.Setup(t)
	acc := accounts.New(env.DB, env.Bus, testhelper.Passwords())

	err := acc.Leave(t.Context(), env.DB.Scoped(fixtures.InitechID), fixtures.OwnerJohnID)
	assert.True(t, ent.IsNotFound(err))
}

func TestLeaveAuditsTheUserAsActor(t *testing.T) {
	env := testhelper.Setup(t)
	acc := accounts.New(env.DB, env.Bus, testhelper.Passwords())
	ctx := t.Context()

	require.NoError(t, acc.Leave(ctx, env.DB.Scoped(fixtures.InitechID), fixtures.InitechMemberMaxID))
	env.DeliverToEE(t)

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
	client := db.NewEntClient(sqlDB)
	acc := accounts.New(client, events.New(sqlDB), testhelper.Passwords())
	s := client.Scoped(fixtures.InitechID)

	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i, userID := range []int64{fixtures.InitechOwnerOlgaID, fixtures.InitechCoOwnerOwenID} {
		wg.Go(func() { errs[i] = acc.Leave(t.Context(), s, userID) })
	}
	wg.Wait()

	refused := lo.Filter(errs, func(err error, _ int) bool { return err != nil })
	require.Len(t, refused, 1)
	require.ErrorIs(t, refused[0], accounts.ErrLastOwner)
	owners, err := s.Membership().Query().Where(membership.RoleEQ(membership.RoleOwner)).Count(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, owners)
}

// Two co-Owners deleting themselves at the same moment serialize on the Workspace's Owner
// rows (ADR 0050): exactly one is deleted, the other is refused as the last Owner.
func TestTwoCoOwnersDeletingThemselvesAtOnceDeleteExactlyOne(t *testing.T) {
	sqlDB := testhelper.RealDB(t)
	sqlDB.SetMaxOpenConns(8)
	client := db.NewEntClient(sqlDB)
	acc := accounts.New(client, events.New(sqlDB), testhelper.Passwords())

	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i, userID := range []int64{fixtures.InitechOwnerOlgaID, fixtures.InitechCoOwnerOwenID} {
		wg.Go(func() { _, errs[i] = acc.DeleteUser(t.Context(), userID, nil) })
	}
	wg.Wait()

	refused := lo.Filter(errs, func(err error, _ int) bool { return err != nil })
	require.Len(t, refused, 1)
	require.ErrorIs(t, refused[0], accounts.ErrLastOwner)
	var sole *accounts.SoleOwnerError
	require.ErrorAs(t, refused[0], &sole)
	assert.Equal(t, fixtures.InitechName, sole.Workspaces[0].Name)
	owners, err := client.Scoped(fixtures.InitechID).Membership().Query().Where(membership.RoleEQ(membership.RoleOwner)).Count(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, owners)
}
