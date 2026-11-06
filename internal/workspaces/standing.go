package workspaces

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/workspace"
)

// The Workspace's sending standing lives on its row: the suspension, the abuse exemption, the
// Send cap (internal/suspension says what they mean and audits each change) and the markers of the
// notices the abuse detector sends (ee/abuse, internal/suspension.NotifyOwners). These functions
// are their writes; each runs on the client or transaction it is given.

// SuspendIn suspends the Workspace id inside tx while it is not suspended: it sets suspended_at to
// now with who did it (kind, and byID for a Staff member) and why. It returns how many rows it
// changed: 0 when the Workspace was already suspended, whose first actor and reason stand.
func SuspendIn(ctx context.Context, tx *ent.Client, id int64, kind workspace.SuspendedByKind, byID *string, reason string) (int, error) {
	return tx.Workspace.Update().
		Where(workspace.ID(id), workspace.SuspendedAtIsNil()).
		SetSuspendedAt(time.Now()).
		SetSuspendedByKind(kind).
		SetNillableSuspendedByID(byID).
		SetSuspensionReason(reason).
		Save(ctx)
}

// UnsuspendIn lifts the suspension of the Workspace id inside tx while it is suspended: it clears
// the suspension and its attribution and stamps cleared_at with now. It returns how many rows it
// changed: 0 when the Workspace was not suspended.
func UnsuspendIn(ctx context.Context, tx *ent.Client, id int64) (int, error) {
	return tx.Workspace.Update().
		Where(workspace.ID(id), workspace.SuspendedAtNotNil()).
		ClearSuspendedAt().
		ClearSuspendedByKind().
		ClearSuspendedByID().
		ClearSuspensionReason().
		SetClearedAt(time.Now()).
		Save(ctx)
}

// SetAbuseExemptIn sets the abuse exemption of the Workspace id inside tx while it differs, and
// returns how many rows it changed.
func SetAbuseExemptIn(ctx context.Context, tx *ent.Client, id int64, exempt bool) (int, error) {
	return tx.Workspace.Update().
		Where(workspace.ID(id), workspace.AbuseExempt(!exempt)).
		SetAbuseExempt(exempt).
		Save(ctx)
}

// SetSendCapIn sets the Send cap of the Workspace id inside tx, or clears it with nil.
func SetSendCapIn(ctx context.Context, tx *ent.Client, id int64, sendCap *int) error {
	upd := tx.Workspace.UpdateOneID(id)
	if sendCap == nil {
		upd.ClearSendCap()
	} else {
		upd.SetSendCap(*sendCap)
	}
	return upd.Exec(ctx)
}

// MarkAbuseFreezeNotified stamps, with now, that the Owners of the Workspace id were told of its
// suspension.
func MarkAbuseFreezeNotified(ctx context.Context, client *ent.Client, id int64) error {
	return client.Workspace.UpdateOneID(id).SetAbuseFreezeNotifiedAt(time.Now()).Exec(ctx)
}

// MarkAbuseWarned records, at, that the Owners of the Workspace id were warned of a breach.
func MarkAbuseWarned(ctx context.Context, client *ent.Client, id int64, at time.Time) error {
	return client.Workspace.UpdateOneID(id).SetAbuseWarningNotifiedAt(at).Exec(ctx)
}

// ClearAbuseWarning ends the warning episode of the Workspace id.
func ClearAbuseWarning(ctx context.Context, client *ent.Client, id int64) error {
	return client.Workspace.UpdateOneID(id).ClearAbuseWarningNotifiedAt().Exec(ctx)
}

// ClearAbuseWarnings ends the warning episode of every Workspace that is warned and matches which.
func ClearAbuseWarnings(ctx context.Context, client *ent.Client, which ...predicate.Workspace) error {
	return client.Workspace.Update().
		Where(append([]predicate.Workspace{workspace.AbuseWarningNotifiedAtNotNil()}, which...)...).
		ClearAbuseWarningNotifiedAt().
		Exec(ctx)
}

// SetFeedPosition gives the Workspaces ids the service feed position (ADR 0062): the write of
// internal/workspacefeed's hook, which takes the position under its lock.
func SetFeedPosition(ctx context.Context, client *ent.Client, position int64, ids ...int64) error {
	return client.Workspace.Update().Where(workspace.IDIn(ids...)).SetFeedPosition(position).Exec(ctx)
}
