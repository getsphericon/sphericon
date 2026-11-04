package twilio_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/messaging/twilio"
)

func TestFromKindNamesWhatTwilioSendsFrom(t *testing.T) {
	cases := map[string]messaging.SMSFromKind{
		"+15017250604":                       messaging.SMSFromNumber,
		"Acme Shop":                          messaging.SMSFromAlphanumeric,
		"ACME":                               messaging.SMSFromAlphanumeric,
		"MG9752274e9e519418a7406176694466fa": messaging.SMSFromMessagingService,
	}
	for from, want := range cases {
		got, ok := twilio.FromKind(from)
		assert.True(t, ok, from)
		assert.Equal(t, want, got, from)
	}
	for _, from := range []string{"", "15017250604", "+0123456789", "Acme Shop Online", "12345", "Acme!", "+1 501 725 0604"} {
		_, ok := twilio.FromKind(from)
		assert.False(t, ok, "%q is no sender", from)
	}
}

// Twilio's answer to a send is sorted by whom it concerns (ADR 0079): the account, the rate, the
// number, a number that reaches no handset, or nothing anyone can tell.
func TestClassifySortsTwilioRefusalsByWhomTheyConcern(t *testing.T) {
	cases := []struct {
		name   string
		status int
		code   int
		want   messaging.SMSRefusal
	}{
		{"credentials refused", 401, 20003, messaging.SMSRefusalAccount},
		{"a 401 with any code", 401, 0, messaging.SMSRefusalAccount},
		{"a 403", 403, 0, messaging.SMSRefusalAccount},
		{"account not active", 400, 20005, messaging.SMSRefusalAccount},
		{"invalid From", 400, 21212, messaging.SMSRefusalAccount},
		{"From not the account's", 400, 21606, messaging.SMSRefusalAccount},
		{"From not a Twilio number", 400, 21659, messaging.SMSRefusalAccount},
		{"From and account mismatch", 400, 21660, messaging.SMSRefusalAccount},
		{"no such Messaging Service", 400, 21701, messaging.SMSRefusalAccount},
		{"Messaging Service with no numbers", 400, 21704, messaging.SMSRefusalAccount},
		{"too many requests", 429, 20429, messaging.SMSRefusalRate},
		{"a 429 with any code", 429, 0, messaging.SMSRefusalRate},
		{"queue full", 400, 21611, messaging.SMSRefusalRate},
		{"send rate between two numbers", 400, 14107, messaging.SMSRefusalRate},
		{"invalid To", 400, 21211, messaging.SMSRefusalDestination},
		{"To region not permitted", 400, 21408, messaging.SMSRefusalDestination},
		{"unverified To on a trial", 400, 21608, messaging.SMSRefusalDestination},
		{"To and From do not work together", 400, 21612, messaging.SMSRefusalDestination},
		{"not a mobile number", 400, 21614, messaging.SMSRefusalDeadNumber},
		{"unknown handset", 400, 30005, messaging.SMSRefusalDeadNumber},
		{"landline", 400, 30006, messaging.SMSRefusalDeadNumber},
		{"carrier filter", 400, 30007, messaging.SMSRefusalAmbiguous},
		{"handset unreachable", 400, 30003, messaging.SMSRefusalAmbiguous},
		{"an unknown code", 400, 29999, messaging.SMSRefusalAmbiguous},
		{"a server error", 500, 20500, messaging.SMSRefusalTransient},
		{"a 503 with any code", 503, 0, messaging.SMSRefusalTransient},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, twilio.Classify(tc.status, tc.code))
		})
	}
}
