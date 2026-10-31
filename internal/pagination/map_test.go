package pagination_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapKeepsTheEnvelopeAndMapsTheItems(t *testing.T) {
	page := pagination.Page[string]{Items: []string{"a", "b"}, Page: 2, PageSize: 2, TotalItems: 5, TotalPages: 3}

	got := pagination.Map(page, strings.ToUpper)

	assert.Equal(t, pagination.Page[string]{Items: []string{"A", "B"}, Page: 2, PageSize: 2, TotalItems: 5, TotalPages: 3}, got)
}

func TestMapOfAnEmptyPageSerializesAsArray(t *testing.T) {
	got := pagination.Map(pagination.Page[string]{Page: 9, PageSize: 2, TotalItems: 5, TotalPages: 3}, strings.ToUpper)

	assert.NotNil(t, got.Items, "an empty page serializes as [], not null")
	assert.Empty(t, got.Items)
}

func TestMapErrKeepsTheEnvelopeAndMapsTheItems(t *testing.T) {
	page := pagination.Page[string]{Items: []string{"a", "b"}, Page: 1, PageSize: 2, TotalItems: 2, TotalPages: 1}

	got, err := pagination.MapErr(page, func(s string) (string, error) { return strings.ToUpper(s), nil })

	require.NoError(t, err)
	assert.Equal(t, pagination.Page[string]{Items: []string{"A", "B"}, Page: 1, PageSize: 2, TotalItems: 2, TotalPages: 1}, got)
}

func TestMapErrReturnsTheFirstError(t *testing.T) {
	boom := errors.New("boom")
	page := pagination.Page[string]{Items: []string{"a", "b"}, Page: 1, PageSize: 2, TotalItems: 2, TotalPages: 1}

	got, err := pagination.MapErr(page, func(string) (string, error) { return "", boom })

	require.ErrorIs(t, err, boom)
	assert.Zero(t, got)
}

func TestWholeIsTheOnePageOfAList(t *testing.T) {
	got := pagination.Whole([]string{"a", "b", "c"})

	assert.Equal(t, pagination.Page[string]{Items: []string{"a", "b", "c"}, Page: 1, PageSize: 3, TotalItems: 3, TotalPages: 1}, got)
}

func TestWholeOfNoItemsIsStillOnePage(t *testing.T) {
	got := pagination.Whole[string](nil)

	assert.Equal(t, pagination.Page[string]{Items: []string{}, Page: 1, PageSize: 0, TotalItems: 0, TotalPages: 1}, got)
}
