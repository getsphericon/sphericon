// Package integrationdeletion is the use case that deletes an Integration (ADR 0102, ADR 0049).
// An Integration is in Live use while a Broadcast or an Automation above the integrations module
// will still send through it (ADR 0099); deleting it runs the Removal check
// (internal/domain/orchestration/removalcheck), which refuses with a *liveuse.Error naming the work
// if any sends through it, and otherwise calls the module's unchecked removal,
// integrationsunchecked.DeleteIn, which only a use case may import (ADR 0110).
package integrationdeletion

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/removalcheck"
	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
	"github.com/getsphericon/sphericon/internal/domain/sending/integrations/integrationsunchecked"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// UseCase deletes an Integration. Every call receives the Workspace-scoped client (ADR 0017) built
// by the entry point; the use case opens the transaction the modules it calls join.
type UseCase struct {
	bus          *events.Bus
	integrations *integrations.Module
	check        *removalcheck.Module
	now          func() time.Time
}

// New builds the Integration deletion use case over the Integrations module, which reads the row
// and releases a deleted Telegram bot's webhook, and the Removal check; now is the Instance clock.
func New(bus *events.Bus, integs *integrations.Module, check *removalcheck.Module, now func() time.Time) *UseCase {
	return &UseCase{bus: bus, integrations: integs, check: check, now: now}
}

// Delete discards an Integration for good (ADR 0049): it leaves the lists and is never restored,
// and what it sent keeps its history. Its Live use (ADR 0099) refuses the delete with the Removal
// check's *liveuse.Error naming that work: a scheduled or sending Broadcast, the live version of an
// active Automation or a version an unfinished Enrollment stands on, whose send goes through it, the
// Integration it names or, naming none, its own Channel's default. A sent or failed Broadcast is
// history and does not block. A deleted Integration gives up the default: its channel has none
// until another is picked. Deleting a deleted Integration changes nothing. An unknown id is
// integrations.ErrNotFound. The check and the discard hold the Workspace's lock (ADR 0099), so no
// work becomes live between them; a Telegram bot lets go of its webhook once the discard commits.
func (u *UseCase) Delete(ctx context.Context, s *ent.Scoped, id int64) error {
	var released *ent.Integration
	err := u.bus.WithinWorkspaceLockTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		row, err := u.integrations.Get(ctx, ts, id)
		if err != nil {
			return err
		}
		if row.DiscardedAt != nil {
			return nil
		}
		if err := u.check.Integration(ctx, ts, row.ID); err != nil {
			return err
		}
		released = row
		return integrationsunchecked.DeleteIn(ctx, ts, row, u.now())
	})
	if err != nil {
		return err
	}
	if released != nil {
		u.integrations.ReleaseDeleted(ctx, released)
	}
	return nil
}
