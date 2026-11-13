package usersunchecked_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/domain/identity/users/usersunchecked"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// deleteProven opens the deletion of the User the proof was made on, proven, and applies it, in
// one transaction.
func deleteProven(t *testing.T, env *testhelper.TestEnv, proven *ent.User) error {
	t.Helper()
	ctx := t.Context()
	return env.Bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		d, err := usersunchecked.DeletionIn(ctx, tx, proven)
		if err != nil {
			return err
		}
		return d.Apply(ctx, pub)
	})
}

// A proof made before the User's password was set or removed no longer fits the User (ADR 0050):
// the deletion is refused on opening and the User stays.
func TestTheDeletionRefusesAProofThatPredatesAPasswordChange(t *testing.T) {
	for name, tc := range map[string]struct {
		userID int64
		hash   string
	}{
		"password removed": {userID: fixtures.PasskeyPatID, hash: ""},
		"password set":     {userID: fixtures.PasskeyOnlyPegID, hash: "a-hash-set-after-the-proof"},
	} {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			ctx := t.Context()
			proven, err := env.DB.User.Get(ctx, tc.userID)
			require.NoError(t, err)
			require.NoError(t, env.FeedDB.User.UpdateOneID(tc.userID).SetPasswordHash(tc.hash).Exec(ctx))

			err = deleteProven(t, env, proven)

			require.ErrorIs(t, err, users.ErrProofStale)
			var refused *users.DeletionRefusedError
			require.ErrorAs(t, err, &refused)
			_, err = env.DB.User.Get(ctx, tc.userID)
			require.NoError(t, err, "the User is not deleted")
			published := env.QueuedEnvelopes(t, events.NameUserDeleted)
			assert.Empty(t, published, "nothing is published")
		})
	}
}

// A proof that still fits the User deletes them and publishes the deletion.
func TestApplyingTheDeletionOfAProofThatStillFitsRemovesTheUser(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	proven, err := env.DB.User.Get(ctx, fixtures.PasskeyOnlyPegID)
	require.NoError(t, err)

	err = deleteProven(t, env, proven)

	require.NoError(t, err)
	_, err = env.DB.User.Get(ctx, fixtures.PasskeyOnlyPegID)
	notFound := ent.IsNotFound(err)
	assert.True(t, notFound, "the User is deleted")
	published := env.QueuedEnvelopes(t, events.NameUserDeleted)
	assert.Len(t, published, 1)
}
