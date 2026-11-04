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
