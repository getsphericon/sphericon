package workspaces_test

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/workspacestanding"
	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/cleanup"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A Workspace id another Workspace has inserts nothing: Creation.CreateIn draws again, and a generator that
// only draws taken ids is a fault after a few draws, the transaction it ran in still usable.
func TestCreationGivesUpOnAGeneratorThatOnlyDrawsTakenIDs(t *testing.T) {
	env := testhelper.Setup(t)
	draws := 0
	taken := func() string { draws++; return fixtures.AcmePublicID }
	var err error

	txErr := env.Bus.WithinTx(t.Context(), func(tx *ent.Client, _ events.Publisher) error {
		_, err = workspaces.NewCreation(taken).CreateIn(t.Context(), tx, "Fresh", nil)
		return nil
	})

	require.NoError(t, txErr)
	require.ErrorContains(t, err, "every workspace id drawn is taken")
	assert.Equal(t, 3, draws, "it draws a few ids, then gives up")
}

func TestCreationKeepsTheFirstIDThatIsFree(t *testing.T) {
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
		ws, err = workspaces.NewCreation(next).CreateIn(t.Context(), tx, "Fresh", nil)
		return err
	})

	require.NoError(t, err)
	assert.Equal(t, "FreshId1", ws.PublicID)
}

// A new Workspace takes the next service feed position as it is created (ADR 0062), held in its
// standing beside it (ADR 0168), which the creation makes.
func TestCreationGivesTheWorkspaceItsStandingWithAFeedPosition(t *testing.T) {
	env := testhelper.Setup(t)
	top, err := workspaces.NewFeed().TopPosition(t.Context(), env.DB)
	require.NoError(t, err)

	var ws *ent.Workspace
	err = env.Bus.WithinTx(t.Context(), func(tx *ent.Client, _ events.Publisher) error {
		var err error
		ws, err = workspaces.NewCreation(func() string { return "FreshId1" }).CreateIn(t.Context(), tx, "Fresh", nil)
		return err
	})

	require.NoError(t, err)
	standing, err := env.DB.WorkspaceStanding.Query().Where(workspacestanding.WorkspaceID(ws.ID)).Only(t.Context())
	require.NoError(t, err)
	assert.Equal(t, top+1, standing.FeedPosition)
	assert.Nil(t, standing.SetupCompletedAt)
}

// On a new Instance no Workspace holds a feed position yet: the top one is 0, so the first Workspace
// takes 1.
func TestTheTopFeedPositionOfAnInstanceWithoutWorkspacesIsZero(t *testing.T) {
	sqlDB, err := sql.Open("pgx", testhelper.EmptyDBURL(t))
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Close(sqlDB, "empty instance") })
	_, err = db.Migrate(t.Context(), sqlDB)
	require.NoError(t, err)

	top, err := workspaces.NewFeed().TopPosition(t.Context(), entclient.Core.New(sqlDB))

	require.NoError(t, err)
	assert.Zero(t, top)
}
