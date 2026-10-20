package i18n

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSupportedListsShippedLocalesAndReturnsACopy(t *testing.T) {
	got := Supported()
	assert.Contains(t, got, "en")

	got[0] = "xx"
	assert.NotContains(t, Supported(), "xx", "mutating the returned slice must not change the supported set")
	for _, l := range Supported() {
		assert.Equalf(t, l, Normalize(l), "Normalize(%q) = %q; every supported locale must be valid", l, Normalize(l))
	}
}
