package i18n

import (
	"slices"
	"testing"
)

func TestSupportedListsShippedLocalesAndReturnsACopy(t *testing.T) {
	got := Supported()
	for _, want := range []string{"en"} {
		if !slices.Contains(got, want) {
			t.Errorf("Supported() = %v, missing %q", got, want)
		}
	}

	got[0] = "xx"
	if slices.Contains(Supported(), "xx") {
		t.Fatal("mutating the returned slice must not change the supported set")
	}
	for _, l := range Supported() {
		if Normalize(l) != l {
			t.Errorf("Normalize(%q) = %q; every supported locale must be valid", l, Normalize(l))
		}
	}
}
