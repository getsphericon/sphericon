package outboundmessages_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/infra/events"
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
		{&events.EmailEngagement{Action: events.NameEmailOpened, OutboundMessageID: id, Machine: machine}, events.NameEmailOpened, true},
		{&events.EmailEngagement{Action: events.NameEmailClicked, OutboundMessageID: id, Machine: machine}, events.NameEmailClicked, true},
		{&events.SmsClicked{OutboundMessageID: id, Machine: machine}, events.NameSmsClicked, true},
		{&events.TelegramClicked{OutboundMessageID: id, Machine: machine}, events.NameTelegramClicked, true},
		{&events.EmailDelivery{Action: events.NameEmailDelivered, OutboundMessageID: id}, events.NameEmailDelivered, false},
		{&events.EmailDelivery{Action: events.NameEmailBounced, OutboundMessageID: id}, events.NameEmailBounced, false},
		{&events.EmailDelivery{Action: events.NameEmailComplained, OutboundMessageID: id}, events.NameEmailComplained, false},
		{&events.EmailEngagement{Action: noise, OutboundMessageID: id}, "", false},
		{&events.EmailDelivery{Action: noise, OutboundMessageID: id}, "", false},
		{&events.EmailDelivery{Action: events.NameEmailDelayed, OutboundMessageID: id}, "", false},
		{&events.EmailDelivery{Action: events.NameEmailDropped, OutboundMessageID: id}, "", false},
		{&events.SmsDelivery{Action: events.NameSmsDelivered, OutboundMessageID: id}, "", false},
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
