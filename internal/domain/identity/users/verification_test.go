package users_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/hook"
	"github.com/getsphericon/sphericon/ent/invitation"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/user"
	"github.com/getsphericon/sphericon/internal/domain/identity/invitations"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A Verified User is attached only with their own session, decided inside the
// transaction: whatever the caller believed about the address, no session is refused.
func TestAcceptInvitationOfAVerifiedUserNeedsTheirSession(t *testing.T) {
	env := testhelper.Setup(t)
	acc := users.New(env.DB, env.Bus, testhelper.Passwords(), env.ConsentRecords, time.Now)
	inv, _, err := invitations.New(env.DB, env.Bus, time.Now).Invite(t.Context(), env.DB.Scoped(fixtures.AcmeID), events.Actor{Kind: events.ActorUser},
		invitations.InviteInput{Email: fixtures.OutsiderOscarEmail, Role: invitation.RoleMember, InvitedBy: fixtures.OwnerJohnID})
	require.NoError(t, err)

	require.ErrorIs(t, acc.AcceptInvitation(t.Context(), inv, users.InvitationAcceptance{}), users.ErrSignInRequired)
	require.ErrorIs(t, acc.AcceptInvitation(t.Context(), inv, users.InvitationAcceptance{SessionEmail: fixtures.OwnerJaneEmail}), users.ErrWrongAccount)
	require.NoError(t, acc.AcceptInvitation(t.Context(), inv, users.InvitationAcceptance{SessionEmail: fixtures.OutsiderOscarEmail}))
}

// firstInsertMember is an ent hook that, just before an invitation's Membership is
// written, lets a concurrent accept win the race: it inserts the User's `member` Membership.
func firstInsertMember(next ent.Mutator) ent.Mutator {
	return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
		m, ok := mutation.(*ent.MembershipMutation)
		if !ok {
			return next.Mutate(ctx, mutation)
		}
		source, _ := m.Source()
		if source != membership.SourceInvitation {
			return next.Mutate(ctx, m)
		}
		userID, _ := m.UserID()
		workspaceID, _ := m.WorkspaceID()
		// test writes: a concurrent write simulated inside a hook is the race under test
		err := m.Client().Membership.Create().
			SetUserID(userID).
			SetWorkspaceID(workspaceID).
			SetRole(membership.RoleMember).
			SetSource(membership.SourceDomain).
			Exec(ctx)
		if err != nil {
			return nil, err
		}
		return next.Mutate(ctx, m)
	})
}

// A concurrent double accept: the Membership another accept inserted first is not an error,
// and the explicit Invitation's admin Role still raises it.
func TestAcceptInvitationToleratesAMembershipInsertedFirst(t *testing.T) {
	env := testhelper.Setup(t)
	acc := users.New(env.DB, env.Bus, testhelper.Passwords(), env.ConsentRecords, time.Now)
	inv, _, err := invitations.New(env.DB, env.Bus, time.Now).Invite(t.Context(), env.DB.Scoped(fixtures.AcmeID), events.Actor{Kind: events.ActorUser},
		invitations.InviteInput{Email: fixtures.OutsiderOscarEmail, Role: invitation.RoleAdmin, InvitedBy: fixtures.OwnerJohnID})
	require.NoError(t, err)
	env.Bus.Use(hook.On(firstInsertMember, ent.OpCreate))

	err = acc.AcceptInvitation(t.Context(), inv, users.InvitationAcceptance{SessionEmail: fixtures.OutsiderOscarEmail})

	require.NoError(t, err)
	oscar := env.DB.User.Query().Where(user.EmailEqualFold(fixtures.OutsiderOscarEmail)).OnlyX(t.Context())
	memberships := env.DB.Scoped(fixtures.AcmeID).Membership().Query().Where(membership.UserID(oscar.ID)).AllX(t.Context())
	require.Len(t, memberships, 1)
	assert.Equal(t, membership.RoleAdmin, memberships[0].Role)
}

// An Invitation's link is accepted by its token: a token that names no pending Invitation is
// refused before anything sent is checked, and a password sent must pass its rule.
func TestAnInvitationLinkIsAcceptedByItsToken(t *testing.T) {
	env := testhelper.Setup(t)
	acc := users.New(env.DB, env.Bus, testhelper.Passwords(), env.ConsentRecords, time.Now)

	unknown := acc.AcceptInvitationLink(t.Context(), "inv_unknown", users.InvitationAcceptance{Password: "short"})
	short := acc.AcceptInvitationLink(t.Context(), fixtures.InvitationAcmeOscarToken, users.InvitationAcceptance{Password: "short", SessionEmail: fixtures.OutsiderOscarEmail})
	accepted := acc.AcceptInvitationLink(t.Context(), fixtures.InvitationAcmeOscarToken, users.InvitationAcceptance{SessionEmail: fixtures.OutsiderOscarEmail})

	require.ErrorIs(t, unknown, invitations.ErrInvitationLinkNotFound)
	fields, ok := errors.AsType[*validation.FieldsError](short)
	require.True(t, ok, "got %v", short)
	assert.Contains(t, fields.Fields, "password")
	require.NoError(t, accepted)
	joined, err := env.DB.Membership.Query().Where(membership.UserID(fixtures.OutsiderOscarID), membership.WorkspaceID(fixtures.AcmeID)).Exist(t.Context())
	require.NoError(t, err)
	assert.True(t, joined)
}
