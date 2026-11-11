// Package tagdeletion is the use case that deletes a Tag from the catalogue (ADR 0102, ADR 0049).
// A Tag is deleted outright unless it is in Definition use: the Removal check
// (internal/domain/orchestration/removalcheck) asks every module that may name it, and refuses
// with a *liveuse.Error naming them. The tags module knows none of them, so the check runs here,
// under the Workspace lock, before the module's unchecked removal, tagsunchecked.DeleteIn, which
// only a use case may import (ADR 0110).
package tagdeletion

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/tags"
	"github.com/getsphericon/sphericon/internal/domain/contacts/tags/tagsunchecked"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/removalcheck"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// UseCase deletes a Tag. Every call receives the Workspace-scoped client (ADR 0017) built by the
// entry point; the use case opens the transaction the modules it calls join.
type UseCase struct {
	bus   *events.Bus
	check *removalcheck.Module
}

// New builds the Tag deletion use case over the Removal check.
func New(bus *events.Bus, check *removalcheck.Module) *UseCase {
	return &UseCase{bus: bus, check: check}
}

// Delete removes a Tag from the catalogue, and from every Contact that has it, unless it is in
// Definition use: a Segment's Condition, a step of a kept Automation's live version or draft, a
// kept Rule or the Condition an armed Broadcast copied from its Segment names it (ADR 0049, ADR
// 0065). Then the Removal check's *liveuse.Error names them, because each would silently stop
// matching or applying. A missing Tag is tags.ErrNotFound. The check and the delete run in one
// transaction under the Workspace's lock (ADR 0099), which saving a Segment, a Rule or an
// Automation takes too, so none of them names the Tag between the check and the delete.
func (u *UseCase) Delete(ctx context.Context, s *ent.Scoped, id int64) error {
	return u.bus.WithinWorkspaceLockTx(ctx, s, func(wl *ent.WorkspaceLocked, _ events.Publisher) error {
		t, err := tags.Get(ctx, wl.Scope(), id)
		if err != nil {
			return err
		}
		if err := u.check.Tag(ctx, wl, t.Name); err != nil {
			return err
		}
		return tagsunchecked.DeleteIn(ctx, wl, id)
	})
}
