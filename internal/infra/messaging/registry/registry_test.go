package registry_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/infra/messaging/registry"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
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
