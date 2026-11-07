package users_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/throttle"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// requests are the account's requests over the test's database, with a mail budget of one slot per
// address for the password reset and the email change.
func requests(env *testhelper.TestEnv) *users.Requests {
	attempts := throttle.NewAttempts(env.DB, time.Now,
		throttle.WithRule(throttle.KindPasswordReset, throttle.ResetRule(1)),
		throttle.WithRule(throttle.KindEmailChange, throttle.ResetRule(1)))
	return users.NewRequests(users.New(env.DB, env.Bus, testhelper.Passwords(), env.ConsentRecords, time.Now), attempts)
}

// published are the account's events the test's requests published, oldest first.
func published(t *testing.T, env *testhelper.TestEnv) []events.DomainEvent {
	t.Helper()
	return env.QueuedEvents(t,
		events.NameUserPasswordResetRequested, events.NameUserVerificationRequested,
		events.NameUserEmailChangeRequested, events.NameUserDeletionRequested, events.NameUserAddressClaimed)
}

// ADR 0031: a known and an unknown address do the same work and each publish one request; only the
// known one, within its budget, is delivered, to the address as the User holds it.
func TestAPasswordResetIsPublishedForEveryAddressAndDeliveredOnlyToAUser(t *testing.T) {
	env := testhelper.Setup(t)
	r := requests(env)

	require.NoError(t, r.PasswordReset(t.Context(), "  "+fixtures.OwnerJohnEmail+" "))
	require.NoError(t, r.PasswordReset(t.Context(), "nobody@example.com"))
	require.NoError(t, r.PasswordReset(t.Context(), fixtures.OwnerJohnEmail))
	require.NoError(t, r.PasswordReset(t.Context(), "   "))

	assert.Equal(t, []events.DomainEvent{
		&events.LinkRequested{Action: events.NameUserPasswordResetRequested, UserID: fixtures.OwnerJohnID, Email: fixtures.OwnerJohnEmail, Deliver: true},
		&events.LinkRequested{Action: events.NameUserPasswordResetRequested, Email: "nobody@example.com"},
		&events.LinkRequested{Action: events.NameUserPasswordResetRequested, UserID: fixtures.OwnerJohnID, Email: fixtures.OwnerJohnEmail},
	}, published(t, env), "the second request of the address is past its budget; a blank address asks for nothing")
}

// The verification link is sent again only to an Unverified User, trusting the Sign-up browser of
// the request.
func TestAVerificationIsDeliveredOnlyToAnUnverifiedUser(t *testing.T) {
	env := testhelper.Setup(t)
	r := requests(env)

	require.NoError(t, r.Verification(t.Context(), fixtures.FreshUnverifiedUserEmail, "browser-1"))
	require.NoError(t, r.Verification(t.Context(), fixtures.OwnerJohnEmail, ""))

	assert.Equal(t, []events.DomainEvent{
		&events.LinkRequested{
			Action: events.NameUserVerificationRequested, UserID: fixtures.FreshUnverifiedUserID, Email: fixtures.FreshUnverifiedUserEmail,
			BrowserHash: users.BrowserHash("browser-1"), Deliver: true,
		},
		&events.LinkRequested{
			Action: events.NameUserVerificationRequested, UserID: fixtures.OwnerJohnID, Email: fixtures.OwnerJohnEmail,
			BrowserHash: users.BrowserHash(""),
		},
	}, published(t, env), "a Verified User's request is not delivered")
}

// ADR 0031: a free new address gets the confirm link, a Taken one the claim its owner is told of;
// the requester cannot tell them apart.
func TestAnEmailChangeAsksForTheLinkOrClaimsATakenAddress(t *testing.T) {
	env := testhelper.Setup(t)
	r := requests(env)
	john := env.DB.User.GetX(t.Context(), fixtures.OwnerJohnID)

	require.NoError(t, r.EmailChange(t.Context(), john, "john.new@example.com"))
	require.NoError(t, r.EmailChange(t.Context(), john, fixtures.OwnerJaneEmail))

	assert.Equal(t, []events.DomainEvent{
		&events.LinkRequested{Action: events.NameUserEmailChangeRequested, UserID: fixtures.OwnerJohnID, Email: "john.new@example.com", Deliver: true},
		&events.AddressClaimed{Email: fixtures.OwnerJaneEmail, Deliver: true},
	}, published(t, env))
}

// ADR 0050: only a User with no password asks for a deletion link, with their own address; a User
// with a password confirms with it and asks for nothing.
func TestADeletionLinkIsAskedOnlyByAUserWithNoPassword(t *testing.T) {
	env := testhelper.Setup(t)
	r := requests(env)

	require.NoError(t, r.DeletionLink(t.Context(), fixtures.ProviderOnlyPiaID, fixtures.ProviderOnlyPiaEmail))
	err := r.DeletionLink(t.Context(), fixtures.OwnerJohnID, fixtures.OwnerJohnEmail)

	require.ErrorIs(t, err, users.ErrDeletionUsePassword)
	assert.Equal(t, []events.DomainEvent{
		&events.LinkRequested{Action: events.NameUserDeletionRequested, UserID: fixtures.ProviderOnlyPiaID, Email: fixtures.ProviderOnlyPiaEmail, Deliver: true},
	}, published(t, env))
}
