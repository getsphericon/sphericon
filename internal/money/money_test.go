package money_test

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/money"
)

func TestADecimalBecomesMinorUnitsByTheCurrencysOwnExponent(t *testing.T) {
	for name, tc := range map[string]struct {
		amount   any
		currency string
		want     int64
	}{
		"two places":        {json.Number("19.99"), "USD", 1999},
		"fewer places":      {json.Number("20.5"), "USD", 2050},
		"whole number":      {json.Number("7"), "EUR", 700},
		"no places at all":  {json.Number("1500"), "JPY", 1500},
		"three places":      {json.Number("1.234"), "KWD", 1234},
		"decoded as float":  {19.99, "USD", 1999},
		"exponent notation": {json.Number("1e2"), "USD", 10000},
		"trailing zeros":    {json.Number("10.50"), "USD", 1050},
		"zero":              {json.Number("0"), "USD", 0},
	} {
		got, err := money.Minor(tc.amount, tc.currency)
		require.NoError(t, err, name)
		assert.Equal(t, tc.want, got, name)
	}
}

func TestAnAmountIsRefusedWithAReasonNeverRounded(t *testing.T) {
	for name, tc := range map[string]struct {
		amount   any
		currency string
		reason   string
	}{
		"too many places":         {json.Number("19.999"), "USD", "more than 2 decimal places"},
		"places for a whole-unit": {json.Number("10.5"), "JPY", "more than 0 decimal places"},
		"unknown currency":        {json.Number("1"), "QQQ", "unknown currency"},
		"lowercase code":          {json.Number("1"), "usd", "unknown currency"},
		"not a number":            {"12", "USD", "must be a number"},
		"a boolean":               {true, "USD", "must be a number"},
		"negative":                {json.Number("-1"), "USD", "must not be negative"},
		"too large":               {json.Number("10000000000000"), "USD", "too large"},
	} {
		_, err := money.Minor(tc.amount, tc.currency)
		require.ErrorContains(t, err, tc.reason, name)
	}
}

func TestEveryAmountWithinTheCurrencysPlacesSurvivesTheRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		code := rapid.SampledFrom([]string{"USD", "EUR", "JPY", "KWD", "CLP"}).Draw(t, "currency")
		exp, err := money.Exponent(code)
		require.NoError(t, err)
		minor := rapid.Int64Range(0, 999_999_999_999_999).Draw(t, "minor")

		text := decimal(minor, exp)
		got, err := money.Minor(json.Number(text), code)

		require.NoError(t, err)
		assert.Equal(t, minor, got)
	})
}

// decimal writes minor units as the plain decimal a shop sends, with exp places.
func decimal(minor int64, exp int) string {
	digits := strconv.FormatInt(minor, 10)
	if exp == 0 {
		return digits
	}
	digits = strings.Repeat("0", max(0, exp+1-len(digits))) + digits
	cut := len(digits) - exp
	return digits[:cut] + "." + digits[cut:]
}
