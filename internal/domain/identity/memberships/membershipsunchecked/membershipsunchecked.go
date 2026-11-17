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

// Module holds the unchecked operation over the memberships module.
type Module struct {
	memberships *memberships.Module
	bus         *events.Bus
}

// New builds the unchecked operation over the memberships module it reads the leaving Membership
// through, and the bus it publishes the Audit entry on, in the transaction of the scope it is
// given (events.Bus.RecordAuditIn).
func New(ms *memberships.Module, bus *events.Bus) *Module {
	return &Module{memberships: ms, bus: bus}
}

// LeaveDoomedWorkspaceIn removes User userID's own Membership like memberships.Module.LeaveIn but
// lets the only Owner go too, for User deletion of a Workspace that is in its deletion window
// (ADR 0050): nobody is left to hand it to, and the Workspace is erased on schedule. The User is the
// actor of the `membership.delete` Audit entry, published in the bus transaction ts is bound to.
func (m *Module) LeaveDoomedWorkspaceIn(ctx context.Context, ts *ent.Scoped, userID int64) error {
	self, err := m.memberships.LockSelfIn(ctx, ts, userID)
	if err != nil {
		return err
	}
	if err := ts.Membership().DeleteOneID(self.Membership.ID).Exec(ctx); err != nil {
		return err
	}
	return m.bus.RecordAuditIn(ctx, ts, memberships.RemovalAudit(ts.WorkspaceID(), self.Actor, self.Membership))
}
