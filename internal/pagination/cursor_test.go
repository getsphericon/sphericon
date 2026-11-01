package pagination_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/pagination"
)

func TestCursorKeepsTheLimitAndReportsMore(t *testing.T) {
	got := pagination.Cursor([]int{4, 5, 6}, 2, "3", strconv.Itoa)

	assert.Equal(t, pagination.CursorPage[int]{Items: []int{4, 5}, NextCursor: "5", HasMore: true}, got)
}

func TestCursorOfTheLastRowsHasNoMore(t *testing.T) {
	got := pagination.Cursor([]int{4, 5}, 2, "3", strconv.Itoa)

	assert.Equal(t, pagination.CursorPage[int]{Items: []int{4, 5}, NextCursor: "5", HasMore: false}, got)
}

func TestCursorOfNoRowsKeepsTheRequestedCursor(t *testing.T) {
	got := pagination.Cursor[int](nil, 2, "3", strconv.Itoa)

	assert.Equal(t, pagination.CursorPage[int]{Items: []int{}, NextCursor: "3", HasMore: false}, got)
}

func TestMapCursorKeepsTheCursorAndMapsTheItems(t *testing.T) {
	page := pagination.CursorPage[string]{Items: []string{"a", "b"}, NextCursor: "b", HasMore: true}

	got := pagination.MapCursor(page, strings.ToUpper)

	assert.Equal(t, pagination.CursorPage[string]{Items: []string{"A", "B"}, NextCursor: "b", HasMore: true}, got)
}

func TestMapCursorOfAnEmptyPageHasNoNilItems(t *testing.T) {
	got := pagination.MapCursor(pagination.CursorPage[string]{NextCursor: "x"}, strings.ToUpper)

	assert.NotNil(t, got.Items, "an empty page serializes as [], not null")
	assert.Empty(t, got.Items)
}
