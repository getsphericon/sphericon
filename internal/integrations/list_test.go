package integrations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/integrations"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestListPagesTheWorkspaceIntegrationsAscendingByID(t *testing.T) {
	env := testhelper.Setup(t)
	m := new(integrations.Module)

	first, err := m.List(context.Background(), env.DB.Scoped(fixtures.AcmeID), pagination.Params{Page: 1, PageSize: 1})
	require.NoError(t, err)
	require.Len(t, first.Items, 1)
	assert.Equal(t, int64(fixtures.IntegrationAcmeDefaultID), first.Items[0].ID)
	total, err := env.DB.Scoped(fixtures.AcmeID).Integration().Query().Count(context.Background())
	require.NoError(t, err)
	assert.Equal(t, total, first.TotalItems)
	assert.Equal(t, total, first.TotalPages, "one Integration a page")

	second, err := m.List(context.Background(), env.DB.Scoped(fixtures.AcmeID), pagination.Params{Page: 2, PageSize: 1})
	require.NoError(t, err)
	require.Len(t, second.Items, 1)
	assert.Equal(t, int64(fixtures.IntegrationAcmeSesID), second.Items[0].ID)
}
