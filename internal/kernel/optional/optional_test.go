package optional_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/kernel/optional"
)

func TestEqualTellsAnAbsentValueFromAZeroOne(t *testing.T) {
	zero := ""
	assert.False(t, optional.Equal(nil, &zero))
	assert.False(t, optional.Equal(&zero, nil))
}

func TestEqualComparesValuesNotPointers(t *testing.T) {
	a, b, c := 7, 7, 8
	assert.True(t, optional.Equal(&a, &b))
	assert.False(t, optional.Equal(&a, &c))
	assert.True(t, optional.Equal[int](nil, nil))
}
