package users_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/hook"
	"github.com/getsphericon/sphericon/ent/invitation"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/user"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/invitations"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/users"
)

// A Verified User is attached only with their own session, decided inside the
// transaction: whatever the caller believed about the address, no session is refused.
func TestAcceptInvitationOfAVerifiedUserNeedsTheirSession(t *testing.T) {
	env := testhelper.Setup(t)
	acc := users.New(env.DB, env.Bus, testhelper.Passwords(), env.ConsentRecords)
	inv, _, err := invitations.New(env.DB, env.Bus).Invite(t.Context(), env.DB.Scoped(fixtures.AcmeID), events.Actor{Kind: events.ActorUser},
		invitations.InviteInput{Email: fixtures.OutsiderOscarEmail, Role: invitation.RoleMember, InvitedBy: fixtures.OwnerJohnID})
	require.NoError(t, err)

	require.ErrorIs(t, acc.AcceptInvitation(t.Context(), inv, users.InvitationAcceptance{}), users.ErrSignInRequired)
	require.ErrorIs(t, acc.AcceptInvitation(t.Context(), inv, users.InvitationAcceptance{SessionEmail: fixtures.OwnerJaneEmail}), users.ErrWrongAccount)
	require.NoError(t, acc.AcceptInvitation(t.Context(), inv, users.InvitationAcceptance{SessionEmail: fixtures.OutsiderOscarEmail}))
}

// firstInsertMember is an ent hook that, just before an invitation's Membership is
// written, lets a concurrent accept win the race: it inserts the User's `member` Membership.
func firstInsertMember(next ent.Mutator) ent.Mutator {
	return hook.MembershipFunc(func(ctx context.Context, m *ent.MembershipMutation) (ent.Value, error) {
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
	acc := users.New(env.DB, env.Bus, testhelper.Passwords(), env.ConsentRecords)
	inv, _, err := invitations.New(env.DB, env.Bus).Invite(t.Context(), env.DB.Scoped(fixtures.AcmeID), events.Actor{Kind: events.ActorUser},
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
