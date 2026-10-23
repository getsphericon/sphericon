package messaging

import (
	"context"
	"errors"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const otelScope = "github.com/getsphericon/sphericon/internal/messaging"

// SendStatus is the outcome label of the email.send.outcomes counter. The set is
// closed: the counter carries only provider and status, never a tenant or
// recipient identifier (ADR 0018).
type SendStatus string

const (
	// SendAccepted: the provider accepted the message.
	SendAccepted SendStatus = "accepted"
	// SendError: the provider call failed.
	SendError SendStatus = "error"
	// SendBusy: the provider said we are sending too fast; the message is deferred
	// and retried, not failed (ADR 0023). Kept out of the error ratio.
	SendBusy SendStatus = "busy"
	// SendQuotaExceeded: the provider's daily quota is spent; the message is deferred
	// (ADR 0023). Kept out of the error ratio.
	SendQuotaExceeded SendStatus = "quota_exceeded"
	// SendBounce: the provider reported a bounce for a message it had accepted.
	SendBounce SendStatus = "bounce"
	// SendComplaint: the provider reported a spam complaint.
	SendComplaint SendStatus = "complaint"
)

// RecordSendOutcome counts one send outcome for a provider on the global meter
// provider (a no-op when telemetry is disabled).
func RecordSendOutcome(ctx context.Context, provider Provider, status SendStatus) {
	counter, err := otel.Meter(otelScope).Int64Counter(
		"email.send.outcomes",
		metric.WithDescription("Email send outcomes by provider and status."),
	)
	if err != nil {
		return
	}
	counter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("provider", string(provider)),
		attribute.String("status", string(status)),
	))
}

// instrumentedSender counts accepted, error, busy and quota_exceeded per provider
// around Send.
type instrumentedSender struct {
	EmailSender

	provider Provider
}

// DefaultFrom forwards the wrapped sender's DefaultFromer, which the decorator
// would otherwise hide from the send path (see DefaultFromer).
func (s instrumentedSender) DefaultFrom() (string, string) {
	if d, ok := s.EmailSender.(DefaultFromer); ok {
		return d.DefaultFrom()
	}
	return "", ""
}

func (s instrumentedSender) Send(ctx context.Context, msg EmailMessage) (Receipt, error) {
	receipt, err := s.EmailSender.Send(ctx, msg)
	RecordSendOutcome(ctx, s.provider, sendStatusOf(err))
	return receipt, err
}

// sendStatusOf is the outcome label of one Send result: a deferral (the provider is
// busy or its quota is spent) is not an error.
func sendStatusOf(err error) SendStatus {
	switch {
	case err == nil:
		return SendAccepted
	case errors.Is(err, ErrQuotaExceeded):
		return SendQuotaExceeded
	case errors.Is(err, ErrBusy):
		return SendBusy
	default:
		return SendError
	}
}
