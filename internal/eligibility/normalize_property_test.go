package eligibility_test

import (
	"strings"
	"testing"
	"unicode"

	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/eligibility"
)

// Every write and lookup routes a destination through NormalizeDestination, so normalizing a
// normalized value must change nothing, or a stored key would not match its own lookup.
func TestNormalizeDestinationIsIdempotent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		dest := rapid.String().Draw(t, "dest")

		once := eligibility.NormalizeDestination(dest)
		twice := eligibility.NormalizeDestination(once)

		if once != twice {
			t.Fatalf("%q normalizes to %q, then to %q", dest, once, twice)
		}
	})
}

// The normalized form carries no surrounding space and no letter that has a lower-case form left,
// so the point lookup and the lower()-folding batch SQL agree on it.
func TestNormalizeDestinationIsTrimmedAndLowerCased(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		dest := rapid.String().Draw(t, "dest")

		got := eligibility.NormalizeDestination(dest)

		if got != strings.TrimSpace(got) {
			t.Fatalf("%q keeps surrounding space", got)
		}
		for _, r := range got {
			if unicode.ToLower(r) != r {
				t.Fatalf("%q keeps %q, which still has a lower-case form", got, r)
			}
		}
	})
}
