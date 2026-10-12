package contacts_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/contacts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestListPagesTheWorkspaceContactsAscendingByID(t *testing.T) {
	env := testhelper.Setup(t)
	m := contacts.New(env.Bus)
	s := env.DB.Scoped(fixtures.AcmeID)

	all, err := m.List(context.Background(), s, pagination.Params{Page: 1, PageSize: 100})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(all.Items), 2)
	for i := 1; i < len(all.Items); i++ {
		assert.Less(t, all.Items[i-1].ID, all.Items[i].ID)
	}

	second, err := m.List(context.Background(), s, pagination.Params{Page: 2, PageSize: 1})
	require.NoError(t, err)
	require.Len(t, second.Items, 1)
	assert.Equal(t, all.Items[1].ID, second.Items[0].ID)
	assert.Equal(t, all.TotalItems, second.TotalItems)
	assert.Equal(t, all.TotalItems, second.TotalPages)
}
