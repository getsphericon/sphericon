package messaging_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/testhelper/metricstest"
)

type outcomeSender struct{ err error }

func (s outcomeSender) Send(context.Context, messaging.EmailMessage) (messaging.Receipt, error) {
	return messaging.Receipt{MessageID: "m1"}, s.err
}

func (outcomeSender) DefaultFrom() (address, name string) { return "from@example.com", "From" }

// Senders built through the catalog count accepted and error per provider, with
// no other label.
func TestCatalogSenderCountsSendOutcomes(t *testing.T) {
	metricstest.StartMetrics(t)
	failing := errors.New("provider down")
	catalog := messaging.NewCatalog(
		messaging.ProviderDescriptor{
			Channel: messaging.ChannelEmail, Provider: messaging.ProviderSMTP,
			Build: func([]byte, messaging.Deps) (any, error) { return outcomeSender{}, nil },
		},
		messaging.ProviderDescriptor{
			Channel: messaging.ChannelEmail, Provider: messaging.ProviderSES,
			Build: func([]byte, messaging.Deps) (any, error) { return outcomeSender{err: failing}, nil },
		},
	)
	ctx := t.Context()

	smtp, err := catalog.BuildEmail(messaging.ProviderSMTP, nil, messaging.Deps{})
	require.NoError(t, err)
	_, err = smtp.Send(ctx, messaging.EmailMessage{})
	require.NoError(t, err)
	_, err = smtp.Send(ctx, messaging.EmailMessage{})
	require.NoError(t, err)

	ses, err := catalog.BuildEmail(messaging.ProviderSES, nil, messaging.Deps{})
	require.NoError(t, err)
	_, err = ses.Send(ctx, messaging.EmailMessage{})
	require.ErrorIs(t, err, failing)

	scrape := metricstest.ScrapeMetrics(t)
	assert.InDelta(t, 2, scrape.Value(t, "email_send_outcomes_total", map[string]string{"provider": "smtp", "status": "accepted"}), 0)
	assert.InDelta(t, 1, scrape.Value(t, "email_send_outcomes_total", map[string]string{"provider": "ses", "status": "error"}), 0)

	// The decorator keeps the optional DefaultFromer the send path relies on.
	from, ok := smtp.(messaging.DefaultFromer)
	require.True(t, ok)
	addr, _ := from.DefaultFrom()
	assert.Equal(t, "from@example.com", addr)
}

// SES bounce and complaint notifications count under provider "ses".
func TestRecordSendOutcomeBounceAndComplaint(t *testing.T) {
	metricstest.StartMetrics(t)
	messaging.RecordSendOutcome(t.Context(), messaging.ProviderSES, messaging.SendBounce)
	messaging.RecordSendOutcome(t.Context(), messaging.ProviderSES, messaging.SendComplaint)

	scrape := metricstest.ScrapeMetrics(t)
	assert.InDelta(t, 1, scrape.Value(t, "email_send_outcomes_total", map[string]string{"provider": "ses", "status": "bounce"}), 0)
	assert.InDelta(t, 1, scrape.Value(t, "email_send_outcomes_total", map[string]string{"provider": "ses", "status": "complaint"}), 0)
}

// A provider that says "slow down" or "daily quota spent" defers the message
// (ADR 0023): it is not a failed send, so it must not feed the error ratio.
func TestCatalogSenderCountsDeferralsApartFromErrors(t *testing.T) {
	metricstest.StartMetrics(t)
	catalog := messaging.NewCatalog(messaging.ProviderDescriptor{
		Channel: messaging.ChannelEmail, Provider: messaging.ProviderSES,
		Build: func(cfg []byte, _ messaging.Deps) (any, error) {
			switch string(cfg) {
			case "busy":
				return outcomeSender{err: fmt.Errorf("throttled: %w", messaging.ErrBusy)}, nil
			case "quota":
				return outcomeSender{err: fmt.Errorf("daily: %w", messaging.ErrQuotaExceeded)}, nil
			default:
				return outcomeSender{err: errors.New("rejected")}, nil
			}
		},
	})
	ctx := t.Context()
	for _, cfg := range []string{"busy", "quota", "quota", "other"} {
		s, err := catalog.BuildEmail(messaging.ProviderSES, []byte(cfg), messaging.Deps{})
		require.NoError(t, err)
		_, err = s.Send(ctx, messaging.EmailMessage{})
		require.Error(t, err)
	}

	scrape := metricstest.ScrapeMetrics(t)
	labels := func(status string) map[string]string {
		return map[string]string{"provider": "ses", "status": status}
	}
	assert.InDelta(t, 1, scrape.Value(t, "email_send_outcomes_total", labels("busy")), 0)
	assert.InDelta(t, 2, scrape.Value(t, "email_send_outcomes_total", labels("quota_exceeded")), 0)
	assert.InDelta(t, 1, scrape.Value(t, "email_send_outcomes_total", labels("error")), 0)
}
