// Package integrationdeletion is the use case that deletes an Integration (ADR 0102, ADR 0049).
// An Integration is in Live use while a Broadcast or an Automation above the integrations module
// will still send through it (ADR 0099); deleting it runs the Removal check
// (internal/domain/orchestration/removalcheck), which refuses with a *liveuse.Error naming the work
// if any sends through it. The rest is the module's unchecked removal,
// integrationsunchecked.Deletion, which only a use case may import (ADR 0110) and which keeps every
// rule of the Integration's own row (ADR 0145): the use case locks, opens it, checks and applies.
package integrationdeletion

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/removalcheck"
	"github.com/getsphericon/sphericon/internal/domain/sending/integrations/integrationsunchecked"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// UseCase deletes an Integration. Every call receives the Workspace-scoped client (ADR 0017) built
// by the entry point; the use case opens the transaction the modules it calls join.
type UseCase struct {
	bus   *events.Bus
	check *removalcheck.Module
	now   func() time.Time
}

// New builds the Integration deletion use case over the Removal check; now is the Instance clock.
func New(bus *events.Bus, check *removalcheck.Module, now func() time.Time) *UseCase {
	return &UseCase{bus: bus, check: check, now: now}
}

// Delete discards an Integration for good (ADR 0049): it leaves the lists and is never restored,
// and what it sent keeps its history. Its Live use (ADR 0099) refuses the delete with the Removal
// check's *liveuse.Error naming that work: a scheduled or sending Broadcast, the live version of an
// active Automation or a version an unfinished Enrollment stands on, whose send goes through it, the
// Integration it names or, naming none, its own Channel's default. A sent or failed Broadcast is
// history and does not block. A deleted Integration gives up the default: its channel has none
// until another is picked. Deleting a deleted Integration changes nothing. An unknown id is
// integrations.ErrNotFound. The read, the check and the discard hold the Workspace's lock (ADR
// 0099, ADR 0108), which a promotion to default and a change of config take too, so no work becomes
// live between them and the Integration is never discarded as its Channel's default; a Telegram bot
// lets go of its webhook through a job queued in the discard's transaction.
func (u *UseCase) Delete(ctx context.Context, s *ent.Scoped, id int64) error {
	return u.bus.WithinWorkspaceLockEnqueueTx(ctx, s, func(wl *ent.WorkspaceLocked, _ events.Publisher, q events.Enqueuer) error {
		deletion, err := integrationsunchecked.DeletionIn(ctx, wl, id)
		if err != nil || deletion.Done() {
			return err
		}
		if err := u.check.Integration(ctx, wl, id); err != nil {
			return err
		}
		return deletion.Apply(ctx, u.now(), q)
	})
}
