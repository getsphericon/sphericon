package noticedelivery_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/domain/identity/suspension"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The Notices are read the way their recipient reads them: an event is published as its producer
// publishes it, and env.SystemMail works the Notices' consumer and the sends before a read.

// publish publishes ev in a transaction of its own, as a request that changes no state does.
func publish(t *testing.T, env *testhelper.TestEnv, ev events.DomainEvent) {
	t.Helper()
	require.NoError(t, env.Bus.Publish(context.Background(), ev))
}

// A link request that is delivered mails the address the page its link opens, with a token minted
// for the User; each request opens its own page.
func TestALinkRequestMailsItsLink(t *testing.T) {
	cases := map[string]struct {
		userID      int64
		email, page string
	}{
		events.NameUserPasswordResetRequested: {fixtures.OwnerJohnID, fixtures.OwnerJohnEmail, "/reset-password?token="},
		events.NameUserVerificationRequested:  {fixtures.FreshUnverifiedUserID, fixtures.FreshUnverifiedUserEmail, "/verify-email?token="},
		events.NameUserEmailChangeRequested:   {fixtures.OwnerJohnID, "john.new@example.com", "/confirm-email-change?token="},
		// Only a User with no password confirms a deletion with a link (ADR 0050).
		events.NameUserDeletionRequested: {fixtures.ProviderOnlyPiaID, fixtures.ProviderOnlyPiaEmail, "/delete-account?token="},
	}
	for action, c := range cases {
		t.Run(action, func(t *testing.T) {
			env := testhelper.Setup(t)

			publish(t, env, &events.LinkRequested{Action: action, UserID: c.userID, Email: c.email, Deliver: true})

			msgs := env.SystemMail.Messages()
			require.Len(t, msgs, 1)
			assert.Equal(t, c.email, msgs[0].To)
			assert.Contains(t, msgs[0].Text, env.AppURL+c.page)
		})
	}
}

// A request published only so that every request does the same work (an unknown address, a spent
// budget) mails nothing, and neither does one for a User gone since.
func TestALinkRequestThatIsNotDeliveredMailsNothing(t *testing.T) {
	env := testhelper.Setup(t)

	publish(t, env, &events.LinkRequested{Action: events.NameUserPasswordResetRequested, UserID: fixtures.OwnerJohnID, Email: fixtures.OwnerJohnEmail})
	publish(t, env, &events.LinkRequested{Action: events.NameUserPasswordResetRequested, Email: "nobody@example.com", Deliver: true})
	publish(t, env, &events.LinkRequested{Action: events.NameUserPasswordResetRequested, UserID: 999999, Email: "gone@example.com", Deliver: true})

	assert.Empty(t, env.SystemMail.Messages())
}

// The Taken-address notice (ADR 0031) carries no token: only the sign-in and the forgot-password
// pages, so it can never be a way into the account.
func TestTheTakenAddressNoticeCarriesNoToken(t *testing.T) {
	env := testhelper.Setup(t)

	publish(t, env, &events.AddressClaimed{Email: fixtures.OwnerJohnEmail, Deliver: true})
	publish(t, env, &events.AddressClaimed{Email: fixtures.OwnerJaneEmail})

	msgs := env.SystemMail.Messages()
	require.Len(t, msgs, 1, "only the delivered claim is mailed")
	assert.Equal(t, fixtures.OwnerJohnEmail, msgs[0].To)
	assert.NotContains(t, msgs[0].Text, "token")
	assert.Contains(t, msgs[0].Text, env.AppURL+"/login")
	assert.Contains(t, msgs[0].Text, env.AppURL+"/forgot-password")
}

// A completed Sign-up is welcomed by name, and by a greeting of its own without one.
func TestACompletedSignUpIsWelcomed(t *testing.T) {
	env := testhelper.Setup(t)

	publish(t, env, &events.UserSignedUp{UserID: fixtures.OwnerJohnID, Email: fixtures.OwnerJohnEmail, Name: "Wendy"})
	publish(t, env, &events.UserSignedUp{UserID: fixtures.OwnerJaneID, Email: fixtures.OwnerJaneEmail})

	msgs := env.SystemMail.Messages()
	require.Len(t, msgs, 2)
	assert.Contains(t, msgs[0].Text, "Wendy")
	assert.Contains(t, msgs[1].Text, i18n.T(i18n.EmailWelcomeGreetingFallback, nil))
}

// An Invitation mails its link, naming the Workspace and the inviter; an inviter gone since is
// "Someone".
func TestAnInvitationMailsItsLink(t *testing.T) {
	env := testhelper.Setup(t)

	publish(t, env, &events.InvitationCreated{WorkspaceID: fixtures.AcmeID, InvitationID: 1, Email: "i@example.com", Token: "tok-1", InvitedBy: fixtures.OwnerJohnID})
	publish(t, env, &events.InvitationCreated{WorkspaceID: fixtures.AcmeID, InvitationID: 2, Email: "j@example.com", Token: "tok-2", InvitedBy: 999999})

	msgs := env.SystemMail.Messages()
	require.Len(t, msgs, 2)
	john := env.DB.User.GetX(t.Context(), fixtures.OwnerJohnID)
	assert.Equal(t, "You're invited to Acme on sphericon", msgs[0].Subject)
	assert.Contains(t, msgs[0].Text, john.Name+" invited you")
	assert.Contains(t, msgs[0].Text, env.AppURL+"/invitations/tok-1")
	assert.Contains(t, msgs[1].Text, "Someone invited you")
}

// support is the Instance's support contact a suspended Workspace's owners appeal to.
const support = "support@sphericon.test"

// The owners are told that sending is frozen, why, who did it and where to appeal, through the
// platform's System sender, which keeps working while the Workspace itself is suspended (ADR
// 0007, 0015); the notice is recorded on the Workspace.
func TestASuspensionTellsTheOwners(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithConfig(func(c *config.Config) { c.SupportEmail = support }))

	_, err := suspension.SuspendWorkspace(t.Context(), env.Bus, fixtures.AcmeID, suspension.System, "complaint rate above 0.3%", time.Now())
	require.NoError(t, err)

	msgs := env.SystemMail.Messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, fixtures.OwnerJohnEmail, msgs[0].To, "the workspace owner")
	assert.Contains(t, msgs[0].Subject, "Acme")
	assert.Contains(t, msgs[0].Text, "complaint rate above 0.3%", "the reason is stated")
	assert.Contains(t, msgs[0].Text, "automated abuse detection", "who set it is stated")
	assert.Contains(t, msgs[0].Text, support)
	ws := env.DB.Workspace.GetX(t.Context(), fixtures.AcmeID)
	assert.NotNil(t, ws.AbuseFreezeNotifiedAt, "the notice is recorded")
}

// A Staff member or the CLI appears to the owners as "Sphericon staff"; the Staff member's real id
// never leaves the platform.
func TestASuspensionNamesStaffForStaffAndCLI(t *testing.T) {
	for name, by := range map[string]suspension.Actor{"staff": suspension.StaffMember("op-42"), "cli": suspension.CLI} {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)

			_, err := suspension.SuspendWorkspace(t.Context(), env.Bus, fixtures.AcmeID, by, "abuse report", time.Now())
			require.NoError(t, err)

			msgs := env.SystemMail.Messages()
			require.Len(t, msgs, 1)
			assert.Contains(t, msgs[0].Text, "Set by: Sphericon staff")
			assert.NotContains(t, msgs[0].Text, "op-42")
		})
	}
}

// A Workspace suspended by the abuse detector names the system as the actor, one suspended from
// the command line Sphericon staff.
func TestASuspensionNoticeNamesSystemOrSphericonStaffAsTheActor(t *testing.T) {
	for name, tc := range map[string]struct {
		id  int64
		key i18n.ID
	}{
		"system": {fixtures.NakatomiID, i18n.EmailWorkspaceSuspendedActorSystem},
		"cli":    {fixtures.SiriusID, i18n.EmailWorkspaceSuspendedActorStaff},
	} {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)

			publish(t, env, &events.WorkspaceSuspended{WorkspaceID: tc.id})

			msgs := env.SystemMail.Messages()
			require.Len(t, msgs, 1)
			assert.Contains(t, msgs[0].Text, i18n.T(tc.key, nil))
		})
	}
}

// A suspension lifted before its notice is handled tells nobody.
func TestALiftedSuspensionTellsNobody(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := suspension.SuspendWorkspace(t.Context(), env.Bus, fixtures.AcmeID, suspension.System, "complaints", time.Now())
	require.NoError(t, err)
	_, err = suspension.UnsuspendWorkspace(t.Context(), env.Bus, fixtures.AcmeID, suspension.CLI, time.Now())
	require.NoError(t, err)

	assert.Empty(t, env.SystemMail.Messages())
}
