package optout

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/binding"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
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
	// not resolve. Its Broadcast counts the opt-out, and its Broadcast or Automation is named on
	// its Contact's event.
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
	exists, err := s.Unsubscribe().Query().Where(telegramEverything(st.Destination)...).Exist(ctx)
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
	create := s.Unsubscribe().Create().
		SetChannel(channels.Telegram).
		SetDestination(st.Destination).
		SetSendingSource(eligibility.SourceEverything).
		// The ways of stopping (events.UnsubscribedBy*) are the row's own values.
		SetVia(unsubscribe.Via(st.Via))
	if len(contactIDs) > 0 {
		create.SetContactID(contactIDs[0])
	}
	if err := create.Exec(ctx); err != nil {
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
			e.BroadcastID, e.AutomationID = lo.FromPtr(m.BroadcastID), lo.FromPtr(m.AutomationID)
			if e.BroadcastID != 0 {
				if err := countForBroadcast(ctx, s, e.BroadcastID, c.ID, st.Now); err != nil {
					return err
				}
			}
		}
		if err := pub.Publish(ctx, e); err != nil {
			return err
		}
	}
	return nil
}

// LiftTelegram lifts the Channel-wide opt-out of the user: they asked to receive again, by
// unblocking a bot or by starting one. Nothing to lift is a no-op. Only the `everything` scope is
// lifted: a source-scoped opt-out the person chose by a link in a message stays.
func LiftTelegram(ctx context.Context, s *ent.Scoped, destination string) error {
	if _, err := s.Unsubscribe().Delete().Where(telegramEverything(destination)...).Exec(ctx); err != nil {
		return fmt.Errorf("lift opt-out: %w", err)
	}
	return nil
}

func telegramEverything(destination string) []predicate.Unsubscribe {
	return []predicate.Unsubscribe{
		unsubscribe.ChannelEQ(channels.Telegram),
		unsubscribe.DestinationEQ(destination),
		unsubscribe.SendingSourceEQ(eligibility.SourceEverything),
	}
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

// countForBroadcast marks the Contact's recipient row of the Broadcast as unsubscribed, once.
func countForBroadcast(ctx context.Context, s *ent.Scoped, broadcastID, contactID int64, now time.Time) error {
	_, err := s.BroadcastRecipient().Update().
		Where(broadcastrecipient.BroadcastID(broadcastID), broadcastrecipient.ContactID(contactID), broadcastrecipient.UnsubscribedAtIsNil()).
		SetUnsubscribedAt(now).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("count the unsubscribe for the broadcast: %w", err)
	}
	return nil
}
