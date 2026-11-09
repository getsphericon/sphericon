package pagination_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/kernel/pagination"
)

type opt struct {
	v  int32
	ok bool
}

func (o opt) Get() (int32, bool) { return o.v, o.ok }

// numbers is a query over 0..n-1 that behaves like an ent builder: Limit and Offset mutate it and
// return it, Clone copies it, and Count and All honour the limit and offset already set.
type numbers struct {
	n             int
	limit, offset *int
	countErr      error
	allErr        error
	none          bool
}

func (q *numbers) Clone() *numbers { c := *q; return &c }

func (q *numbers) Limit(n int) *numbers { q.limit = &n; return q }

func (q *numbers) Offset(n int) *numbers { q.offset = &n; return q }

func (q *numbers) rows() []int {
	out := []int{}
	for i := range q.n {
		out = append(out, i)
	}
	if q.offset != nil {
		out = out[min(*q.offset, len(out)):]
	}
	if q.limit != nil {
		out = out[:min(*q.limit, len(out))]
	}
	return out
}

func (q *numbers) Count(context.Context) (int, error) { return len(q.rows()), q.countErr }

func (q *numbers) All(context.Context) ([]int, error) {
	if q.none {
		return nil, q.allErr
	}
	return q.rows(), q.allErr
}

func TestParamsNormalizesOptionalPageParams(t *testing.T) {
	assert.Equal(t, pagination.Params{Page: 1, PageSize: 25}, pagination.ParamsOf(opt{}, opt{}))
	assert.Equal(t, pagination.Params{Page: 3, PageSize: 10}, pagination.ParamsOf(opt{3, true}, opt{10, true}))
	assert.Equal(t, pagination.Params{Page: 1, PageSize: 25}, pagination.ParamsOf(opt{-2, true}, opt{0, true}))
}

func TestListReturnsTheRequestedPageWithItsEnvelope(t *testing.T) {
	p, err := pagination.List(context.Background(), pagination.Params{Page: 2, PageSize: 2}, &numbers{n: 5})
	require.NoError(t, err)
	assert.Equal(t, []int{2, 3}, p.Items)
	assert.Equal(t, 2, p.Page)
	assert.Equal(t, 2, p.PageSize)
	assert.Equal(t, 5, p.TotalItems)
	assert.Equal(t, 3, p.TotalPages)
}

func TestListPagesOneQueryAgainAndAgain(t *testing.T) {
	q := &numbers{n: 5}
	_, err := pagination.List(context.Background(), pagination.Params{Page: 2, PageSize: 2}, q)
	require.NoError(t, err)

	p, err := pagination.List(context.Background(), pagination.Params{Page: 3, PageSize: 2}, q)
	require.NoError(t, err)
	assert.Equal(t, []int{4}, p.Items)
	assert.Equal(t, 5, p.TotalItems)
}

func TestListBeyondTheLastPageIsEmptyAndSerializesAsArray(t *testing.T) {
	p, err := pagination.List(context.Background(), pagination.Params{Page: 9, PageSize: 2}, &numbers{n: 5})
	require.NoError(t, err)
	assert.Empty(t, p.Items)
	assert.Equal(t, 5, p.TotalItems)
	body, err := json.Marshal(p)
	require.NoError(t, err)
	assert.Contains(t, string(body), `"items":[]`)
}

func TestListNilRowsSerializeAsArray(t *testing.T) {
	p, err := pagination.List(context.Background(), pagination.Params{Page: 1, PageSize: 2}, &numbers{none: true})
	require.NoError(t, err)
	body, err := json.Marshal(p)
	require.NoError(t, err)
	assert.Contains(t, string(body), `"items":[]`)
	assert.Equal(t, 0, p.TotalPages)
}

func TestListReportsCountAndFetchErrors(t *testing.T) {
	_, err := pagination.List(context.Background(), pagination.Params{Page: 1, PageSize: 1}, &numbers{n: 1, countErr: assert.AnError})
	require.ErrorIs(t, err, assert.AnError)

	_, err = pagination.List(context.Background(), pagination.Params{Page: 1, PageSize: 1}, &numbers{n: 1, allErr: assert.AnError})
	assert.ErrorIs(t, err, assert.AnError)
}
