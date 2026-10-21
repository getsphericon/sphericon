package abuse

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/reputation"
	"github.com/getsphericon/sphericon/internal/suspension"
)

// Watch reads a Workspace's standing with the detector, without acting on it: the
// console and the dashboard use it, the Detector builds on it.
type Watch struct {
	lic    *licensekey.License
	limits Limits
	rep    *reputation.Module
}

// NewWatch builds a Watch over the limits (zero Limits are the defaults).
func NewWatch(lic *licensekey.License, limits Limits) *Watch {
	return &Watch{lic: lic, limits: limits.orDefault(), rep: reputation.New()}
}

// Limits are the limits in force.
func (w *Watch) Limits() Limits { return w.limits }

// Licensed reports whether the detector runs at all (the `staff` license).
func (w *Watch) Licensed() bool { return w.lic.Has(licensekey.FeatureStaff) }

// WindowFor is what the detector counts for the Workspace at now: the trailing 24 hours,
// or since the Staff member last cleared a suspension, whichever is shorter.
func WindowFor(ws *ent.Workspace, now time.Time) reputation.Window {
	since := now.Add(-Window)
	if ws.ClearedAt != nil && ws.ClearedAt.After(since) {
		since = *ws.ClearedAt
	}
	return reputation.Window{Since: since, Until: now}
}

// Standing is the Workspace's breach at now, whatever its state.
func (w *Watch) Standing(ctx context.Context, s *ent.Scoped, ws *ent.Workspace, now time.Time) (Breach, error) {
	rates, err := w.rep.Rates(ctx, s, WindowFor(ws, now))
	if err != nil {
		return Breach{}, err
	}
	return w.limits.Classify(rates), nil
}

// Warning is the Workspace's breach for display: the detector's standing, but none for an
// unlicensed instance, an exempt Workspace (never warned or frozen automatically) or one
// that is already suspended.
func (w *Watch) Warning(ctx context.Context, s *ent.Scoped, ws *ent.Workspace, now time.Time) (Breach, error) {
	if !w.Licensed() || ws.AbuseExempt || suspension.Is(ws) {
		return Breach{}, nil
	}
	return w.Standing(ctx, s, ws, now)
}

// Review narrows the console's Workspace list to the ones the abuse detector surfaces
// (ADR 0026); the filters are stated here, with the detector, once.
type Review string

const (
	ReviewNone Review = ""
	// ReviewWarned: the owners were warned and the Workspace is still in the warning tier.
	ReviewWarned Review = "warned"
	// ReviewSystemFrozen: suspended by the detector (actor `system`).
	ReviewSystemFrozen Review = "system_frozen"
	// ReviewDeleted: in the deletion window or erased (ADR 0043); every other list hides them.
	ReviewDeleted Review = "deleted"
)

// Predicate is the Workspace filter a review state names. Every list but the deleted one hides
// the deleted Workspaces (accounts.Live).
func (r Review) Predicate() predicate.Workspace {
	switch r {
	case ReviewDeleted:
		return accounts.Deleted()
	case ReviewWarned:
		return workspace.And(accounts.Live(), workspace.AbuseWarningNotifiedAtNotNil(), suspension.NotSuspended(), workspace.AbuseExempt(false))
	case ReviewSystemFrozen:
		return workspace.And(accounts.Live(), suspension.BySystem())
	default:
		return accounts.Live()
	}
}
