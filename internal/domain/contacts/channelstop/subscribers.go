package channelstop

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/internal/domain/contacts/consentstamps"
	"github.com/getsphericon/sphericon/internal/domain/contacts/unsubscribes"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
)

// Subscribers applies the consent reaction in the publication's transaction. The bus owns
// deduplication; this module owns the atomic stop and the distinction between observation and
// change. The bus that registers them is the module's own: the reaction publishes in the
// publication's transaction through it.
func (m *Module) Subscribers() []events.Subscriber {
	return []events.Subscriber{{
		Wants: func(_ events.Envelope, ev events.DomainEvent) bool {
			switch ev.(type) {
			case *events.SmsStopObserved, *events.SmsStartObserved, *events.TelegramStopObserved, *events.TelegramResumeObserved:
				return true
			default:
				return false
			}
		},
		Handle: m.react,
	}}
}

// react applies an observation in the order it occurred (ADR 0154): the envelope's time is when it
// did (its ObservedAt, else when it was published). One older than the consent standing changes
// nothing and is recorded as contact.consent_superseded for the Contacts the Destination is theirs.
func (m *Module) react(ctx context.Context, s *ent.Scoped, _ events.Publisher, env events.Envelope, ev events.DomainEvent) error {
	switch e := ev.(type) {
	case *events.TelegramResumeObserved:
		at := consentstamps.Occurrence{At: env.OccurredAt, IntegrationID: e.IntegrationID, Sequence: e.Sequence}
		applied, err := m.unsubs.LiftTelegram(ctx, s, e.Destination, at)
		if err != nil || applied {
			return err
		}
		contactIDs, err := contactsOf(ctx, s, e.Destination)
		if err != nil {
			return err
		}
		return m.publishSuperseded(ctx, s, contactIDs, &events.ConsentSuperseded{
			Channel: string(channels.Telegram), Observation: events.ObservationStart, IntegrationID: e.IntegrationID,
			DedupID: e.DedupID, ObservedAt: env.OccurredAt,
		})
	case *events.TelegramStopObserved:
		return m.StopTelegram(ctx, s, TelegramStop{
			Destination: e.Destination, IntegrationID: e.IntegrationID, Sequence: e.Sequence, Via: e.Via,
			OutboundMessageID: e.OutboundMessageID, ContactID: e.ContactID, BroadcastID: e.BroadcastID, AutomationID: e.AutomationID,
			At: env.OccurredAt, DedupKey: e.DedupID,
		})
	case *events.SmsStartObserved:
		applied, err := m.confirmations.StartSMSIn(ctx, s, e.Phone, consentstamps.At(env.OccurredAt))
		if err != nil || applied {
			return err
		}
		return m.publishSMSSuperseded(ctx, s, e.Phone, &events.ConsentSuperseded{
			Observation: events.ObservationStart, DedupID: e.DedupID, ObservedAt: env.OccurredAt,
		})
	case *events.SmsStopObserved:
		number, err := unsubscribes.SMSNumber(e.Phone)
		if err != nil {
			return err
		}
		change, err := m.stopSMS(ctx, s, number, unsubscribe.Via(e.Via), consentstamps.At(env.OccurredAt))
		if err != nil {
			return err
		}
		if change.Superseded {
			return m.publishSMSSuperseded(ctx, s, number, &events.ConsentSuperseded{
				Observation: events.ObservationStop, Via: e.Via, DedupID: e.DedupID, ObservedAt: env.OccurredAt,
			})
		}
		if change.ID == 0 {
			return nil
		}
		if err := m.bus.PublishIn(ctx, s, &events.SmsOptOutRecorded{WorkspaceID: s.WorkspaceID(), Phone: e.Phone, ObservedAt: env.OccurredAt}); err != nil {
			return err
		}
		// The public Event is the Contact's projection of the transition; the Broadcast's count
		// follows the internal fact below alone, so it counts the opt-out once.
		if err := m.publishSMSUnsubscribed(ctx, s, e, number, change.ID); err != nil {
			return err
		}
		if attributesSMSStop(e) {
			return m.bus.PublishIn(ctx, s, &events.MessageUnsubscribed{WorkspaceID: s.WorkspaceID(), OutboundMessageID: e.OutboundMessageID, ObservedAt: env.OccurredAt})
		}
	}
	return nil
}
