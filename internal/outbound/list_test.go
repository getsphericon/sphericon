package outbound_test

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestListTransactionalPagesOnlyTransactionalMessagesNewestFirst(t *testing.T) {
	env := testhelper.Setup(t)
	m := newModule(env)
	s := env.DB.Scoped(fixtures.AcmeID)

	all, err := m.ListTransactional(context.Background(), s, pagination.Params{Page: 1, PageSize: 100})
	require.NoError(t, err)
	require.Greater(t, len(all.Items), 2)
	kinds := make([]outboundmessage.Kind, len(all.Items))
	ids := make([]int64, len(all.Items))
	for i, msg := range all.Items {
		kinds[i] = msg.Kind
		ids[i] = msg.ID
	}
	assert.Len(t, all.Items, all.TotalItems)
	assert.Equal(t, []outboundmessage.Kind{outboundmessage.KindTransactional}, lo.Uniq(kinds))
	assert.IsDecreasing(t, ids)

	second, err := m.ListTransactional(context.Background(), s, pagination.Params{Page: 2, PageSize: 1})
	require.NoError(t, err)
	require.Len(t, second.Items, 1)
	assert.Equal(t, all.Items[1].ID, second.Items[0].ID)
	assert.Equal(t, all.TotalItems, second.TotalItems)
}
