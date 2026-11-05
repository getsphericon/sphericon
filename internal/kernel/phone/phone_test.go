package phone_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/kernel/phone"
)

func TestNormalizeReadsANumberAsE164(t *testing.T) {
	cases := []struct {
		name, raw, country, want string
	}{
		{"international with punctuation", "+1 (415) 555-0100", "", "+14155550100"},
		{"already E.164", "+14155550100", "", "+14155550100"},
		{"international with a country that is not its own", "+44 20 7946 0958", "US", "+442079460958"},
		{"00 prefix without a country", "0049 30 1234567", "", "+49301234567"},
		{"00 prefix in a country whose prefix is another", "0049 30 1234567", "US", "+49301234567"},
		{"00 prefix in a country whose prefix it is", "0049 30 1234567", "DE", "+49301234567"},
		{"local with its trunk prefix", "030 1234567", "DE", "+49301234567"},
		{"local in the US", "(415) 555-0100", "US", "+14155550100"},
		{"local with a lowercase country", "8 999 123-45-67", "ru", "+79991234567"},
		{"a national (0) inside an international number", "+49 (0) 30 1234567", "", "+49301234567"},
		{"tel URI", "tel:+14155550100", "", "+14155550100"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := phone.Normalize(c.raw, c.country)
			require.NoError(t, err)
			assert.Equal(t, c.want, got)
		})
	}
}

func TestNormalizeRefusesWhatIsNoNumberATextCanReach(t *testing.T) {
	cases := []struct {
		name, raw, country string
		want               error
	}{
		{"local without a country", "415 555 0100", "", phone.ErrCountryRequired},
		{"local with an unknown country", "415 555 0100", "XX", phone.ErrCountryRequired},
		{"letters", "call me", "", phone.ErrInvalid},
		{"letters with a country", "call me", "US", phone.ErrInvalid},
		{"a plus alone", "+", "", phone.ErrInvalid},
		{"too short", "+15550001", "", phone.ErrInvalid},
		{"no such area code", "+1 555 555 5555", "", phone.ErrInvalid},
		{"too long", "+1 415 555 01001234", "", phone.ErrInvalid},
		{"local too short for its country", "1234567", "GB", phone.ErrInvalid},
		{"with an extension", "+1 415 555 0100 ext. 12", "", phone.ErrExtension},
		{"with an RFC 3966 extension", "+1-415-555-0100;ext=5", "", phone.ErrExtension},
		{"blank", "  ", "", phone.ErrInvalid},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := phone.Normalize(c.raw, c.country)
			assert.ErrorIs(t, err, c.want)
		})
	}
}

func TestNormalizeOfANormalizedNumberIsItself(t *testing.T) {
	for _, raw := range []string{"+14155550100", "+49301234567", "+79991234567", "+442079460958"} {
		got, err := phone.Normalize(raw, "FR")
		require.NoError(t, err)
		assert.Equal(t, raw, got)
	}
}
