package channelstop

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/internal/domain/contacts/confirmations"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// Subscribers applies the consent reaction in the publication's transaction. The bus owns
// deduplication; this module owns the atomic stop and the distinction between observation and change.
func Subscribers() []events.Subscriber {
	return []events.Subscriber{{
		Wants: func(_ events.Envelope, ev events.DomainEvent) bool {
			switch ev.(type) {
			case *events.SmsStopObserved, *events.SmsStartObserved:
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
	case *events.SmsStartObserved:
		return confirmations.StartSMSIn(ctx, s, pub, e.Phone)
	case *events.SmsStopObserved:
		changed, err := stopSMS(ctx, s, e.Phone, unsubscribe.Via(e.Via))
		if err != nil || !changed {
			return err
		}
		if err := pub.Publish(ctx, &events.SmsOptOutRecorded{WorkspaceID: s.WorkspaceID(), Phone: e.Phone, ObservedAt: env.OccurredAt}); err != nil {
			return err
		}
		if e.OutboundMessageID != 0 && e.Via == events.UnsubscribedByLink {
			return pub.Publish(ctx, &events.MessageUnsubscribed{WorkspaceID: s.WorkspaceID(), OutboundMessageID: e.OutboundMessageID, ObservedAt: env.OccurredAt})
		}
	}
	return nil
}
