package i18n_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/i18n"
)

func TestSupportedListsShippedLocalesAndReturnsACopy(t *testing.T) {
	got := i18n.Supported()
	assert.Contains(t, got, "en")

	got[0] = "xx"
	assert.NotContains(t, i18n.Supported(), "xx", "mutating the returned slice must not change the supported set")
	for _, l := range i18n.Supported() {
		assert.Equalf(t, l, i18n.Normalize(l), "Normalize(%q) = %q; every supported locale must be valid", l, i18n.Normalize(l))
	}
}
