package outbound

import (
	"context"
	"fmt"
	"time"

	"entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
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
		Where(outboundmessage.ProviderMessageID(providerMessageID)).
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

// DeliveryResult is one of the stamps of a message's delivery result.
type DeliveryResult string

const (
	ResultDelivered  DeliveryResult = "delivered"
	ResultBounced    DeliveryResult = "bounced"
	ResultComplained DeliveryResult = "complained"
)

// StampDelivery sets the message's delivery result of the given kind to at, only while it is
// empty: the first notice of each kind stamps once and a stamp is never replaced or cleared,
// so a redelivered notice changes nothing and an old report does not shrink (ADR 0053).
func StampDelivery(ctx context.Context, s *ent.Scoped, id int64, result DeliveryResult, at time.Time) error {
	u := s.OutboundMessage().Update()
	switch result {
	case ResultDelivered:
		u.Where(outboundmessage.ID(id), outboundmessage.DeliveredAtIsNil()).SetDeliveredAt(at)
	case ResultBounced:
		u.Where(outboundmessage.ID(id), outboundmessage.BouncedAtIsNil()).SetBouncedAt(at)
	case ResultComplained:
		u.Where(outboundmessage.ID(id), outboundmessage.ComplainedAtIsNil()).SetComplainedAt(at)
	default:
		return fmt.Errorf("unknown delivery result %q", result)
	}
	if _, err := u.Save(ctx); err != nil {
		return fmt.Errorf("stamp message %d %s: %w", id, result, err)
	}
	return nil
}
