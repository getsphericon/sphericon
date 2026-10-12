package pagination_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/pagination"
)

func TestTotalPages(t *testing.T) {
	assert.Equal(t, 0, pagination.TotalPages(10, 0), "no page size means no pages, not a division by zero")
	assert.Equal(t, 0, pagination.TotalPages(0, 10))
	assert.Equal(t, 1, pagination.TotalPages(10, 10))
	assert.Equal(t, 2, pagination.TotalPages(11, 10), "a partial last page counts")
	assert.Equal(t, 3, pagination.TotalPages(30, 10))
}
