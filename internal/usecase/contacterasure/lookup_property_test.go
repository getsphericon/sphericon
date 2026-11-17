package contacterasure_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/domain/contacts/unsubscribes"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/contacterasure"
)

// A Privacy request address is trimmed and lowercased, so any padding or case of a plain
// address finds what the address itself finds.
func TestLookupIgnoresPaddingAndCase(t *testing.T) {
	env := testhelper.Setup(t)
	erasure := contacterasure.New(env.Bus, testhelper.Module[*broadcasts.Module](env), testhelper.Module[*unsubscribes.Module](env), time.Now)
	acme := env.DB.Scoped(fixtures.AcmeID)
	want, err := erasure.Lookup(t.Context(), acme, fixtures.ContactAliceEmail)
	require.NoError(t, err)
	require.True(t, want.ContactHeld)

	rapid.Check(t, func(rt *rapid.T) {
		pad := rapid.StringMatching(`[ \t]{0,3}`).Draw(rt, "pad")
		raw := rapid.SampledFrom([]string{fixtures.ContactAliceEmail, strings.ToUpper(fixtures.ContactAliceEmail)}).Draw(rt, "case")

		got, err := erasure.Lookup(t.Context(), acme, pad+raw+pad)

		require.NoErrorf(rt, err, "%q", pad+raw+pad)
		require.Equalf(rt, want, got, "%q", pad+raw+pad)
	})
}

// Whatever comes in, a lookup either reads the address or refuses it as ErrInvalidEmail.
func TestLookupReadsAnAddressOrRefusesIt(t *testing.T) {
	env := testhelper.Setup(t)
	erasure := contacterasure.New(env.Bus, testhelper.Module[*broadcasts.Module](env), testhelper.Module[*unsubscribes.Module](env), time.Now)
	acme := env.DB.Scoped(fixtures.AcmeID)

	rapid.Check(t, func(rt *rapid.T) {
		raw := rapid.String().Draw(rt, "raw")

		_, err := erasure.Lookup(t.Context(), acme, raw)

		readOrRefused := err == nil || errors.Is(err, contacterasure.ErrInvalidEmail)
		require.Truef(rt, readOrRefused, "%q: unexpected error %v", raw, err)
	})
}
