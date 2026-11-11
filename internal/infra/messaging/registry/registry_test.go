package registry_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/infra/messaging/registry"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
)

// Every Provider an Integration can store has its descriptor in the default catalog, so a stored
// provider is always one the catalog can validate and build.
func TestDefaultRegistersEveryProvider(t *testing.T) {
	cat := registry.Default()
	for _, p := range providers.Provider("").Values() {
		_, ok := cat.Descriptor(providers.Provider(p))
		assert.True(t, ok, "provider %s", p)
	}
	_, ok := cat.Descriptor(providers.Provider("carrier-pigeon"))
	assert.False(t, ok)
}
