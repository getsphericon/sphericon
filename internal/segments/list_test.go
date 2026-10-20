package segments_test

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/segments"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestListPagesTheWorkspaceSegmentsAscendingByID(t *testing.T) {
	env := testhelper.Setup(t)
	m := segments.New()

	all, err := m.List(context.Background(), env.DB.Scoped(fixtures.AcmeID), pagination.Params{Page: 1, PageSize: 100})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(all.Items), 2)
	ids := lo.Map(all.Items, func(sg *ent.Segment, _ int) int64 { return sg.ID })
	assert.IsIncreasing(t, ids)

	second, err := m.List(context.Background(), env.DB.Scoped(fixtures.AcmeID), pagination.Params{Page: 2, PageSize: 1})
	require.NoError(t, err)
	require.Len(t, second.Items, 1)
	assert.Equal(t, all.Items[1].ID, second.Items[0].ID)
	assert.Equal(t, all.TotalItems, second.TotalPages)

	other, err := m.List(context.Background(), env.DB.Scoped(fixtures.GlobexID), pagination.Params{Page: 1, PageSize: 100})
	require.NoError(t, err)
	require.Len(t, other.Items, 1)
	assert.Equal(t, int64(fixtures.SegmentGlobexID), other.Items[0].ID)
}
