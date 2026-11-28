package pagination_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/kernel/pagination"
)

func idOf(v int64) int64 { return v }

func TestByIDKeepsTheLimitAndPointsAtTheLastKeptRow(t *testing.T) {
	got := pagination.ByID([]int64{9, 8, 7}, 2, idOf)

	assert.Equal(t, pagination.IDPage[int64]{Items: []int64{9, 8}, Next: 8}, got)
}

func TestByIDOfTheLastRowsHasNoNext(t *testing.T) {
	got := pagination.ByID([]int64{9, 8}, 2, idOf)

	assert.Equal(t, pagination.IDPage[int64]{Items: []int64{9, 8}, Next: 0}, got)
}

func TestByIDOfNoRowsHasNoNilItems(t *testing.T) {
	got := pagination.ByID[int64](nil, 2, idOf)

	assert.NotNil(t, got.Items, "an empty page serializes as [], not null")
	assert.Equal(t, int64(0), got.Next)
}

// An id cursor a page hands out reads back as its id; none is the first page; anything else is no
// cursor a page handed out.
func TestAnIDCursorReadsBackAsTheIDItWasGivenFor(t *testing.T) {
	after, ok := pagination.ParseIDCursor(pagination.IDCursor(42))
	first, firstOK := pagination.ParseIDCursor("")
	_, garbled := pagination.ParseIDCursor("not a cursor!")
	_, zero := pagination.ParseIDCursor(pagination.IDCursor(0))

	assert.Equal(t, int64(42), after)
	assert.True(t, ok)
	assert.Zero(t, first)
	assert.True(t, firstOK)
	assert.False(t, garbled)
	assert.False(t, zero)
}
