// Package membershipsunchecked holds the memberships module's unchecked operation (ADR 0110): the
// leave of the only Owner of a Workspace in its deletion window, which only User deletion makes
// after it has refused a User who solely owns a live Workspace (ADR 0050). Its check is the use
// case's (internal/usecase/userdeletion); only a use case may import this package. The memberships
// module does not import it.
package membershipsunchecked

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// LeaveDoomedWorkspaceIn removes User userID's own Membership like memberships.LeaveIn but lets the
// only Owner go too, for User deletion of a Workspace that is in its deletion window (ADR 0050):
// nobody is left to hand it to, and the Workspace is erased on schedule. The User is the actor of
// the `membership.delete` Audit entry.
func LeaveDoomedWorkspaceIn(ctx context.Context, ts *ent.Scoped, pub events.Publisher, userID int64) error {
	target, _, actor, err := memberships.LockSelfIn(ctx, ts, userID)
	if err != nil {
		return err
	}
	if err := ts.Membership().DeleteOneID(target.ID).Exec(ctx); err != nil {
		return err
	}
	return events.RecordAudit(ctx, pub, memberships.RemovalAudit(ts.WorkspaceID(), actor, target))
}
