package registry_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/channels"
	"github.com/getsphericon/sphericon/internal/messaging/registry"
	"github.com/getsphericon/sphericon/internal/providers"
)

func TestDefaultRegistersBuiltInProviders(t *testing.T) {
	cat := registry.Default()
	for _, p := range []providers.Provider{providers.SMTP, providers.SES} {
		_, ok := cat.Descriptor(channels.Email, p)
		assert.True(t, ok, "provider %s", p)
	}
	_, ok := cat.Descriptor(channels.SMS, providers.SMTP)
	assert.False(t, ok)
}
