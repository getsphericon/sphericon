package workspaces_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A Workspace id another Workspace has inserts nothing: CreateIn draws again, and a generator that
// only draws taken ids is a fault after a few draws, the transaction it ran in still usable.
func TestCreateInGivesUpOnAGeneratorThatOnlyDrawsTakenIDs(t *testing.T) {
	env := testhelper.Setup(t)
	draws := 0
	taken := func() string { draws++; return fixtures.AcmePublicID }
	var err error

	txErr := env.Bus.WithinTx(t.Context(), func(tx *ent.Client, _ events.Publisher) error {
		_, err = workspaces.CreateIn(t.Context(), tx, "Fresh", taken, nil)
		return nil
	})

	require.NoError(t, txErr)
	require.ErrorContains(t, err, "every workspace id drawn is taken")
	assert.Equal(t, 3, draws, "it draws a few ids, then gives up")
}

func TestCreateInKeepsTheFirstIDThatIsFree(t *testing.T) {
	env := testhelper.Setup(t)
	ids := []string{fixtures.AcmePublicID, "FreshId1"}
	next := func() string {
		id := ids[0]
		ids = ids[1:]
		return id
	}

	var ws *ent.Workspace
	err := env.Bus.WithinTx(t.Context(), func(tx *ent.Client, _ events.Publisher) error {
		var err error
		ws, err = workspaces.CreateIn(t.Context(), tx, "Fresh", next, nil)
		return err
	})

	require.NoError(t, err)
	assert.Equal(t, "FreshId1", ws.PublicID)
}

// A new Workspace takes the next service feed position as it is created (ADR 0062), held in its
// standing beside it (ADR 0168), which the creation makes.
func TestCreateInGivesTheWorkspaceItsStandingWithAFeedPosition(t *testing.T) {
	env := testhelper.Setup(t)
	top, err := workspaces.TopFeedPosition(t.Context(), env.DB)
	require.NoError(t, err)

	var ws *ent.Workspace
	err = env.Bus.WithinTx(t.Context(), func(tx *ent.Client, _ events.Publisher) error {
		var err error
		ws, err = workspaces.CreateIn(t.Context(), tx, "Fresh", func() string { return "FreshId1" }, nil)
		return err
	})

	require.NoError(t, err)
	standing, err := workspaces.StandingIn(t.Context(), env.DB, ws.ID)
	require.NoError(t, err)
	assert.Equal(t, top+1, standing.FeedPosition)
	assert.Nil(t, standing.SetupCompletedAt)
}
