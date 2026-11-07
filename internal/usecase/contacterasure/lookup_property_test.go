package contacterasure_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/contacterasure"
)

// A Privacy request address is trimmed and lowercased, so any padding or case of a plain
// address finds what the address itself finds.
func TestLookupIgnoresPaddingAndCase(t *testing.T) {
	env := testhelper.Setup(t)
	erasure := contacterasure.New(env.Bus)
	acme := env.DB.Scoped(fixtures.AcmeID)
	want, err := erasure.Lookup(t.Context(), acme, fixtures.ContactAliceEmail)
	require.NoError(t, err)
	require.True(t, want.ContactHeld)

	rapid.Check(t, func(rt *rapid.T) {
		pad := rapid.StringMatching(`[ \t]{0,3}`).Draw(rt, "pad")
		upper := rapid.Bool().Draw(rt, "upper")
		raw := fixtures.ContactAliceEmail
		if upper {
			raw = strings.ToUpper(raw)
		}

		got, err := erasure.Lookup(t.Context(), acme, pad+raw+pad)

		if err != nil || got != want {
			rt.Fatalf("%q: got %+v, %v, want %+v", pad+raw+pad, got, err, want)
		}
	})
}

// Whatever comes in, a lookup either reads the address or refuses it as ErrInvalidEmail.
func TestLookupReadsAnAddressOrRefusesIt(t *testing.T) {
	env := testhelper.Setup(t)
	erasure := contacterasure.New(env.Bus)
	acme := env.DB.Scoped(fixtures.AcmeID)

	rapid.Check(t, func(rt *rapid.T) {
		raw := rapid.String().Draw(rt, "raw")

		_, err := erasure.Lookup(t.Context(), acme, raw)

		if err != nil && !errors.Is(err, contacterasure.ErrInvalidEmail) {
			rt.Fatalf("%q: unexpected error %v", raw, err)
		}
	})
}
