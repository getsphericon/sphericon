package preset_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/sending/preset"
)

// Every built-in Preset the core names in code is one the Instance loads, so a renamed or removed
// builtin/<name>.yaml fails here instead of leaving an Integration with no Preset to read with.
func TestEveryNamedBuiltinIsLoaded(t *testing.T) {
	registry, err := preset.Load(nil)
	require.NoError(t, err)

	for _, name := range []string{preset.BuiltinSES, preset.BuiltinTelegram, preset.BuiltinTwilio} {
		_, ok := registry.Get(name)
		assert.True(t, ok, "no built-in Preset %q", name)
	}
}
