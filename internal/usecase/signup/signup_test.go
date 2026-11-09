package signup_test

import (
	"context"
	"database/sql"
	"errors"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/authattempt"
	"github.com/getsphericon/sphericon/ent/user"
	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/domain/identity/consentrecords"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/throttle"
	"github.com/getsphericon/sphericon/internal/kernel/authtoken"
	"github.com/getsphericon/sphericon/internal/kernel/credentials"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/signup"
)

// requests are the events a Sign-up publishes for its mail, read from what the publish left in
// the queue: the request of a verification link, the claim of a Taken address, the completed
// Sign-up.
type requests struct{ sqlDB *sql.DB }

// published are the mail events published so far, oldest first.
func (q requests) published(t *testing.T) []events.DomainEvent {
	t.Helper()
	envelopes := testhelper.QueuedEnvelopesOn(t, q.sqlDB, events.NameUserVerificationRequested, events.NameUserAddressClaimed, events.NameUserSignedUp)
	out := make([]events.DomainEvent, 0, len(envelopes))
	for i := range envelopes {
		ev, err := events.Decode(envelopes[i])
		require.NoError(t, err)
		out = append(out, ev)
	}
	return out
}

// verificationRequested is the request of a verification link for the User id at email that
// trusts the Sign-up browser of the answer res, delivered when the budget granted it.
func verificationRequested(id int64, email string, res signup.SignUpResult, deliver bool) *events.LinkRequested {
	return &events.LinkRequested{
		Action: events.NameUserVerificationRequested, UserID: id, Email: email,
		BrowserHash: users.BrowserHash(res.Browser), Deliver: deliver,
	}
}

type rig struct {
	env      *testhelper.TestEnv
	db       *ent.Client
	queue    requests
	module   *signup.UseCase
	consents *consentrecords.Module
	links    *users.Links
}

type option func(*users.Module, *signup.SignUpConfig)

// verificationOff runs the rig on an Instance that does not require email verification.
func verificationOff(_ *users.Module, cfg *signup.SignUpConfig) {
	cfg.RequireVerification = false
}

// newRig builds the module over the fixtures, with a mail budget of budget per address.
func newRig(t *testing.T, budget int, opts ...option) rig {
	t.Helper()
	env := testhelper.Setup(t)
	r := build(t, env.SQLDB, env.DB, env.Bus, budget, opts...)
	r.env = env
	return r
}

// newRacingRig builds the module with a first lookup that misses, as the lookup of a Sign-up that
// lost a race on a new address did before the winner's User was written, on a database of the
// test's own: the create that meets the winner's row fails a statement, which inside the one
// transaction of the ordinary tests would abort everything after it.
func newRacingRig(t *testing.T) rig {
	t.Helper()
	sqlDB := testhelper.RealDB(t)
	return build(t, sqlDB, entclient.New(sqlDB), testhelper.BusOn(t, sqlDB), 5, func(acc *users.Module, cfg *signup.SignUpConfig) {
		looked := false
		cfg.Lookup = func(ctx context.Context, email string) (*ent.User, bool, error) {
			if !looked {
				looked = true
				return nil, false, nil
			}
			u, err := acc.UserByEmail(ctx, email)
			if ent.IsNotFound(err) {
				return nil, false, nil
			}
			return u, err == nil, err
		}
	})
}

func build(t *testing.T, sqlDB *sql.DB, client *ent.Client, bus *events.Bus, budget int, opts ...option) rig {
	t.Helper()
	consents := testhelper.ConsentRecords(client, bus)
	acc := users.New(client, bus, testhelper.Passwords(), consents, time.Now)
	attempts := throttle.NewAttempts(client, time.Now, throttle.WithRule(throttle.KindSignUp, throttle.ResetRule(budget)))
	q := requests{sqlDB: sqlDB}
	cfg := signup.SignUpConfig{
		Budget: attempts.For(throttle.KindSignUp), Consents: consents,
		RequireVerification: true, SurveyDue: func() bool { return false },
	}
	for _, o := range opts {
		o(acc, &cfg)
	}
	links := users.NewLinks(acc, authtoken.New("secret", time.Now))
	module := signup.New(bus, acc, links, testhelper.Passwords(), nil, cfg, signup.SignInConfig{}, time.Now)
	return rig{db: client, queue: q, module: module, consents: consents, links: links}
}

func (r rig) signUp(t *testing.T, name, email, password string) signup.SignUpResult {
	t.Helper()
	res, err := r.module.SignUp(t.Context(), signup.SignUpInput{Name: name, Email: email, Password: password})
	require.NoError(t, err)
	return res
}

func (r rig) user(t *testing.T, email string) *ent.User {
	t.Helper()
	u, err := r.db.User.Query().Where(user.Email(email)).Only(t.Context())
	require.NoError(t, err)
	return u
}

// budgetTaken reports whether a slot of the address's mail budget was taken.
func (r rig) budgetTaken(t *testing.T, email string) bool {
	t.Helper()
	taken, err := r.db.AuthAttempt.Query().Where(authattempt.Email(email), authattempt.KindEQ(throttle.KindSignUp)).Exist(t.Context())
	require.NoError(t, err)
	return taken
}

const password = "form-password-1"

// ADR 0031: a new, an Unverified and a Verified address answer alike and cost the same
// work: a budget slot, one Sign-up browser and exactly one published event each. Only the
// event says which it was, and only the address's owner reads its mail.
func TestEverySignUpDoesTheSameWorkWhateverTheAddress(t *testing.T) {
	r := newRig(t, 5)
	john := r.user(t, fixtures.OwnerJohnEmail)

	fresh := r.signUp(t, "Grace", "grace@example.com", password)
	unverified := r.signUp(t, "Mallory", fixtures.FreshUnverifiedUserEmail, password)
	verified := r.signUp(t, "Mallory", fixtures.OwnerJohnEmail, password)

	assert.NotEmpty(t, fresh.Browser)
	assert.NotEmpty(t, unverified.Browser)
	assert.NotEmpty(t, verified.Browser)
	assert.True(t, r.budgetTaken(t, "grace@example.com"))
	assert.True(t, r.budgetTaken(t, fixtures.FreshUnverifiedUserEmail))
	assert.True(t, r.budgetTaken(t, fixtures.OwnerJohnEmail))
	grace := r.user(t, "grace@example.com")
	assert.Equal(t, []events.DomainEvent{
		verificationRequested(grace.ID, "grace@example.com", fresh, true),
		verificationRequested(fixtures.FreshUnverifiedUserID, fixtures.FreshUnverifiedUserEmail, unverified, true),
		&events.AddressClaimed{Email: fixtures.OwnerJohnEmail, Deliver: true},
	}, r.queue.published(t), "one event each, and the taken address's asks for no link")
	after := r.user(t, fixtures.OwnerJohnEmail)
	assert.Equal(t, john.PasswordHash, after.PasswordHash, "a Verified User's password is never replaced")
	assert.Equal(t, john.Name, after.Name)
}

func TestASignUpOfANewAddressCreatesAnUnverifiedUserAndMailsItsLink(t *testing.T) {
	r := newRig(t, 5)

	res := r.signUp(t, "  Grace  ", "  grace@example.com ", password)

	u := r.user(t, "grace@example.com")
	assert.Equal(t, "Grace", u.Name)
	assert.Nil(t, u.EmailVerifiedAt)
	assert.True(t, credentials.VerifyPassword(u.PasswordHash, password))
	assert.Equal(t, []events.DomainEvent{verificationRequested(u.ID, "grace@example.com", res, true)}, r.queue.published(t),
		"the link is the User's and trusts the Sign-up browser of the answer")
}

// ADR 0030: a repeated Sign-up of an Unverified address replaces its password, which ends
// the earlier links, and keeps the name it was first given.
func TestARepeatedSignUpOfAnUnverifiedAddressReplacesItsPassword(t *testing.T) {
	r := newRig(t, 5)
	before := r.user(t, fixtures.FreshUnverifiedUserEmail)

	r.signUp(t, "Mallory", fixtures.FreshUnverifiedUserEmail, password)

	after := r.user(t, fixtures.FreshUnverifiedUserEmail)
	assert.True(t, credentials.VerifyPassword(after.PasswordHash, password))
	assert.Equal(t, before.Name, after.Name)
	assert.Nil(t, after.EmailVerifiedAt)
}

// Two Sign-ups of one new address race: the loser's lookup missed, its create meets the
// winner's row on the unique email, and it ends as a repeated Sign-up of the winner.
func TestASignUpThatLosesARaceEndsAsARepeatedSignUpOfTheWinner(t *testing.T) {
	r := newRacingRig(t)

	res := r.signUp(t, "Mallory", fixtures.FreshUnverifiedUserEmail, password)

	assert.NotEmpty(t, res.Browser)
	winner := r.user(t, fixtures.FreshUnverifiedUserEmail)
	assert.True(t, credentials.VerifyPassword(winner.PasswordHash, password), "the loser replaced the winner's password")
	assert.Equal(t, []events.DomainEvent{verificationRequested(winner.ID, fixtures.FreshUnverifiedUserEmail, res, true)}, r.queue.published(t))
}

// ADR 0031: once the address's mail budget is spent a Sign-up still does all the work, and
// its event is published but not delivered.
func TestASignUpPastTheMailBudgetPublishesAnUndeliveredEvent(t *testing.T) {
	r := newRig(t, 1)

	r.signUp(t, "Mallory", fixtures.OwnerJohnEmail, password)
	r.signUp(t, "Mallory", fixtures.OwnerJohnEmail, password)

	assert.Equal(t, []events.DomainEvent{
		&events.AddressClaimed{Email: fixtures.OwnerJohnEmail, Deliver: true},
		&events.AddressClaimed{Email: fixtures.OwnerJohnEmail, Deliver: false},
	}, r.queue.published(t))
}

func TestARefusedFormNamesEveryFieldAndCostsNothing(t *testing.T) {
	r := newRig(t, 5)

	_, err := r.module.SignUp(t.Context(), signup.SignUpInput{Name: "  ", Email: "not-an-address", Password: "short"})

	fields, ok := errors.AsType[*validation.FieldsError](err)
	require.True(t, ok, "got %v", err)
	refused := slices.Sorted(maps.Keys(fields.Fields))
	assert.Equal(t, []string{"email", "name", "password"}, refused)
	assert.Empty(t, r.queue.published(t))
	assert.False(t, r.budgetTaken(t, "not-an-address"))
}

// ADR 0030: with verification off the User is Verified at once and welcomed, and no Sign-up
// browser is drawn: no link follows.
func TestWithVerificationOffASignUpMakesAVerifiedUserAndWelcomesIt(t *testing.T) {
	r := newRig(t, 5, verificationOff)

	res := r.signUp(t, "Grace", "grace@example.com", password)

	assert.Empty(t, res.Browser)
	u := r.user(t, "grace@example.com")
	assert.NotNil(t, u.EmailVerifiedAt)
	assert.Equal(t, []events.DomainEvent{&events.UserSignedUp{UserID: u.ID, Email: "grace@example.com", Name: "Grace"}}, r.queue.published(t))
}

// ADR 0031's one exception: with no mail to tell the owner, a Taken address is the answer.
func TestWithVerificationOffATakenAddressIsTheAnswer(t *testing.T) {
	r := newRig(t, 5, verificationOff)
	john := r.user(t, fixtures.OwnerJohnEmail)

	_, err := r.module.SignUp(t.Context(), signup.SignUpInput{Name: "Mallory", Email: fixtures.OwnerJohnEmail, Password: password})

	require.ErrorIs(t, err, users.ErrEmailTaken)
	assert.Empty(t, r.queue.published(t))
	assert.Equal(t, john.PasswordHash, r.user(t, fixtures.OwnerJohnEmail).PasswordHash)
}

func TestTheSignUpBrowserIsTheOneWhoseHashTheLinkCarries(t *testing.T) {
	r := newRig(t, 5)
	res := r.signUp(t, "Grace", "grace@example.com", password)
	other := r.signUp(t, "Alan", "alan@example.com", password)

	hash := users.BrowserHash(res.Browser)

	assert.True(t, users.SameBrowser(hash, res.Browser))
	assert.False(t, users.SameBrowser(hash, other.Browser))
	assert.False(t, users.SameBrowser("", res.Browser), "a link minted without a browser trusts none")
	assert.False(t, users.SameBrowser(hash, ""))
}
