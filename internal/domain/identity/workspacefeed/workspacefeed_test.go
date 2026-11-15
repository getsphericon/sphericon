package workspacefeed_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/workspacefeed"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/workspaceid"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A position taken outside a transaction would be a second statement: a crash between
// the two loses the change, and two of them can commit out of order. The plain client
// refuses such a change, so every path goes through a transaction.
func TestPlainClientRefusesAChangeToTheFeed(t *testing.T) {
	// env.DB is the plain client, as production builds it: it carries the Guards.
	client := testhelper.Setup(t).DB

	// test writes: the hook fires on this write
	_, err := client.Workspace.Create().SetName("Plain").SetPublicID(workspaceid.New()).Save(t.Context())
	require.ErrorIs(t, err, workspacefeed.ErrNeedsTransaction)

	err = client.User.UpdateOneID(fixtures.OwnerJohnID).SetEmail("moved@plain.test").Exec(t.Context())
	require.ErrorIs(t, err, workspacefeed.ErrNeedsTransaction)
}
