package accounts_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A User's email is stored as registered (trimmed, case kept) while the login module
// lower-cases the address it looks up, so the lookup must ignore case: otherwise a User
// registered as "Grace@Example.com" could never sign in.
func TestUserByLoginAddressIgnoresTheCaseOfTheStoredEmail(t *testing.T) {
	env := testhelper.Setup(t)
	acc := accounts.New(env.DB, env.Bus)
	u, err := acc.Register(t.Context(), "Grace", "Grace@Example.com", "hash", false)
	require.NoError(t, err)

	found, err := acc.UserByLoginAddress(t.Context(), "grace@example.com")
	require.NoError(t, err)
	assert.Equal(t, u.ID, found.ID)

	_, err = acc.UserByLoginAddress(t.Context(), "nobody@example.com")
	assert.True(t, ent.IsNotFound(err))
}
