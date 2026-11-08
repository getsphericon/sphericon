package suppressions

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	entsuppression "github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
)

// Subscribers applies semantic unreachable verdicts synchronously, with one owner of the write.
func Subscribers() []events.Subscriber {
	return []events.Subscriber{{
		Wants: func(_ events.Envelope, ev events.DomainEvent) bool {
			_, ok := ev.(*events.SmsDestinationUnreachable)
			return ok
		},
		Handle: func(ctx context.Context, s *ent.Scoped, pub events.Publisher, env events.Envelope, ev events.DomainEvent) error {
			e, ok := ev.(*events.SmsDestinationUnreachable)
			if !ok {
				return nil
			}
			changed, err := add(ctx, s, Entry{Channel: channels.SMS, Destination: e.Phone, ContactID: e.ContactID, Reason: entsuppression.ReasonBounce})
			if err != nil || !changed {
				return err
			}
			return pub.Publish(ctx, &events.SmsSuppressionRecorded{WorkspaceID: s.WorkspaceID(), Phone: e.Phone, ObservedAt: env.OccurredAt})
		},
	}}
}
