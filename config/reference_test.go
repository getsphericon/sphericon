package config_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
)

func TestTheCommittedReferenceIsTheConfigurations(t *testing.T) {
	committed, err := os.ReadFile("../docs/configuration.md")
	require.NoError(t, err)

	generated, err := config.Reference()
	require.NoError(t, err)
	assert.Equal(t, string(generated), string(committed), "run `mise run generate` after changing the config package's environment")
}

func TestTheReferenceListsEveryVariable(t *testing.T) {
	reference, err := config.Reference()
	require.NoError(t, err)

	for _, name := range config.Variables() {
		assert.Containsf(t, string(reference), "| `"+name+"` |", "%s has no row in the configuration reference", name)
	}
}
