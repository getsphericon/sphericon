package channelstop

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/internal/domain/contacts/confirmations"
	"github.com/getsphericon/sphericon/internal/domain/contacts/unsubscribes"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// Subscribers applies the consent reaction in the publication's transaction. The bus owns
// deduplication; this module owns the atomic stop and the distinction between observation and change.
func Subscribers() []events.Subscriber {
	return []events.Subscriber{{
		Wants: func(_ events.Envelope, ev events.DomainEvent) bool {
			switch ev.(type) {
			case *events.SmsStopObserved, *events.SmsStartObserved, *events.TelegramStopObserved, *events.TelegramResumeObserved:
				return true
			default:
				return false
			}
		},
		Handle: react,
	}}
}
func react(ctx context.Context, s *ent.Scoped, pub events.Publisher, env events.Envelope, ev events.DomainEvent) error {
	switch e := ev.(type) {
	case *events.TelegramResumeObserved:
		return unsubscribes.LiftTelegram(ctx, s, e.Destination)
	case *events.TelegramStopObserved:
		return StopTelegram(ctx, s, pub, TelegramStop{
			Destination: e.Destination, IntegrationID: e.IntegrationID, Via: e.Via,
			OutboundMessageID: e.OutboundMessageID, ContactID: e.ContactID, BroadcastID: e.BroadcastID, AutomationID: e.AutomationID,
			Now: env.OccurredAt, DedupKey: e.DedupID,
		})
	case *events.SmsStartObserved:
		return confirmations.StartSMSIn(ctx, s, pub, e.Phone)
	case *events.SmsStopObserved:
		number, err := unsubscribes.SMSNumber(e.Phone)
		if err != nil {
			return err
		}
		optOutID, err := stopSMS(ctx, s, number, unsubscribe.Via(e.Via))
		if err != nil || optOutID == 0 {
			return err
		}
		if err := pub.Publish(ctx, &events.SmsOptOutRecorded{WorkspaceID: s.WorkspaceID(), Phone: e.Phone, ObservedAt: env.OccurredAt}); err != nil {
			return err
		}
		// The public Event is the Contact's projection of the transition; the Broadcast's count
		// follows the internal fact below alone, so it counts the opt-out once.
		if err := publishSMSUnsubscribed(ctx, s, pub, e, number, optOutID); err != nil {
			return err
		}
		if attributesSMSStop(e) {
			return pub.Publish(ctx, &events.MessageUnsubscribed{WorkspaceID: s.WorkspaceID(), OutboundMessageID: e.OutboundMessageID, ObservedAt: env.OccurredAt})
		}
	}
	return nil
}
