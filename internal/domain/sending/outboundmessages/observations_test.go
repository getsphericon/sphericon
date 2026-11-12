package outboundmessages_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
)

// Measurements require a delivery reference, retain the observation time and never turn
// machine engagement or non-measurement facts into report work.
func TestObservationSelectionPreservesAnonymousMeasurementInvariants(t *testing.T) {
	rapid.Check(t, checkObservationSelection)
}

func checkObservationSelection(t *rapid.T) {
	id := rapid.Int64Min(0).Draw(t, "message")
	machine := rapid.Bool().Draw(t, "machine")
	at := time.Unix(rapid.Int64Range(0, 1<<32).Draw(t, "seconds"), 0).UTC()
	noise := rapid.StringMatching(`unrecognized_[a-z]{1,20}`).Draw(t, "unknown action")
	type sample struct {
		event     events.DomainEvent
		action    string
		humanOnly bool
	}
	cases := []sample{
		{&events.EmailEngagement{Action: systemevent.EmailOpened, OutboundMessageID: id, Machine: machine}, systemevent.EmailOpened, true},
		{&events.EmailEngagement{Action: systemevent.EmailClicked, OutboundMessageID: id, Machine: machine}, systemevent.EmailClicked, true},
		{&events.SmsClicked{OutboundMessageID: id, Machine: machine}, systemevent.SmsClicked, true},
		{&events.TelegramClicked{OutboundMessageID: id, Machine: machine}, systemevent.TelegramClicked, true},
		{&events.EmailDelivery{Action: systemevent.EmailDelivered, OutboundMessageID: id}, systemevent.EmailDelivered, false},
		{&events.EmailDelivery{Action: systemevent.EmailBounced, OutboundMessageID: id}, systemevent.EmailBounced, false},
		{&events.EmailDelivery{Action: systemevent.EmailComplained, OutboundMessageID: id}, systemevent.EmailComplained, false},
		{&events.EmailEngagement{Action: noise, OutboundMessageID: id}, "", false},
		{&events.EmailDelivery{Action: noise, OutboundMessageID: id}, "", false},
		{&events.EmailDelivery{Action: systemevent.EmailDelayed, OutboundMessageID: id}, "", false},
		{&events.EmailDelivery{Action: systemevent.EmailDropped, OutboundMessageID: id}, "", false},
		{&events.SmsDelivery{Action: systemevent.SmsDelivered, OutboundMessageID: id}, "", false},
		{&events.SmsStopObserved{OutboundMessageID: id}, "", false},
	}
	selected := rapid.SampledFrom(cases).Draw(t, "event")
	got, accepted := outboundmessages.ObservationOf(events.Envelope{OccurredAt: at}, selected.event)
	want := selected.action != "" && id != 0 && (!selected.humanOnly || !machine)
	require.Equal(t, want, accepted)
	if accepted {
		assert.Equal(t, outboundmessages.Observation{MessageID: id, Action: selected.action, ObservedAt: at}, got)
	}
}
