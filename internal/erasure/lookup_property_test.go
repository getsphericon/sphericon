package erasure_test

import (
	"errors"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/erasure"
)

// A Privacy request address is trimmed and lowercased like ByEmail does, so any padding or
// case of a plain address finds the same person.
func TestNormalizeEmailIgnoresPaddingAndCase(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		local := rapid.StringMatching(`[a-z0-9]{1,12}`).Draw(t, "local")
		domain := rapid.StringMatching(`[a-z0-9]{1,12}\.[a-z]{2,6}`).Draw(t, "domain")
		pad := rapid.StringMatching(`[ \t]{0,3}`).Draw(t, "pad")
		plain := local + "@" + domain

		got, err := erasure.NormalizeEmail(pad + strings.ToUpper(plain) + pad)

		if err != nil || got != plain {
			t.Fatalf("got %q, %v, want %q", got, err, plain)
		}
	})
}

// Whatever comes in, an accepted address is already normalized, so normalizing it again
// changes nothing, and a rejected one is always ErrInvalidEmail.
func TestNormalizeEmailIsIdempotentOrRejected(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		raw := rapid.String().Draw(t, "raw")

		once, err := erasure.NormalizeEmail(raw)
		if err != nil {
			if !errors.Is(err, erasure.ErrInvalidEmail) {
				t.Fatalf("%q: unexpected error %v", raw, err)
			}
			return
		}
		twice, err := erasure.NormalizeEmail(once)

		if err != nil || once != twice {
			t.Fatalf("%q normalizes to %q, then to %q, %v", raw, once, twice, err)
		}
	})
}
