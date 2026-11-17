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

// CheckRequirement is nil unless the Two-factor requirement of the Workspace of ms withholds its
// member at the time at, when the request is made (ErrSecondFactorRequired). The Membership's
// Workspace edge must be loaded.
// A member is withheld once their grace ends (GracePeriod after the later of the requirement's
// start and the Membership's creation) and they have no Second factor. Whether a request may pass
// anyway (a strong session, an operation that lets a withheld User through) is the admission's to
// say, not the requirement's.
func (m *Module) CheckRequirement(ctx context.Context, ms *ent.Membership, at time.Time) error {
	end, ok := graceEnds(ms.Edges.Workspace.SecondFactorRequiredAt, ms.CreatedAt)
	if !ok || at.Before(end) {
		return nil
	}
	has, err := m.Active(ctx, ms.UserID)
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	return ErrSecondFactorRequired
}

// Deadlines are the grace ends of the Memberships' Users in their Workspaces, by Membership id,
// for the Memberships a deadline applies to (the Workspace has a requirement and the User no Second
// factor), read in one query of the factors. Each Membership's Workspace edge must be loaded.
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

// RequirementGraceEnds returns when the grace of the members a Workspace had when its Two-factor
// requirement began (requiredAt) ends, nil when it has no requirement. A later member's grace
// counts from their joining instead.
func RequirementGraceEnds(requiredAt *time.Time) *time.Time {
	end, ok := graceEnds(requiredAt, time.Time{})
	if !ok {
		return nil
	}
	return &end
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
