// Package integrationdeletion is the use case that deletes an Integration (ADR 0102, ADR 0049).
// An Integration is in Live use while a Broadcast or an Automation above the integrations module
// will still send through it (ADR 0099); deleting it reads that Live use from the orchestration
// modules, refuses with an *inuse.Error naming the work if any sends through it, and otherwise
// calls the module's unchecked removal, integrations.DeleteIn, which easement forbids every other
// package to call.
package integrationdeletion

import (
	"context"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/integrations"
	"github.com/getsphericon/sphericon/internal/inuse"
)

// UseCase deletes an Integration. Every call receives the Workspace-scoped client (ADR 0017) built
// by the entry point; the use case opens the transaction the modules it calls join.
type UseCase struct {
	bus          *events.Bus
	integrations *integrations.Module
}

// New builds the Integration deletion use case over the Integrations module, which reads the row
// and releases a deleted Telegram bot's webhook.
func New(bus *events.Bus, integs *integrations.Module) *UseCase {
	return &UseCase{bus: bus, integrations: integs}
}

// Delete discards an Integration for good (ADR 0049): it leaves the lists and is never restored,
// and what it sent keeps its history. Its Live use (ADR 0099) refuses the delete with an
// *inuse.Error naming that work: a scheduled or sending Broadcast, the live version of an active
// Automation or a version an unfinished Enrollment stands on, whose send goes through it, the
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
		users, err := liveUsers(ctx, ts, row.ID)
		if err != nil {
			return err
		}
		if len(users) > 0 {
			return &inuse.Error{What: inuse.SubjectIntegration, Users: users}
		}
		released = row
		return integrations.DeleteIn(ctx, ts, row)
	})
	if err != nil {
		return err
	}
	if released != nil {
		u.integrations.ReleaseDeleted(ctx, released)
	}
	return nil
}

// liveUsers is the Live use of the Integration (ADR 0099): the work whose sends go through it, by
// the same resolution a send takes (messaging.Through), Broadcasts first, then Automations.
func liveUsers(ctx context.Context, s *ent.Scoped, id int64) ([]inuse.User, error) {
	sends, err := broadcasts.LiveSends(ctx, s)
	if err != nil {
		return nil, err
	}
	steps, err := automations.LiveSends(ctx, s)
	if err != nil {
		return nil, err
	}
	sends = append(sends, steps...)
	return inuse.SendUsers(lo.Filter(sends, func(send inuse.Send, _ int) bool { return send.Uses(id) })), nil
}
