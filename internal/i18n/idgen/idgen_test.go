package idgen_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/i18n/idgen"
)

func TestNameCapitalizesEveryWordOfTheKey(t *testing.T) {
	assert.Equal(t, "EmailPasswordResetSubject", idgen.Name("email.password_reset.subject"))
	assert.Equal(t, "ErrorsDestinationEmpty", idgen.Name("errors.destination_empty"))
}

func TestGenerateRefusesTwoKeysOfOneName(t *testing.T) {
	_, err := idgen.Generate([]byte(`{"a.b_c": "one", "a_b.c": "two"}`))

	require.ErrorContains(t, err, "ABC")
}

func TestTheCommittedIDsAreTheCatalogs(t *testing.T) {
	catalog, err := os.ReadFile("../locales/en.json")
	require.NoError(t, err)
	committed, err := os.ReadFile("../ids_gen.go")
	require.NoError(t, err)

	generated, err := idgen.Generate(catalog)
	require.NoError(t, err)
	assert.Equal(t, string(generated), string(committed), "run `mise run generate` after changing locales/en.json")
}
