package emailcase_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A Contact's address is lower-cased by the client itself, so a write that skips the contacts
// module still keeps the address one alias key and matches the Destination of its Suppressions.
func TestAContactAddressIsStoredLowerCased(t *testing.T) {
	env := testhelper.Setup(t)
	s := events.Unaudited(env.DB.Scoped(fixtures.AcmeID))

	// test writes: the hook acts on a write through the client, which is what is tested.
	created, err := s.Contact().Create().SetEmail("Zed.Case@Example.COM").Save(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "zed.case@example.com", *created.Email)

	updated, err := s.Contact().UpdateOneID(created.ID).SetEmail("ZED@example.com").Save(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "zed@example.com", *updated.Email)
}
