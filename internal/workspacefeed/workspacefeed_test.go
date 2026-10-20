package workspacefeed_test

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/workspacefeed"
	"github.com/getsphericon/sphericon/internal/workspaceid"
)

// A position taken outside a transaction would be a second statement: a crash between
// the two loses the change, and two of them can commit out of order. The plain client
// refuses such a change, so every path goes through a transaction.
func TestPlainClientRefusesAChangeToTheFeed(t *testing.T) {
	testhelper.Setup(t) // loads the schema into the real database once
	cfg, err := config.Load("test")
	require.NoError(t, err)
	pool, err := sql.Open("pgx", cfg.DatabaseURL)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pool.Close() })
	client := db.NewEntClient(pool)

	_, err = client.Workspace.Create().SetName("Plain").SetPublicID(workspaceid.New()).Save(t.Context())
	require.ErrorIs(t, err, workspacefeed.ErrNeedsTransaction)

	err = client.User.UpdateOneID(1).SetEmail("moved@plain.test").Exec(t.Context())
	require.ErrorIs(t, err, workspacefeed.ErrNeedsTransaction)
}
