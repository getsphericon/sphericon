package providers_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
)

// Every Provider sends on one Channel, and each Channel lists exactly the Providers that send on it.
func TestEveryProviderSendsOnOneChannel(t *testing.T) {
	for _, v := range providers.Provider("").Values() {
		p := providers.Provider(v)
		ch := p.Channel()
		assert.NotEmpty(t, ch, "provider %s", p)
		assert.Contains(t, providers.OnChannel(ch), p)
	}
	assert.Equal(t, channels.Email, providers.SES.Channel())
	assert.Equal(t, channels.SMS, providers.Twilio.Channel())
	assert.Empty(t, providers.Provider("carrier-pigeon").Channel())
}
