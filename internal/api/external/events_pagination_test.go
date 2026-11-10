package external_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A page of the distinct actions holds the ones at its place in the ascending order.
func TestExternalEventActionsListPagesTheActionsAscending(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "events:read")
	ctx := context.Background()

	all, err := c.EventActionsList(ctx, externalapi.EventActionsListParams{PageSize: externalapi.NewOptInt32(100)})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(all.Items), 2)

	second, err := c.EventActionsList(ctx, externalapi.EventActionsListParams{
		Page: externalapi.NewOptInt32(2), PageSize: externalapi.NewOptInt32(1),
	})
	require.NoError(t, err)
	require.Len(t, second.Items, 1)
	assert.Equal(t, all.Items[1].Action, second.Items[0].Action)
	assert.EqualValues(t, len(all.Items), second.TotalItems)
	assert.EqualValues(t, len(all.Items), second.TotalPages)
}

// A page past the end is an empty page that still reports the true totals.
func TestExternalEventActionsListPageBeyondTheTotal(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "events:read")
	ctx := context.Background()

	first, err := c.EventActionsList(ctx, externalapi.EventActionsListParams{})
	require.NoError(t, err)
	all := first
	require.NotZero(t, all.TotalItems, "the fixtures log some actions")

	res, err := c.EventActionsList(ctx, externalapi.EventActionsListParams{Page: externalapi.NewOptInt32(500)})
	require.NoError(t, err)
	beyond := res
	assert.Empty(t, beyond.Items)
	assert.Equal(t, all.TotalItems, beyond.TotalItems)
	assert.EqualValues(t, 500, beyond.Page)
}
