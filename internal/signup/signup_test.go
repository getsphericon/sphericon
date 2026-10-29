package signup_test

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/authattempt"
	"github.com/getsphericon/sphericon/ent/user"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/credentials"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/signup"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/validation"
)

// store is the adapter's side of the seam over accounts, the way /site supplies it.
type store struct{ acc *accounts.Accounts }

func (s store) Open(ctx context.Context) (bool, error) { return s.acc.SignupOpen(ctx) }

func (s store) Lookup(ctx context.Context, email string) (*ent.User, bool, error) {
	u, err := s.acc.UserByEmail(ctx, email)
	if ent.IsNotFound(err) {
		return nil, false, nil
	}
	return u, err == nil, err
}

func (s store) CreateUnverified(ctx context.Context, name, email, hash string, surveyDue bool) (*ent.User, error) {
	u, err := s.acc.CreateUnverified(ctx, name, email, hash, surveyDue)
	if db.IsUniqueViolation(err) {
		return nil, signup.ErrTaken
	}
	return u, err
}

func (s store) ReplaceUnverifiedPassword(ctx context.Context, id int64, hash string) (*ent.User, error) {
	return s.acc.ReplaceUnverifiedPassword(ctx, id, hash)
}

func (s store) CreateVerified(ctx context.Context, name, email, hash string, surveyDue bool) (*ent.User, error) {
	u, err := s.acc.SignUpVerified(ctx, name, email, hash, surveyDue)
	if errors.Is(err, accounts.ErrSignupClosed) {
		return nil, signup.ErrClosed
	}
	if db.IsUniqueViolation(err) {
		return nil, signup.ErrTaken
	}
	return u, err
}

// lateStore is a store whose first lookup misses: the Sign-up that lost a race on a new
// address looked before the winner's User was written.
type lateStore struct {
	store

	looked bool
}

func (s *lateStore) Lookup(ctx context.Context, email string) (*ent.User, bool, error) {
	if !s.looked {
		s.looked = true
		return nil, false, nil
	}
	return s.store.Lookup(ctx, email)
}

// job is a mail job the Sign-up left queued.
type job struct {
	Kind  string
	To    string
	Token string
	Send  bool
}

// queue is the mail queue: an outside boundary, read for what was left on it.
type queue struct{ jobs []job }

func (q *queue) EnqueueEmailVerification(_ context.Context, email, token string, send bool) error {
	q.jobs = append(q.jobs, job{Kind: "verification", To: email, Token: token, Send: send})
	return nil
}

func (q *queue) EnqueueAddressTakenNotice(_ context.Context, email string, send bool) error {
	q.jobs = append(q.jobs, job{Kind: "taken", To: email, Send: send})
	return nil
}

func (q *queue) EnqueueWelcome(_ context.Context, email, _ string) error {
	q.jobs = append(q.jobs, job{Kind: "welcome", To: email, Send: true})
	return nil
}

// mint names the User and the Sign-up browser hash a token was minted for.
func mint(u *ent.User, browserHash string) (string, error) {
	return tokenFor(u.ID, browserHash), nil
}

func tokenFor(id int64, browserHash string) string {
	return strconv.FormatInt(id, 10) + ":" + browserHash
}

// linkOf is the token a link minted for the User id carries after the Sign-up res.
func linkOf(id int64, res signup.Result) string { return tokenFor(id, signup.BrowserHash(res.Browser)) }

type rig struct {
	db     *ent.Client
	queue  *queue
	module *signup.Module
}

type option func(*accounts.Accounts, *signup.Config)

// verificationOff runs the rig on an Instance that does not require email verification.
func verificationOff(_ *accounts.Accounts, cfg *signup.Config) { cfg.RequireVerification = false }

// newRig builds the module over the fixtures, with a mail budget of budget per address.
func newRig(t *testing.T, budget int, signupRule accounts.SignupRule, opts ...option) rig {
	t.Helper()
	env := testhelper.Setup(t)
	return build(env.DB, env.Bus, budget, signupRule, opts...)
}

// newRacingRig builds the module over a store whose first lookup misses, on a database of the
// test's own: the create that meets the winner's row fails a statement, which inside the one
// transaction of the ordinary tests would abort everything after it.
func newRacingRig(t *testing.T) rig {
	t.Helper()
	sqlDB := testhelper.RealDB(t)
	return build(db.NewEntClient(sqlDB), events.New(sqlDB), 5, accounts.SignupAlways,
		func(acc *accounts.Accounts, cfg *signup.Config) { cfg.Store = &lateStore{store: store{acc}} })
}

func build(client *ent.Client, bus *events.Bus, budget int, signupRule accounts.SignupRule, opts ...option) rig {
	acc := accounts.New(client, bus, testhelper.Passwords(), accounts.WithSignupRule(signupRule))
	attempts := accounts.NewAttempts(client, accounts.WithRule(accounts.KindSignUp, accounts.ResetRule(budget)))
	q := &queue{}
	cfg := signup.Config{
		Store: store{acc}, Budget: attempts.For(accounts.KindSignUp), Mail: q, Mint: mint, Passwords: testhelper.Passwords(),
		RequireVerification: true, SurveyDue: func() bool { return false },
	}
	for _, o := range opts {
		o(acc, &cfg)
	}
	return rig{db: client, queue: q, module: signup.New(cfg)}
}

func (r rig) signUp(t *testing.T, name, email, password string) signup.Result {
	t.Helper()
	res, err := r.module.SignUp(t.Context(), signup.Input{Name: name, Email: email, Password: password})
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
	taken, err := r.db.AuthAttempt.Query().Where(authattempt.Email(email), authattempt.KindEQ(accounts.KindSignUp)).Exist(t.Context())
	require.NoError(t, err)
	return taken
}

const password = "form-password-1"

// ADR 0031: a new, an Unverified and a Verified address answer alike and cost the same
// work: a budget slot, one Sign-up browser and exactly one mail job each. Only the job's
// kind says which it was, and only its owner reads it.
func TestEverySignUpDoesTheSameWorkWhateverTheAddress(t *testing.T) {
	r := newRig(t, 5, accounts.SignupAlways)
	john := r.user(t, fixtures.OwnerJohnEmail)

	fresh := r.signUp(t, "Grace", "grace@example.com", password)
	unverified := r.signUp(t, "Mallory", fixtures.FreshUnverifiedUserEmail, password)
	verified := r.signUp(t, "Mallory", fixtures.OwnerJohnEmail, password)

	assert.Equal(t, signup.Accepted, fresh.Outcome)
	assert.Equal(t, signup.Accepted, unverified.Outcome)
	assert.Equal(t, signup.Accepted, verified.Outcome)
	assert.NotEmpty(t, fresh.Browser)
	assert.NotEmpty(t, unverified.Browser)
	assert.NotEmpty(t, verified.Browser)
	assert.True(t, r.budgetTaken(t, "grace@example.com"))
	assert.True(t, r.budgetTaken(t, fixtures.FreshUnverifiedUserEmail))
	assert.True(t, r.budgetTaken(t, fixtures.OwnerJohnEmail))
	grace := r.user(t, "grace@example.com")
	assert.Equal(t, []job{
		{Kind: "verification", To: "grace@example.com", Token: linkOf(grace.ID, fresh), Send: true},
		{Kind: "verification", To: fixtures.FreshUnverifiedUserEmail, Token: linkOf(fixtures.FreshUnverifiedUserID, unverified), Send: true},
		{Kind: "taken", To: fixtures.OwnerJohnEmail, Send: true},
	}, r.queue.jobs, "one job each, and only the taken address's carries no link")
	after := r.user(t, fixtures.OwnerJohnEmail)
	assert.Equal(t, john.PasswordHash, after.PasswordHash, "a Verified User's password is never replaced")
	assert.Equal(t, john.Name, after.Name)
}

func TestASignUpOfANewAddressCreatesAnUnverifiedUserAndMailsItsLink(t *testing.T) {
	r := newRig(t, 5, accounts.SignupAlways)

	res := r.signUp(t, "  Grace  ", "  grace@example.com ", password)

	u := r.user(t, "grace@example.com")
	assert.Equal(t, "Grace", u.Name)
	assert.Nil(t, u.EmailVerifiedAt)
	assert.True(t, credentials.VerifyPassword(u.PasswordHash, password))
	assert.Equal(t, []job{{Kind: "verification", To: "grace@example.com", Token: linkOf(u.ID, res), Send: true}}, r.queue.jobs,
		"the link is the User's and trusts the Sign-up browser of the answer")
}

// ADR 0030: a repeated Sign-up of an Unverified address replaces its password, which ends
// the earlier links, and keeps the name it was first given.
func TestARepeatedSignUpOfAnUnverifiedAddressReplacesItsPassword(t *testing.T) {
	r := newRig(t, 5, accounts.SignupAlways)
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

	assert.Equal(t, signup.Accepted, res.Outcome)
	assert.NotEmpty(t, res.Browser)
	winner := r.user(t, fixtures.FreshUnverifiedUserEmail)
	assert.True(t, credentials.VerifyPassword(winner.PasswordHash, password), "the loser replaced the winner's password")
	assert.Equal(t, []job{{Kind: "verification", To: fixtures.FreshUnverifiedUserEmail, Token: linkOf(winner.ID, res), Send: true}}, r.queue.jobs)
}

// ADR 0031: once the address's mail budget is spent a Sign-up still does all the work, and
// its mail job is queued but delivers nothing.
func TestASignUpPastTheMailBudgetQueuesAJobThatDeliversNothing(t *testing.T) {
	r := newRig(t, 1, accounts.SignupAlways)

	r.signUp(t, "Mallory", fixtures.OwnerJohnEmail, password)
	res := r.signUp(t, "Mallory", fixtures.OwnerJohnEmail, password)

	assert.Equal(t, signup.Accepted, res.Outcome)
	assert.Equal(t, []bool{true, false}, []bool{r.queue.jobs[0].Send, r.queue.jobs[1].Send})
}

// ADR 0042, 0060: a closed sign-up refuses before anything is read or created.
func TestAClosedSignUpReadsAndCreatesNothing(t *testing.T) {
	r := newRig(t, 5, accounts.SignupUntilFirstUser)

	res := r.signUp(t, "Grace", "grace@example.com", password)

	assert.Equal(t, signup.Closed, res.Outcome)
	assert.Empty(t, res.Browser)
	assert.Empty(t, r.queue.jobs)
	assert.False(t, r.budgetTaken(t, "grace@example.com"))
	exists, err := r.db.User.Query().Where(user.Email("grace@example.com")).Exist(t.Context())
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestARefusedFormNamesEveryFieldAndCostsNothing(t *testing.T) {
	r := newRig(t, 5, accounts.SignupAlways)

	_, err := r.module.SignUp(t.Context(), signup.Input{Name: "  ", Email: "not-an-address", Password: "short"})

	fields, ok := errors.AsType[*validation.FieldsError](err)
	require.True(t, ok, "got %v", err)
	refused := slices.Sorted(maps.Keys(fields.Fields))
	assert.Equal(t, []string{"email", "name", "password"}, refused)
	assert.Empty(t, r.queue.jobs)
	assert.False(t, r.budgetTaken(t, "not-an-address"))
}

// ADR 0030: with verification off the User is Verified at once and welcomed, and no Sign-up
// browser is drawn: no link follows.
func TestWithVerificationOffASignUpMakesAVerifiedUserAndWelcomesIt(t *testing.T) {
	r := newRig(t, 5, accounts.SignupAlways, verificationOff)

	res := r.signUp(t, "Grace", "grace@example.com", password)

	assert.Equal(t, signup.Accepted, res.Outcome)
	assert.Empty(t, res.Browser)
	u := r.user(t, "grace@example.com")
	assert.NotNil(t, u.EmailVerifiedAt)
	assert.Equal(t, []job{{Kind: "welcome", To: "grace@example.com", Send: true}}, r.queue.jobs)
}

// ADR 0031's one exception: with no mail to tell the owner, a Taken address is the answer.
func TestWithVerificationOffATakenAddressIsTheAnswer(t *testing.T) {
	r := newRig(t, 5, accounts.SignupAlways, verificationOff)
	john := r.user(t, fixtures.OwnerJohnEmail)

	res := r.signUp(t, "Mallory", fixtures.OwnerJohnEmail, password)

	assert.Equal(t, signup.Taken, res.Outcome)
	assert.Empty(t, r.queue.jobs)
	assert.Equal(t, john.PasswordHash, r.user(t, fixtures.OwnerJohnEmail).PasswordHash)
}

func TestTheSignUpBrowserIsTheOneWhoseHashTheLinkCarries(t *testing.T) {
	r := newRig(t, 5, accounts.SignupAlways)
	res := r.signUp(t, "Grace", "grace@example.com", password)
	other := r.signUp(t, "Alan", "alan@example.com", password)

	hash := signup.BrowserHash(res.Browser)

	assert.True(t, signup.SameBrowser(hash, res.Browser))
	assert.False(t, signup.SameBrowser(hash, other.Browser))
	assert.False(t, signup.SameBrowser("", res.Browser), "a link minted without a browser trusts none")
	assert.False(t, signup.SameBrowser(hash, ""))
}
