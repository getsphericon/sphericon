package channelstop

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/binding"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/internal/domain/contacts/consentstamps"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/contacts/unsubscribes"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/fieldname"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// TelegramStop is a Telegram user stopping the Workspace's Telegram marketing.
type TelegramStop struct {
	// Destination is the Telegram user id: the opt-out is keyed by it, so it covers every bot of
	// the Workspace.
	Destination string
	// IntegrationID is the bot the stop came through.
	IntegrationID int64
	// Sequence is the bot's update_id of the update the stop came in, 0 when it came in none.
	Sequence int64
	// Via is how the person stopped: one of events.UnsubscribedBy*.
	Via string
	// Message attribution is the snapshot of the message the observation refers to.
	OutboundMessageID int64
	ContactID         int64
	BroadcastID       int64
	AutomationID      int64
	// At is when the stop occurred.
	At time.Time
	// DedupKey is the key of the fact the stop came from: the events derive theirs from it.
	DedupKey string
}

// StopTelegram records the Channel-wide opt-out of the user (scope `everything`, Channel
// `telegram`, the Destination) and telegram.unsubscribed for every Contact the Destination is
// Bound to, on any bot or through a Visitor. A Destination nobody is Bound to still gets the
// opt-out, and nothing else. It is idempotent: a user already opted out changes nothing and
// records no second event. A stop older than the user's opt-out state (a return of theirs
// occurred later) changes nothing and is recorded as contact.consent_superseded (ADR 0154). Run it
// inside the bus transaction s is bound to.
func (m *Module) StopTelegram(ctx context.Context, s *ent.Scoped, st TelegramStop) error {
	contactIDs, err := contactsOf(ctx, s, st.Destination)
	if err != nil {
		return err
	}
	if st.ContactID != 0 && !slices.Contains(contactIDs, st.ContactID) {
		contactIDs = append([]int64{st.ContactID}, contactIDs...)
	}
	o := unsubscribes.OptOut{
		Channel: channels.Telegram, Destination: st.Destination, Source: eligibility.SourceEverything,
		// The ways of stopping (events.UnsubscribedBy*) are the row's own values.
		Via: unsubscribe.Via(st.Via),
		At:  consentstamps.Occurrence{At: st.At, IntegrationID: st.IntegrationID, Sequence: st.Sequence},
	}
	if len(contactIDs) > 0 {
		o.ContactID = contactIDs[0]
	}
	change, err := m.unsubs.Add(ctx, s, o)
	if err != nil {
		return err
	}
	if change.Superseded {
		return m.publishSuperseded(ctx, s, contactIDs, &events.ConsentSuperseded{
			Channel: string(channels.Telegram), Observation: events.ObservationStop, Via: st.Via,
			IntegrationID: st.IntegrationID, DedupID: st.DedupKey, ObservedAt: st.At,
		})
	}
	if change.ID == 0 {
		return nil
	}
	for _, id := range contactIDs {
		c, err := s.Contact().Get(ctx, id)
		if ent.IsNotFound(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("read contact: %w", err)
		}
		e := &events.TelegramUnsubscribed{
			WorkspaceID: s.WorkspaceID(), ContactID: c.ID, Email: lo.FromPtr(c.Email),
			IntegrationID: st.IntegrationID, Via: st.Via, DedupID: fmt.Sprintf("%s/unsubscribed/%d", st.DedupKey, c.ID),
		}
		if st.ContactID == c.ID {
			// The public Contact event names the source; anonymous attribution is published below.
			e.BroadcastID, e.AutomationID = st.BroadcastID, st.AutomationID
		}
		if err := m.bus.PublishIn(ctx, s, e); err != nil {
			return err
		}
	}
	// Only an action on a received message attributes the stop; a provider refusing a new send
	// reports an existing refusal, not a reaction to that new Broadcast.
	if st.Via == events.UnsubscribedByButton && st.OutboundMessageID != 0 {
		return m.bus.PublishIn(ctx, s, &events.MessageUnsubscribed{WorkspaceID: s.WorkspaceID(), OutboundMessageID: st.OutboundMessageID, ObservedAt: st.At})
	}

	return nil
}

// contactsOf are the Contacts a Telegram Destination is Bound to on any bot of the Workspace, its
// own or through a Visitor, in the order the Bindings were made.
func contactsOf(ctx context.Context, s *ent.Scoped, destination string) ([]int64, error) {
	rows, err := s.Binding().Query().
		Where(binding.Destination(destination)).
		WithVisitor().
		Order(binding.ByBoundAt(), binding.ByID()).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list bindings: %w", err)
	}
	var ids []int64
	for _, b := range rows {
		id := lo.FromPtr(b.ContactID)
		if id == 0 && b.Edges.Visitor != nil {
			id = lo.FromPtr(b.Edges.Visitor.ContactID)
		}
		if id != 0 && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// ErrTelegramSource refuses a hand-recorded Telegram opt-out of a narrower source: it is
// Channel-wide.
var ErrTelegramSource = validation.RefuseField(fieldname.SendingSource, "consent: a telegram opt-out is for everything", i18n.ErrorsSendingSourceTelegram)

// TelegramUnsubscribe records the Channel-wide opt-out of a Telegram user id that reached the
// Workspace by another route (HandStop), with the effects of the user stopping in Telegram
// (StopTelegram): the row and telegram.unsubscribed for every Contact the id is Bound to. Such an
// opt-out is Channel-wide: only the `everything` source is accepted here; a source-scoped one
// comes from the link in a message (the unsubscribe use case). Idempotent.
func (m *Module) TelegramUnsubscribe(ctx context.Context, s *ent.Scoped, in HandStop) (*ent.Unsubscribe, error) {
	dest, err := unsubscribes.Destination(channels.Telegram, in.Destination)
	if err != nil {
		return nil, err
	}
	if in.Source != eligibility.SourceEverything {
		return nil, ErrTelegramSource
	}
	at := in.At
	if at.IsZero() {
		at = m.now()
	}
	err = m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		return m.StopTelegram(ctx, ts, TelegramStop{
			Destination: dest, Via: in.Via, At: at,
			DedupKey: in.Via + ":" + dest + ":" + strconv.FormatInt(at.UnixNano(), 10),
		})
	})
	if err != nil {
		return nil, err
	}
	return m.unsubs.Get(ctx, s, unsubscribes.Key{Channel: channels.Telegram, Destination: dest, Source: in.Source})
}
