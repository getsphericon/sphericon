package pagination_test

import (
	"testing"

	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/stretchr/testify/assert"
)

func TestPaginateSlicesAnInMemoryCatalogue(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e"}
	i32 := func(v int32) *int32 { return &v }

	p := pagination.Paginate(items, i32(2), i32(2))
	assert.Equal(t, []string{"c", "d"}, p.Items)
	assert.Equal(t, 2, p.Page)
	assert.Equal(t, 2, p.PageSize)
	assert.Equal(t, 5, p.TotalItems)
	assert.Equal(t, 3, p.TotalPages)

	last := pagination.Paginate(items, i32(3), i32(2))
	assert.Equal(t, []string{"e"}, last.Items)

	beyond := pagination.Paginate(items, i32(9), i32(2))
	assert.Empty(t, beyond.Items)
	assert.NotNil(t, beyond.Items, "an empty page serializes as [], not null")

	def := pagination.Paginate(items, nil, nil)
	assert.Equal(t, items, def.Items)
	assert.Equal(t, 1, def.Page)
	assert.Equal(t, 25, def.PageSize)
}

func TestWithItemsKeepsThePageAndReplacesItsItems(t *testing.T) {
	page := pagination.Page[int]{Items: []int{1, 2}, Page: 2, PageSize: 2, TotalItems: 5, TotalPages: 3}

	got := pagination.WithItems(page, []string{"one", "two"})

	assert.Equal(t, pagination.Page[string]{Items: []string{"one", "two"}, Page: 2, PageSize: 2, TotalItems: 5, TotalPages: 3}, got)
}
