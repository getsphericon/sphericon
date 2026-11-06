package users_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/users"
)

// The lookup of a login address ignores its case, so an address typed in another case than the
// stored one still finds its User.
func TestUserByLoginAddressIgnoresTheCaseOfTheAddress(t *testing.T) {
	env := testhelper.Setup(t)
	acc := users.New(env.DB, env.Bus, testhelper.Passwords(), env.ConsentRecords)

	found, err := acc.UserByLoginAddress(t.Context(), "Info@GetSphericon.com")
	require.NoError(t, err)
	assert.EqualValues(t, fixtures.OwnerJohnID, found.ID)

	_, err = acc.UserByLoginAddress(t.Context(), "nobody@example.com")
	assert.True(t, ent.IsNotFound(err))
}
