package secondfactors

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/secondfactor"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// GracePeriod is how long a User without a Second factor keeps access to a Workspace
// with a Two-factor requirement (ADR 0020).
const GracePeriod = 7 * 24 * time.Hour

// ErrSecondFactorRequired refuses a request in a Workspace whose Two-factor requirement
// withholds it from the User (ADR 0020). Every surface answers it 403 second_factor_required.
var ErrSecondFactorRequired = validation.SecondFactorRequired("secondfactor: the second factor is required", i18n.ErrorsSecondFactorRequired)

// Admission is a request made on a User's own credential, as Admit decides it: a site session, or
// the token of a Connection. The Workspace's API token is no User's and is never admitted here.
type Admission struct {
	// UserID is the User the credential is of.
	UserID int64
	// Membership is the User's Membership of the Workspace the operation addresses, its
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
// User with a required acceptance pending (users.ErrAcceptancePending), then, in a Workspace, one
// the Workspace's Two-factor requirement withholds (ErrSecondFactorRequired), each unless the
// operation lets it through, so a User both gated and withheld is refused alike everywhere. What
// the User's Role allows is the Permissions' to say, not Admit's.
func (m *Module) Admit(ctx context.Context, r Admission) error {
	if !r.WhileAcceptancePending {
		if err := m.users.CheckAcceptance(ctx, r.UserID); err != nil {
			return err
		}
	}
	if r.Membership == nil || r.WhileWithheld || r.StrongSession {
		return nil
	}
	end, ok := graceEnds(r.Membership.Edges.Workspace.SecondFactorRequiredAt, r.Membership.CreatedAt)
	if !ok || r.At.Before(end) {
		return nil
	}
	has, err := m.Active(ctx, r.UserID)
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	return ErrSecondFactorRequired
}

// Deadline is the grace end of the Membership's User in its Workspace, or false when none applies:
// the Workspace has no requirement or the User has a Second factor. The Membership's Workspace
// edge must be loaded.
func (m *Module) Deadline(ctx context.Context, ms *ent.Membership) (time.Time, bool, error) {
	deadlines, err := m.Deadlines(ctx, []*ent.Membership{ms})
	if err != nil {
		return time.Time{}, false, err
	}
	end, ok := deadlines[ms.ID]
	return end, ok, nil
}

// Deadlines are the grace ends of the Memberships' Users in their Workspaces, by Membership id,
// for the Memberships a deadline applies to (Deadline), read in one query of the factors. Each
// Membership's Workspace edge must be loaded.
func (m *Module) Deadlines(ctx context.Context, ms []*ent.Membership) (map[int64]time.Time, error) {
	return deadlines(ctx, m.ent, ms)
}

func deadlines(ctx context.Context, client *ent.Client, ms []*ent.Membership) (map[int64]time.Time, error) {
	ends := map[int64]time.Time{}
	var userIDs []int64
	for _, ms := range ms {
		if end, ok := graceEnds(ms.Edges.Workspace.SecondFactorRequiredAt, ms.CreatedAt); ok {
			ends[ms.ID] = end
			userIDs = append(userIDs, ms.UserID)
		}
	}
	if len(userIDs) == 0 {
		return ends, nil
	}
	enrolled, err := client.SecondFactor.Query().
		Where(secondfactor.UserIDIn(userIDs...), secondfactor.ConfirmedAtNotNil(), secondfactor.SecretEncryptedNEQ("")).
		All(ctx)
	if err != nil {
		return nil, err
	}
	has := map[int64]bool{}
	for _, f := range enrolled {
		has[f.UserID] = active(f)
	}
	for _, ms := range ms {
		if has[ms.UserID] {
			delete(ends, ms.ID)
		}
	}
	return ends, nil
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
