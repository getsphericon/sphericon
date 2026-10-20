package pagination_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/pagination"
)

type opt struct {
	v  int32
	ok bool
}

func (o opt) Get() (int32, bool) { return o.v, o.ok }

// catalogue serves a fixed set of numbers through the two closures List takes.
func catalogue(n int) (func(context.Context) (int, error), func(context.Context, int, int) ([]int, error)) {
	count := func(context.Context) (int, error) { return n, nil }
	fetch := func(_ context.Context, limit, offset int) ([]int, error) {
		out := []int{}
		for i := offset; i < min(offset+limit, n); i++ {
			out = append(out, i)
		}
		return out, nil
	}
	return count, fetch
}

func TestParamsNormalizesOptionalPageParams(t *testing.T) {
	assert.Equal(t, pagination.Params{Page: 1, PageSize: 25}, pagination.ParamsOf(opt{}, opt{}))
	assert.Equal(t, pagination.Params{Page: 3, PageSize: 10}, pagination.ParamsOf(opt{3, true}, opt{10, true}))
	assert.Equal(t, pagination.Params{Page: 1, PageSize: 25}, pagination.ParamsOf(opt{-2, true}, opt{0, true}))
}

func TestParamsCapsThePageSize(t *testing.T) {
	assert.Equal(t, pagination.Params{Page: 1, PageSize: pagination.MaxPageSize}, pagination.ParamsOf(opt{}, opt{100000, true}))
	assert.Equal(t, pagination.MaxPageSize, pagination.ParamsOf(opt{}, opt{int32(pagination.MaxPageSize), true}).PageSize)
}

func TestListReturnsTheRequestedPageWithItsEnvelope(t *testing.T) {
	count, fetch := catalogue(5)
	p, err := pagination.List(context.Background(), pagination.Params{Page: 2, PageSize: 2}, count, fetch)
	require.NoError(t, err)
	assert.Equal(t, []int{2, 3}, p.Items)
	assert.Equal(t, 2, p.Page)
	assert.Equal(t, 2, p.PageSize)
	assert.Equal(t, 5, p.TotalItems)
	assert.Equal(t, 3, p.TotalPages)
}

func TestListBeyondTheLastPageIsEmptyAndSerializesAsArray(t *testing.T) {
	count, fetch := catalogue(5)
	p, err := pagination.List(context.Background(), pagination.Params{Page: 9, PageSize: 2}, count, fetch)
	require.NoError(t, err)
	assert.Empty(t, p.Items)
	assert.Equal(t, 5, p.TotalItems)
	body, err := json.Marshal(p)
	require.NoError(t, err)
	assert.Contains(t, string(body), `"items":[]`)
}

func TestListNilFetchResultSerializesAsArray(t *testing.T) {
	p, err := pagination.List(context.Background(), pagination.Params{Page: 1, PageSize: 2},
		func(context.Context) (int, error) { return 0, nil },
		func(context.Context, int, int) ([]int, error) { return nil, nil })
	require.NoError(t, err)
	body, err := json.Marshal(p)
	require.NoError(t, err)
	assert.Contains(t, string(body), `"items":[]`)
	assert.Equal(t, 0, p.TotalPages)
}

func TestListReportsCountAndFetchErrors(t *testing.T) {
	_, fetch := catalogue(1)
	_, err := pagination.List(context.Background(), pagination.Params{Page: 1, PageSize: 1},
		func(context.Context) (int, error) { return 0, assert.AnError }, fetch)
	require.ErrorIs(t, err, assert.AnError)

	count, _ := catalogue(1)
	_, err = pagination.List(context.Background(), pagination.Params{Page: 1, PageSize: 1}, count,
		func(context.Context, int, int) ([]int, error) { return nil, assert.AnError })
	assert.ErrorIs(t, err, assert.AnError)
}
