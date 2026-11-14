package suspension_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/identity/suspension"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// setDefault sets the default Send cap to perDay (nil clears it) in a transaction of its own, as
// the Staff's change does, and returns what SetIn reported.
func setDefault(t *testing.T, client *ent.Client, d *suspension.DefaultSendCap, perDay *int) (map[string]any, bool) {
	t.Helper()
	tx, err := client.Tx(t.Context())
	require.NoError(t, err)
	change, changed, err := d.SetIn(t.Context(), tx.Client(), perDay)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	return change, changed
}

// startingCap is the Send cap a Workspace created now starts with.
func startingCap(t *testing.T, client *ent.Client, d *suspension.DefaultSendCap) *int {
	t.Helper()
	sendCap, err := d.StartingIn(t.Context(), client)
	require.NoError(t, err)
	return sendCap
}

// On an Instance with Send caps a new Workspace starts with the default as it stands, and each
// change of the default reports the old and the new value.
func TestANewWorkspaceStartsWithTheDefaultSendCapAsItStands(t *testing.T) {
	env := testhelper.Setup(t)
	d := suspension.NewDefaultSendCap(env.DB, true)

	before := startingCap(t, env.DB, d)
	assert.Nil(t, before, "an Instance starts with no default")

	change, changed := setDefault(t, env.DB, d, new(500))
	set := startingCap(t, env.DB, d)
	assert.True(t, changed)
	assert.Equal(t, map[string]any{"to": 500}, change)
	assert.Equal(t, new(500), set)

	change, changed = setDefault(t, env.DB, d, new(2000))
	assert.True(t, changed)
	assert.Equal(t, map[string]any{"from": 500, "to": 2000}, change)

	change, changed = setDefault(t, env.DB, d, nil)
	cleared := startingCap(t, env.DB, d)
	assert.True(t, changed)
	assert.Equal(t, map[string]any{"from": 2000}, change)
	assert.Nil(t, cleared)
}

// Setting the default it already has, or clearing none, changes nothing.
func TestSettingTheSameDefaultSendCapChangesNothing(t *testing.T) {
	env := testhelper.Setup(t)
	d := suspension.NewDefaultSendCap(env.DB, true)

	_, changed := setDefault(t, env.DB, d, nil)
	assert.False(t, changed, "clearing none")

	setDefault(t, env.DB, d, new(500))
	_, changed = setDefault(t, env.DB, d, new(500))
	kept := startingCap(t, env.DB, d)
	assert.False(t, changed, "the same value")
	assert.Equal(t, new(500), kept)
}

// A default outside the contract's bounds is refused and leaves the stored one.
func TestADefaultSendCapOutsideItsBoundsIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	d := suspension.NewDefaultSendCap(env.DB, true)
	setDefault(t, env.DB, d, new(500))

	_, _, err := d.SetIn(t.Context(), env.DB, new(0))
	kept := startingCap(t, env.DB, d)

	require.ErrorIs(t, err, validation.ErrOutOfRange)
	assert.Equal(t, new(500), kept)
}

// On an Instance without Send caps there is no default: a new Workspace starts with none, whatever
// default is stored (say, from a restored dump), none is shown, and a change is refused.
func TestAnInstanceWithoutSendCapsHasNoDefaultSendCap(t *testing.T) {
	env := testhelper.Setup(t)
	setDefault(t, env.DB, suspension.NewDefaultSendCap(env.DB, true), new(500))
	d := suspension.NewDefaultSendCap(env.DB, false)

	sendCap := startingCap(t, env.DB, d)
	shown, ok, getErr := d.Get(t.Context())
	_, changed, setErr := d.SetIn(t.Context(), env.DB, new(700))

	assert.Nil(t, sendCap)
	require.NoError(t, getErr)
	assert.False(t, ok)
	assert.Nil(t, shown)
	require.ErrorIs(t, setErr, suspension.ErrNoSendCaps)
	assert.False(t, changed)
}
