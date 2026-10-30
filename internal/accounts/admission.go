package accounts

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/validation"
)

// GracePeriod is how long a User without a Second factor keeps access to a Workspace
// with a Two-factor requirement (ADR 0020).
const GracePeriod = 7 * 24 * time.Hour

var (
	// ErrAcceptancePending refuses a request on the own credential of a User with a required
	// acceptance pending (ADR 0062). Every surface answers it 403 acceptance_pending.
	ErrAcceptancePending = validation.AcceptancePending("accounts: a required acceptance is pending", "errors.acceptance_pending")
	// ErrSecondFactorRequired refuses a request in a Workspace whose Two-factor requirement
	// withholds it from the User (ADR 0020). Every surface answers it 403 second_factor_required.
	ErrSecondFactorRequired = validation.SecondFactorRequired("accounts: the second factor is required", "errors.second_factor_required")
)

// Admission is a request made on a User's own credential, as Admit decides it: a site session, or
// the token of a Connection. The Workspace's API token is no User's and is never admitted here.
type Admission struct {
	// UserID is the User the credential is of.
	UserID int64
	// Membership is the User's Membership of the Workspace the operation addresses, its User and
	// Workspace loaded; nil for an operation of no Workspace.
	Membership *ent.Membership
	// StrongSession is a session that passed a Second factor step or began with a Passkey (ADR
	// 0052): it meets the Two-factor requirement by itself. A token is never one.
	StrongSession bool
	// WhileAcceptancePending and WhileWithheld are what the operation's contract lets through: a
	// required acceptance pending, and the Two-factor requirement withholding the User.
	WhileAcceptancePending bool
	WhileWithheld          bool
	// At is when the request is made, the time the Two-factor grace is checked against.
	At time.Time
}

// Admit is nil when the User may act now, on every surface alike (ADR 0061). It refuses first a
// User with a required acceptance pending (ErrAcceptancePending), then, in a Workspace, one the
// Workspace's Two-factor requirement withholds (ErrSecondFactorRequired), each unless the
// operation lets it through, so a User both gated and withheld is refused alike everywhere. What
// the User's Role allows is the Permissions' to say, not Admit's.
func (a *Accounts) Admit(ctx context.Context, r Admission) error {
	if !r.WhileAcceptancePending {
		gated, err := a.consents.Gated(ctx, r.UserID)
		if err != nil {
			return err
		}
		if gated {
			return ErrAcceptancePending
		}
	}
	if r.Membership != nil && !r.WhileWithheld && withheld(r.Membership, r.At, r.StrongSession) {
		return ErrSecondFactorRequired
	}
	return nil
}

// HasSecondFactor reports whether the User has a confirmed Second factor. A pending enrollment is
// not one.
func HasSecondFactor(u *ent.User) bool {
	return u.SecondFactorConfirmedAt != nil && u.SecondFactorSecretEncrypted != ""
}

// SecondFactorDeadline is the User's grace end in the Workspace of the Membership, or false when
// none applies: the Workspace has no requirement or the User has a Second factor. The
// Membership's User and Workspace edges must be loaded.
func SecondFactorDeadline(m *ent.Membership) (time.Time, bool) {
	if HasSecondFactor(m.Edges.User) {
		return time.Time{}, false
	}
	return graceEnds(m.Edges.Workspace.SecondFactorRequiredAt, m.CreatedAt)
}

// graceEnds returns when a member's grace under a Workspace's Two-factor requirement ends:
// GracePeriod after the later of the requirement's start (requiredAt) and the Membership's
// creation. ok is false when the Workspace has no requirement (requiredAt is nil). It does not
// look at the User: one who has a Second factor is never withheld, whatever this returns.
func graceEnds(requiredAt *time.Time, memberSince time.Time) (time.Time, bool) {
	if requiredAt == nil {
		return time.Time{}, false
	}
	start := *requiredAt
	if memberSince.After(start) {
		start = memberSince
	}
	return start.Add(GracePeriod), true
}

// withheld reports whether the Workspace of the Membership is withheld from its User at now: a
// deadline applies and has passed, and the session does not meet the requirement by itself.
func withheld(m *ent.Membership, now time.Time, strongSession bool) bool {
	if strongSession {
		return false
	}
	end, ok := SecondFactorDeadline(m)
	return ok && !now.Before(end)
}
