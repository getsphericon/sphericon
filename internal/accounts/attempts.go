package accounts

import (
	"context"
	stdsql "database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/authattempt"
	"github.com/getsphericon/sphericon/internal/ratelimit"
)

// Kind is the action an attempt counter belongs to. Each kind has its own counter
// per address and its own Rule.
type Kind = authattempt.Kind

const (
	KindLogin         = authattempt.KindLogin
	KindStaffLogin    = authattempt.KindStaffLogin
	KindPasswordReset = authattempt.KindPasswordReset
	KindRegister      = authattempt.KindRegister
	KindEmailChange   = authattempt.KindEmailChange
	// The caps on a Test send from the System sender (ADR 0041), keyed by a User or Workspace
	// id rather than an address (see counterKey).
	KindTestSendUser      = authattempt.KindTestSendUser
	KindTestSendWorkspace = authattempt.KindTestSendWorkspace
)

// The fixed caps on a Test send from the System sender (ADR 0041): a way to cause mail from
// the platform's own domain, so no environment variable raises them.
const (
	TestSendPerUserHour     = 5
	TestSendPerWorkspaceDay = 20
)

// Rule is the throttle of one Kind (ADR 0025): after Threshold failures within
// Window the next attempt must wait Base, doubling with every further failure up to
// Cap, measured from the last failure. A Threshold of 0 disables the Kind: nothing
// is counted and nothing waits.
type Rule struct {
	Threshold int
	Window    time.Duration
	Base      time.Duration
	Cap       time.Duration
}

// ResetRule is the password-reset budget: at most threshold mails per address per
// hour (granted with Take; there is no delay).
func ResetRule(threshold int) Rule { return Rule{Threshold: threshold, Window: time.Hour} }

// LoginRule is the login throttle: threshold failures in 15 minutes, then 1 s, 2 s,
// ... capped at 15 minutes. No hard lockout: a delay always ends.
func LoginRule(threshold int) Rule {
	return Rule{Threshold: threshold, Window: 15 * time.Minute, Base: time.Second, Cap: 15 * time.Minute}
}

// Attempts counts failed attempts per normalized email and Kind in Postgres, so the
// count follows the account (not the source IP) and is exact across replicas. Rows
// are written for unknown emails too, so the table cannot be used to enumerate
// accounts. It takes the raw client: AuthAttempt has no Workspace (see the package
// doc). now is injectable, because this is the one path whose time cannot come from
// httprate.
type Attempts struct {
	ent   *ent.Client
	now   func() time.Time
	rules map[Kind]Rule
}

// AttemptsOption tunes NewAttempts.
type AttemptsOption func(*Attempts)

// WithClock replaces time.Now, for tests.
func WithClock(now func() time.Time) AttemptsOption { return func(a *Attempts) { a.now = now } }

// WithRule sets the throttle of a Kind. Until it is set a Kind is not throttled.
func WithRule(kind Kind, rule Rule) AttemptsOption {
	return func(a *Attempts) { a.rules[kind] = rule }
}

// WithRateLimits sets the throttle of every Kind from the configured limits, so the
// app and the test harness build the module the same way.
func WithRateLimits(limits config.RateLimits) AttemptsOption {
	return func(a *Attempts) {
		a.rules[KindLogin] = LoginRule(limits.LoginFailures)
		a.rules[KindStaffLogin] = LoginRule(limits.LoginFailures)
		a.rules[KindPasswordReset] = ResetRule(limits.ForgotAddress)
		// The notice to the owner of a Taken address (ADR 0031) shares the reset budget:
		// one limit for every mail a stranger can make us send to an address.
		a.rules[KindRegister] = ResetRule(limits.ForgotAddress)
		a.rules[KindEmailChange] = ResetRule(limits.ForgotAddress)
	}
}

// NewAttempts builds the module over the ent client.
func NewAttempts(client *ent.Client, opts ...AttemptsOption) *Attempts {
	// Every Kind starts disabled but with its window, so a bare module (the purge
	// job's) still knows when a row is stale.
	a := &Attempts{ent: client, now: time.Now, rules: map[Kind]Rule{
		KindLogin:         LoginRule(0),
		KindStaffLogin:    LoginRule(0),
		KindPasswordReset: ResetRule(0),
		KindRegister:      ResetRule(0),
		KindEmailChange:   ResetRule(0),
		// Not configurable: always on, with their own windows.
		KindTestSendUser:      {Threshold: TestSendPerUserHour, Window: time.Hour},
		KindTestSendWorkspace: {Threshold: TestSendPerWorkspaceDay, Window: 24 * time.Hour},
	}}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// NormalizeEmail is the counter key: trimmed and lower-cased.
func NormalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

func (a *Attempts) rule(kind Kind) (Rule, bool) {
	r, ok := a.rules[kind]
	return r, ok && r.Threshold > 0
}

// Limit is the failure threshold of the Kind (0 when it is not throttled).
func (a *Attempts) Limit(kind Kind) int {
	r, _ := a.rule(kind)
	return r.Threshold
}

// Now is the module's clock, so callers compute reset times on the same time base.
func (a *Attempts) Now() time.Time { return a.now() }

// RecordFailure counts one failed attempt. Failures older than the Rule's window
// start a new count.
func (a *Attempts) RecordFailure(ctx context.Context, kind Kind, email string) error {
	rule, ok := a.rule(kind)
	if !ok {
		return nil
	}
	email = NormalizeEmail(email)
	now := a.now()
	n, err := a.ent.AuthAttempt.Update().
		Where(authattempt.Email(email), authattempt.KindEQ(kind), authattempt.LastAttemptAtGTE(now.Add(-rule.Window))).
		AddFailures(1).
		SetLastAttemptAt(now).
		Save(ctx)
	if err != nil || n > 0 {
		return err
	}
	// No current row: insert one, or restart a stale one (the unique key makes a
	// concurrent first failure collapse into one row).
	return a.ent.AuthAttempt.Create().
		SetEmail(email).
		SetKind(kind).
		SetFailures(1).
		SetLastAttemptAt(now).
		OnConflictColumns(authattempt.FieldEmail, authattempt.FieldKind).
		UpdateNewValues().
		Exec(ctx)
}

// RecordSuccess resets the counter of the address.
func (a *Attempts) RecordSuccess(ctx context.Context, kind Kind, email string) error {
	if _, ok := a.rule(kind); !ok {
		return nil
	}
	_, err := a.ent.AuthAttempt.Delete().
		Where(authattempt.Email(NormalizeEmail(email)), authattempt.KindEQ(kind)).
		Exec(ctx)
	return err
}

// Delay reports how long the address must still wait before the next attempt; 0
// means it may proceed.
func (a *Attempts) Delay(ctx context.Context, kind Kind, email string) (time.Duration, error) {
	rule, ok := a.rule(kind)
	if !ok {
		return 0, nil
	}
	row, err := a.ent.AuthAttempt.Query().
		Where(authattempt.Email(NormalizeEmail(email)), authattempt.KindEQ(kind)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	now := a.now()
	if row.Failures < rule.Threshold || now.Sub(row.LastAttemptAt) > rule.Window {
		return 0, nil
	}
	wait := rule.Base
	for range row.Failures - rule.Threshold {
		wait *= 2
		if wait >= rule.Cap {
			wait = rule.Cap
			break
		}
	}
	return max(row.LastAttemptAt.Add(wait).Sub(now), 0), nil
}

// Take grants one slot of a budget Kind (password reset: at most Threshold slots per
// Window) to an address and reports whether it was granted. See take.
func (a *Attempts) Take(ctx context.Context, kind Kind, email string) (bool, error) {
	refusal, err := a.take(ctx, kind, NormalizeEmail(email))
	return refusal == ratelimit.Refusal{}, err
}

// counterKey is the counter key of a Kind that counts per User or per Workspace instead
// of per address. It shares the column of an address, so the prefix keeps the two apart:
// "user:7" is never a normalized email.
func counterKey(owner string, id int64) string { return owner + ":" + strconv.FormatInt(id, 10) }

// take grants one slot of a budget Kind to key, or returns the Refusal: how long until
// the budget frees a slot (a Window after the last granted slot) and its size. Check and
// count are one statement (an upsert whose update is conditional), so concurrent
// requests for one key cannot each see a free slot: exactly Threshold of them are
// granted. A refused request changes nothing. A Kind that is not throttled always
// grants; the Rule's Base and Cap are not involved.
func (a *Attempts) take(ctx context.Context, kind Kind, key string) (ratelimit.Refusal, error) {
	rule, ok := a.rule(kind)
	if !ok {
		return ratelimit.Refusal{}, nil
	}
	now := a.now()
	cutoff := now.Add(-rule.Window)
	row := sql.Table(authattempt.Table)
	_, err := a.ent.AuthAttempt.Create().
		SetEmail(key).
		SetKind(kind).
		SetFailures(1).
		SetLastAttemptAt(now).
		OnConflict(
			sql.ConflictColumns(authattempt.FieldEmail, authattempt.FieldKind),
			// A stale row restarts the count; a current one counts up while a slot is
			// left. Otherwise nothing is updated and no row comes back.
			sql.UpdateWhere(sql.Or(
				sql.LT(row.C(authattempt.FieldLastAttemptAt), cutoff),
				sql.LT(row.C(authattempt.FieldFailures), rule.Threshold),
			)),
			sql.ResolveWith(func(u *sql.UpdateSet) {
				u.Set(authattempt.FieldFailures, sql.ExprFunc(func(b *sql.Builder) {
					b.WriteString("CASE WHEN ").Ident(authattempt.Table).WriteByte('.').Ident(authattempt.FieldLastAttemptAt).
						WriteString(" < ").Arg(cutoff).
						WriteString(" THEN 1 ELSE ").Ident(authattempt.Table).WriteByte('.').Ident(authattempt.FieldFailures).
						WriteString(" + 1 END")
				}))
				u.SetExcluded(authattempt.FieldLastAttemptAt)
			}),
		).
		ID(ctx)
	if err == nil {
		return ratelimit.Refusal{}, nil
	}
	if !errors.Is(err, stdsql.ErrNoRows) {
		return ratelimit.Refusal{}, err
	}
	// Refused: the one read that says when the window frees a slot.
	current, err := a.ent.AuthAttempt.Query().
		Where(authattempt.Email(key), authattempt.KindEQ(kind)).
		Only(ctx)
	if err != nil {
		return ratelimit.Refusal{}, err
	}
	wait := max(current.LastAttemptAt.Add(rule.Window).Sub(now), time.Second)
	return ratelimit.Refusal{Wait: wait, Limit: rule.Threshold}, nil
}

// refund gives back a slot take granted, for a request that a second budget then refused.
func (a *Attempts) refund(ctx context.Context, kind Kind, key string) error {
	return a.ent.AuthAttempt.Update().
		Where(authattempt.Email(key), authattempt.KindEQ(kind), authattempt.FailuresGT(0)).
		AddFailures(-1).
		Exec(ctx)
}

// TakeTestSend grants one Test send from the System sender (ADR 0041): a slot of the
// User's hourly budget and one of the Workspace's daily budget. It returns the zero
// Refusal when both were granted, else the Refusal of the budget that was spent. A
// request the Workspace budget refuses gives the User's slot back.
func (a *Attempts) TakeTestSend(ctx context.Context, userID, workspaceID int64) (ratelimit.Refusal, error) {
	userKey := counterKey("user", userID)
	refusal, err := a.take(ctx, KindTestSendUser, userKey)
	if err != nil || refusal != (ratelimit.Refusal{}) {
		return refusal, err
	}
	refusal, err = a.take(ctx, KindTestSendWorkspace, counterKey("workspace", workspaceID))
	if err != nil || refusal != (ratelimit.Refusal{}) {
		err = errors.Join(err, a.refund(ctx, KindTestSendUser, userKey))
	}
	return refusal, err
}

// Purge deletes the rows whose last failure is past their Kind's window (a delay is
// never longer than the window, so they no longer matter) and reports how many.
func (a *Attempts) Purge(ctx context.Context) (int, error) {
	now := a.now()
	total := 0
	for kind, rule := range a.rules {
		n, err := a.ent.AuthAttempt.Delete().
			Where(authattempt.KindEQ(kind), authattempt.LastAttemptAtLT(now.Add(-rule.Window))).
			Exec(ctx)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}
