package optout

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/binding"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/contacts/suppression"
	"github.com/getsphericon/sphericon/internal/domain/contacts/unsubscribes"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
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
	// Via is how the person stopped: one of events.UnsubscribedBy*.
	Via string
	// Message is the Outbound message a button was pressed on, nil when there is none or it did
	// not resolve. Its Broadcast or Automation is named on its Contact's event, from which the
	// Broadcast counts the opt-out (the broadcasts consumer, internal/jobs).
	Message *ent.OutboundMessage
	// Now is the clock of the fact.
	Now time.Time
	// DedupKey is the key of the fact the stop came from: the events derive theirs from it.
	DedupKey string
}

// StopTelegram records the Channel-wide opt-out of the user (scope `everything`, Channel
// `telegram`, the Destination) and telegram.unsubscribed for every Contact the Destination is
// Bound to, on any bot or through a Visitor. A Destination nobody is Bound to still gets the
// opt-out, and nothing else. It is idempotent: a user already opted out changes nothing and
// records no second event. Run it inside the transaction s and pub are bound to.
func StopTelegram(ctx context.Context, s *ent.Scoped, pub events.Publisher, st TelegramStop) error {
	exists, err := unsubscribes.Exists(ctx, s, channels.Telegram, st.Destination, eligibility.SourceEverything)
	if err != nil {
		return fmt.Errorf("find opt-out: %w", err)
	}
	if exists {
		return nil
	}
	contactIDs, err := contactsOf(ctx, s, st.Destination)
	if err != nil {
		return err
	}
	if st.Message != nil && st.Message.ContactID != nil && !slices.Contains(contactIDs, *st.Message.ContactID) {
		contactIDs = append([]int64{*st.Message.ContactID}, contactIDs...)
	}
	o := unsubscribes.OptOut{
		Channel: channels.Telegram, Destination: st.Destination, Source: eligibility.SourceEverything,
		// The ways of stopping (events.UnsubscribedBy*) are the row's own values.
		Via: unsubscribe.Via(st.Via),
	}
	if len(contactIDs) > 0 {
		o.ContactID = contactIDs[0]
	}
	if err := unsubscribes.Add(ctx, s, o); err != nil {
		return fmt.Errorf("record opt-out: %w", err)
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
		if m := st.Message; m != nil && m.ContactID != nil && *m.ContactID == c.ID {
			// The Broadcast counts the opt-out when its consumer works this event (broadcasts owns
			// the recipient row, ADR 0102): the job commits with the opt-out.
			e.BroadcastID, e.AutomationID = lo.FromPtr(m.BroadcastID), lo.FromPtr(m.AutomationID)
		}
		if err := pub.Publish(ctx, e); err != nil {
			return err
		}
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
var ErrTelegramSource = validation.RefuseField("sendingSource", "consent: a telegram opt-out is for everything", i18n.ErrorsSendingSourceTelegram)

// TelegramUnsubscribe records the Channel-wide opt-out of a Telegram user id, with the effects of
// the user stopping in Telegram (StopTelegram): the row and telegram.unsubscribed for every
// Contact the id is Bound to. A manual opt-out is Channel-wide: only the `everything` source is
// accepted here; a source-scoped one comes from the link in a message (the unsubscribe use case).
// Idempotent.
func TelegramUnsubscribe(ctx context.Context, bus *events.Bus, s *ent.Scoped, destination, source string) (*ent.Unsubscribe, error) {
	dest := strings.TrimSpace(destination)
	if dest == "" {
		return nil, unsubscribes.ErrDestinationEmpty
	}
	if !suppression.ValidDestination("telegram", dest) {
		return nil, unsubscribes.ErrDestinationInvalid
	}
	if source != eligibility.SourceEverything {
		return nil, ErrTelegramSource
	}
	now := time.Now()
	err := bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		return StopTelegram(ctx, ts, pub, TelegramStop{
			Destination: dest, Via: events.UnsubscribedByManual, Now: now,
			DedupKey: "manual:" + dest + ":" + strconv.FormatInt(now.UnixNano(), 10),
		})
	})
	if err != nil {
		return nil, err
	}
	return unsubscribes.Get(ctx, s, channels.Telegram, dest, source)
}
