package external_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A page past the end is an empty page that still reports the true totals.
func TestExternalEventActionsListPageBeyondTheTotal(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "events:read")
	ctx := context.Background()

	first, err := c.EventActionsList(ctx, externalapi.EventActionsListParams{})
	require.NoError(t, err)
	all := first.(*externalapi.EventActionsListOK)
	require.NotZero(t, all.TotalItems, "the fixtures log some actions")

	res, err := c.EventActionsList(ctx, externalapi.EventActionsListParams{Page: externalapi.NewOptInt32(500)})
	require.NoError(t, err)
	beyond, ok := res.(*externalapi.EventActionsListOK)
	require.Truef(t, ok, "got %T", res)
	assert.Empty(t, beyond.Items)
	assert.Equal(t, all.TotalItems, beyond.TotalItems)
	assert.EqualValues(t, 500, beyond.Page)
}
