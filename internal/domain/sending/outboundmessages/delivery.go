package outboundmessages

import (
	"context"
	"fmt"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
)

// FindByProviderMessageID is the Outbound message a Delivery notice names by the provider's
// message id (ADR 0053), nil when there is none: the notice then stays at the level of the
// address. Several messages with the same id (a provider that reuses ids) resolve to the
// newest.
func FindByProviderMessageID(ctx context.Context, s *ent.Scoped, providerMessageID string) (*ent.OutboundMessage, error) {
	if providerMessageID == "" {
		return nil, nil
	}
	m, err := s.OutboundMessage().Query().
		Where(outboundmessage.ProviderMessageID(providerMessageID), outboundmessage.ChannelEQ(channels.Email)).
		Order(outboundmessage.ByID(sql.OrderDesc())).
		First(ctx)
	if ent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find message by provider id: %w", err)
	}
	return m, nil
}

// FindTelegramMessage is the Outbound message a Telegram button press was made on, nil when none
// resolves. A Telegram message id is only unique within its chat, so the press is matched by the
// bot it came to, the user who pressed it (the Destination recorded as the first Binding the
// message was accepted for) and the message id. A message that went to several chats of one
// Contact is found through the first of them only; a press in another does not resolve and is
// not attributed, but is still the opt-out.
func FindTelegramMessage(ctx context.Context, s *ent.Scoped, integrationID int64, destination, providerMessageID string) (*ent.OutboundMessage, error) {
	if providerMessageID == "" {
		return nil, nil
	}
	m, err := s.OutboundMessage().Query().
		Where(
			outboundmessage.ChannelEQ(channels.Telegram),
			outboundmessage.IntegrationID(integrationID),
			outboundmessage.Destination(destination),
			outboundmessage.ProviderMessageID(providerMessageID),
		).
		Order(outboundmessage.ByID(sql.OrderDesc())).
		First(ctx)
	if ent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find telegram message: %w", err)
	}
	return m, nil
}

// FindSMSMessage is the Outbound message an SMS delivery report names, nil when none resolves: the
// text with the provider's id (Twilio's SID) sent through the Integration the report came to. A SID
// is unique only per provider account, so a report never reaches a text of another Integration.
func FindSMSMessage(ctx context.Context, s *ent.Scoped, integrationID int64, providerMessageID string) (*ent.OutboundMessage, error) {
	if providerMessageID == "" {
		return nil, nil
	}
	m, err := s.OutboundMessage().Query().
		Where(
			outboundmessage.ChannelEQ(channels.SMS),
			outboundmessage.IntegrationID(integrationID),
			outboundmessage.ProviderMessageID(providerMessageID),
		).
		Order(outboundmessage.ByID(sql.OrderDesc())).
		First(ctx)
	if ent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find sms message: %w", err)
	}
	return m, nil
}

// FindOrigin is the Ref of the Outbound message id whose Broadcast or Automation it came from (only
// BroadcastID and AutomationID are set), the zero Ref when the message is not the scope's: an SMS
// unsubscribe link carries only the message, since its source is everything (ADR 0077).
func FindOrigin(ctx context.Context, s *ent.Scoped, id int64) (Ref, error) {
	m, err := s.OutboundMessage().Query().Where(outboundmessage.ID(id)).
		Select(outboundmessage.FieldBroadcastID, outboundmessage.FieldAutomationID).
		Only(ctx)
	if ent.IsNotFound(err) {
		return Ref{}, nil
	}
	if err != nil {
		return Ref{}, fmt.Errorf("find the message's origin: %w", err)
	}
	return Ref{BroadcastID: lo.FromPtr(m.BroadcastID), AutomationID: lo.FromPtr(m.AutomationID)}, nil
}

// StampFinal sets a message's final delivery state, delivered or undelivered (ADR 0079). Only a
// message the provider accepted (sent) takes one, and it moves only forward: the first final
// report stamps it, and a later one of either kind, a redelivery included, is
// lifecycle.ErrInvalidTransition, as is a report of a message that was not sent. An undelivered
// stamp keeps the provider's error code, nil when it gave none.
//
// The final report is one latch over the pair (ADR 0109): the write that sets either moment
// matches only while both are null, and the table refuses a row with both
// (outbound_messages_final_report_check).
func StampFinal(ctx context.Context, s *ent.Scoped, id int64, result DeliveryResult, code string, at time.Time) error {
	u := s.OutboundMessage().Update().
		Where(
			outboundmessage.ID(id), outboundmessage.StatusEQ(outboundmessage.StatusSent),
			outboundmessage.DeliveredAtIsNil(), outboundmessage.UndeliveredAtIsNil(),
		)
	switch result {
	case ResultDelivered:
		u.SetDeliveredAt(at)
	case ResultUndelivered:
		u.SetUndeliveredAt(at).SetNillableUndeliveredCode(lo.EmptyableToPtr(code))
	case ResultBounced, ResultComplained:
		return fmt.Errorf("%s is no final delivery state", result)
	default:
		return fmt.Errorf("unknown delivery result %q", result)
	}
	if err := lifecycle.Latch(ctx, func(ctx context.Context) (bool, error) {
		n, err := u.Save(ctx)
		return n == 1, err
	}); err != nil {
		return fmt.Errorf("stamp message %d %s: %w", id, result, err)
	}
	return nil
}

// DeliveryResult is one of the stamps of a message's delivery result.
type DeliveryResult string

const (
	ResultDelivered   DeliveryResult = "delivered"
	ResultUndelivered DeliveryResult = "undelivered"
	ResultBounced     DeliveryResult = "bounced"
	ResultComplained  DeliveryResult = "complained"
)

// StampDelivery retains the earliest observed email result of each kind (ADR 0113), so
// reversed consumer order and retries cannot change the report's meaning. SMS's mutually
// exclusive final result still goes through StampFinal synchronously.
func StampDelivery(ctx context.Context, s *ent.Scoped, id int64, result DeliveryResult, at time.Time) error {
	u := s.OutboundMessage().Update()
	switch result {
	case ResultDelivered:
		u.Where(outboundmessage.ID(id), outboundmessage.Or(outboundmessage.DeliveredAtIsNil(), outboundmessage.DeliveredAtGT(at))).SetDeliveredAt(at)
	case ResultBounced:
		u.Where(outboundmessage.ID(id), outboundmessage.Or(outboundmessage.BouncedAtIsNil(), outboundmessage.BouncedAtGT(at))).SetBouncedAt(at)
	case ResultComplained:
		u.Where(outboundmessage.ID(id), outboundmessage.Or(outboundmessage.ComplainedAtIsNil(), outboundmessage.ComplainedAtGT(at))).SetComplainedAt(at)
	case ResultUndelivered:
		return fmt.Errorf("%s is a final state: StampFinal sets it", result)
	default:
		return fmt.Errorf("unknown delivery result %q", result)
	}
	if _, err := u.Save(ctx); err != nil {
		return fmt.Errorf("stamp message %d %s: %w", id, result, err)
	}
	return nil
}
