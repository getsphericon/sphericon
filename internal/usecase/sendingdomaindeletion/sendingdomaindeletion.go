// Package sendingdomaindeletion is the use case that deletes a Sending domain (ADR 0102, ADR
// 0049). A Sending domain is in Live use while a Broadcast or an Automation above the
// sendingdomains module will still send email from an address on it (ADR 0099); deleting it reads
// that Live use from the orchestration modules, refuses with a *liveuse.Error naming the work if
// any sends from it, and otherwise calls the module's unchecked removal, sendingdomains.DeleteIn,
// which easement forbids every other package to call.
package sendingdomaindeletion

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/liveuse"
	"github.com/getsphericon/sphericon/internal/domain/sending/sendingdomains"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
)

// Senders builds the EmailSender of an Integration, as a send through it does: the Live use check
// reads the From it is configured with from it. *messaging.Resolver satisfies it.
type Senders interface {
	EmailSender(ctx context.Context, s *ent.Scoped, integ *ent.Integration) (messaging.EmailSender, error)
}

// UseCase deletes a Sending domain. Every call receives the Workspace-scoped client (ADR 0017)
// built by the entry point; the use case opens the transaction the modules it calls join.
type UseCase struct {
	bus     *events.Bus
	domains *sendingdomains.Module
	senders Senders
	now     func() time.Time
}

// New builds the Sending domain deletion use case over the Sending domains module, which reads the
// row, and senders, which tells the From an Integration sends with; now is the Instance clock.
func New(bus *events.Bus, domains *sendingdomains.Module, senders Senders, now func() time.Time) *UseCase {
	return &UseCase{bus: bus, domains: domains, senders: senders, now: now}
}

// Delete discards a Sending domain for good (ADR 0049): it leaves the lists and is never
// restored, and the record of mail sent from it stays. Its Live use (ADR 0099) refuses the delete
// with a *liveuse.Error naming that work. A domain is used through the domain of a From address, not
// an id: a scheduled or sending Broadcast, the live version of an active Automation or a version an
// unfinished Enrollment stands on uses the domain of the From its email goes out with, its own or,
// without one, the From of the Integration it goes through (an Automation step carries none). A sent
// or failed Broadcast is history and does not block. Deleting a deleted domain changes nothing; an
// unknown id is sendingdomains.ErrNotFound. The check and the discard hold the Workspace's lock (ADR
// 0099), so no work becomes live between them.
func (u *UseCase) Delete(ctx context.Context, s *ent.Scoped, id int64) error {
	return u.bus.WithinWorkspaceLockTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		row, err := u.domains.Get(ctx, ts, id)
		if err != nil {
			return err
		}
		if row.DiscardedAt != nil {
			return nil
		}
		users, err := u.liveUsers(ctx, ts, row.Domain)
		if err != nil {
			return err
		}
		if len(users) > 0 {
			return &liveuse.Error{What: liveuse.SubjectSendingDomain, Users: users}
		}
		return sendingdomains.DeleteIn(ctx, ts, id, u.now())
	})
}

// liveUsers is the Live use of the domain (ADR 0099): the work whose email goes out from an address
// on it, its own From or, without one, the From of the Integration it goes through, by the same
// resolution a send takes (messaging.Through, messaging.EffectiveFrom), Broadcasts first, then
// Automations.
func (u *UseCase) liveUsers(ctx context.Context, s *ent.Scoped, domain string) ([]liveuse.User, error) {
	sends, err := liveuse.Sends(ctx, s, broadcasts.LiveSends, automations.LiveSends)
	if err != nil {
		return nil, err
	}
	senders := map[int64]messaging.EmailSender{}
	var onDomain []liveuse.Send
	for _, send := range sends {
		if send.Channel != channels.Email {
			continue
		}
		var sender messaging.EmailSender
		if send.Integration != nil {
			if sender = senders[send.Integration.ID]; sender == nil {
				if sender, err = u.senders.EmailSender(ctx, s, send.Integration); err != nil {
					return nil, err
				}
				senders[send.Integration.ID] = sender
			}
		}
		if from, _ := messaging.EffectiveFrom(sender, send.From, ""); messaging.DomainOf(from) == domain {
			onDomain = append(onDomain, send)
		}
	}
	return liveuse.SendUsers(onDomain), nil
}
