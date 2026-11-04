package sendingdomains_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/sendingdomains"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestListPagesTheWorkspaceSendingDomainsAscendingByID(t *testing.T) {
	env := testhelper.Setup(t)
	m := new(sendingdomains.Module)

	all, err := m.List(context.Background(), env.DB.Scoped(fixtures.AcmeID), pagination.Params{Page: 1, PageSize: 100})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(all.Items), 2)
	for i := 1; i < len(all.Items); i++ {
		assert.Less(t, all.Items[i-1].ID, all.Items[i].ID)
	}

	other, err := m.List(context.Background(), env.DB.Scoped(fixtures.GlobexID), pagination.Params{Page: 1, PageSize: 100})
	require.NoError(t, err)
	require.Len(t, other.Items, 1)
	assert.Equal(t, int64(fixtures.SendingDomainGlobexID), other.Items[0].ID)
}
