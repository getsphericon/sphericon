// Package sendingdomaindeletion is the use case that deletes a Sending domain (ADR 0102, ADR
// 0049). A Sending domain is in Live use while a Broadcast or an Automation above the
// sendingdomains module will still send email from an address on it (ADR 0099); deleting it runs
// the Removal check (internal/domain/orchestration/removalcheck), which refuses with a
// *liveuse.Error naming the work if any sends from it, and otherwise calls the module's unchecked
// removal, sendingdomainsunchecked.DeleteIn, which only a use case may import (ADR 0110).
package sendingdomaindeletion

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/removalcheck"
	"github.com/getsphericon/sphericon/internal/domain/sending/sendingdomains"
	"github.com/getsphericon/sphericon/internal/domain/sending/sendingdomains/sendingdomainsunchecked"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// UseCase deletes a Sending domain. Every call receives the Workspace-scoped client (ADR 0017)
// built by the entry point; the use case opens the transaction the modules it calls join.
type UseCase struct {
	bus     *events.Bus
	domains *sendingdomains.Module
	check   *removalcheck.Module
	now     func() time.Time
}

// New builds the Sending domain deletion use case over the Sending domains module, which reads the
// row, and the Removal check; now is the Instance clock.
func New(bus *events.Bus, domains *sendingdomains.Module, check *removalcheck.Module, now func() time.Time) *UseCase {
	return &UseCase{bus: bus, domains: domains, check: check, now: now}
}

// Delete discards a Sending domain for good (ADR 0049): it leaves the lists and is never
// restored, and the record of mail sent from it stays. Its Live use (ADR 0099) refuses the delete
// with the Removal check's *liveuse.Error naming that work. A domain is used through the domain of
// a From address, not an id: a scheduled or sending Broadcast, the live version of an active
// Automation or a version an unfinished Enrollment stands on uses the domain of the From its email
// goes out with, its own or, without one, the From of the Integration it goes through (an
// Automation step carries none). A sent or failed Broadcast is history and does not block. Deleting
// a deleted domain changes nothing; an unknown id is sendingdomains.ErrNotFound. The check and the
// discard hold the Workspace's lock (ADR 0099), so no work becomes live between them.
func (u *UseCase) Delete(ctx context.Context, s *ent.Scoped, id int64) error {
	return u.bus.WithinWorkspaceLockTx(ctx, s, func(wl *ent.WorkspaceLocked, _ events.Publisher) error {
		row, err := u.domains.Get(ctx, wl.Scope(), id)
		if err != nil {
			return err
		}
		if row.DiscardedAt != nil {
			return nil
		}
		if err := u.check.SendingDomain(ctx, wl, row.Domain); err != nil {
			return err
		}
		return sendingdomainsunchecked.DeleteIn(ctx, wl, id, u.now())
	})
}
