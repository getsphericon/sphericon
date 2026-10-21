package secondfactor

import (
	"errors"
	"time"

	"github.com/getsphericon/sphericon/ent"
)

// GracePeriod is how long a User without a Second factor keeps access to a Workspace
// with a Two-factor requirement (ADR 0020).
const GracePeriod = 7 * 24 * time.Hour

// ErrRequired means the Workspace requires a Second factor, the User has none and
// their grace has ended. The site answers it with 403 second_factor_required.
var ErrRequired = errors.New("second factor required")

// GraceEnds returns when a member's grace under a Workspace's Two-factor
// requirement ends: GracePeriod after the later of the requirement's start
// (requiredAt) and the Membership's creation. ok is false when the Workspace has no
// requirement (requiredAt is nil). It does not look at the User: one who has a
// Second factor is never withheld, whatever this returns.
func GraceEnds(requiredAt *time.Time, memberSince time.Time) (time.Time, bool) {
	if requiredAt == nil {
		return time.Time{}, false
	}
	start := *requiredAt
	if memberSince.After(start) {
		start = memberSince
	}
	return start.Add(GracePeriod), true
}

// Deadline is the User's grace end in the Workspace of the Membership, or false when
// none applies: the Workspace has no requirement or the User has a Second factor.
// The Membership's User and Workspace edges must be loaded.
func Deadline(m *ent.Membership) (time.Time, bool) {
	if Active(m.Edges.User) {
		return time.Time{}, false
	}
	return GraceEnds(m.Edges.Workspace.SecondFactorRequiredAt, m.CreatedAt)
}

// Withheld reports whether the Workspace of the Membership is withheld from its User
// at now: a deadline applies and has passed, and the session does not meet the requirement
// by itself. strongSession is true for a session that passed a Second factor step or began
// with a Passkey (ADR 0052). The Membership's User and Workspace edges must be loaded.
func Withheld(m *ent.Membership, now time.Time, strongSession bool) bool {
	if strongSession {
		return false
	}
	end, ok := Deadline(m)
	return ok && !now.Before(end)
}
