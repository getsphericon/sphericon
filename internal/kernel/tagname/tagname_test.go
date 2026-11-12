package tagname_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/kernel/tagname"
)

func TestNamesDifferingInCaseOrSurroundingSpaceAreOneTag(t *testing.T) {
	assert.True(t, tagname.Same("VIP", " vip "))
	assert.True(t, tagname.Same("Café", "CAFÉ"))
	assert.False(t, tagname.Same("vip", "vip2"))
	assert.Equal(t, "vip list", tagname.Key("  VIP List "))
}
