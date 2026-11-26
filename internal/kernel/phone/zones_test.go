package phone_test

import (
	"testing"
	"time"

	"github.com/nyaruka/phonenumbers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/kernel/phone"
)

// The zones a number may be in (ADR 0134): those the phone number library maps its prefix to, and
// every zone of its country when it maps the prefix to none.

func TestANumberIsInTheZonesItsPrefixMapsTo(t *testing.T) {
	cases := []struct {
		name, number string
		want         []string
	}{
		{"a Boston number", "+16175557740", []string{"America/New_York"}},
		{"a Honolulu number", "+18085550100", []string{"Pacific/Honolulu"}},
		{"an Alaska number, a prefix of two zones", "+19075550100", []string{"America/Adak", "America/Anchorage"}},
		{"a London number", "+442071234567", []string{"Europe/London"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, phone.Zones(c.number))
		})
	}
}

// A US toll-free number has no zone of its own prefix: the library falls back to every zone of
// the calling code +1, from Guam to Bermuda. The number's country is the United States, so it is
// in the United States' zones, and in none of the other countries that share +1.
func TestANumberWhosePrefixMapsNoZoneIsInEveryZoneOfItsCountry(t *testing.T) {
	zones := phone.Zones("+18005550100")

	for _, us := range []string{"America/New_York", "America/Chicago", "America/Denver", "America/Los_Angeles",
		"America/Anchorage", "America/Adak", "Pacific/Honolulu"} {
		assert.Contains(t, zones, us)
	}
	for _, other := range []string{"America/Toronto", "America/Puerto_Rico", "Pacific/Guam", "Atlantic/Bermuda"} {
		assert.NotContains(t, zones, other)
	}
}

// The library keeps +49 at the calling code, Berlin alone: that is no zone of the number's own
// prefix, so the number is in every zone of Germany.
func TestANumberMappedOnlyByItsCallingCodeIsInEveryZoneOfItsCountry(t *testing.T) {
	assert.Equal(t, []string{"Europe/Berlin", "Europe/Busingen"}, phone.Zones("+4930123456"))
}

// Kosovo is a country of the library that the zone table does not list: its numbers keep the
// zones the library gives them.
func TestANumberOfACountryTheZoneTableLacksKeepsTheLibrarysZones(t *testing.T) {
	assert.Equal(t, []string{"Europe/Belgrade"}, phone.Zones("+38344123456"))
}

// A global service number has no country and the library knows no zone of it.
func TestANumberOfNoCountryIsInNoZone(t *testing.T) {
	assert.Empty(t, phone.Zones("+80012345678"))
	assert.Empty(t, phone.Zones("not a number"))
}

// Every country the library knows has zones for its numbers, and the zone database loads each:
// a number is never judged in a zone the Instance cannot read.
func TestEveryCountrysNumbersAreInZonesThatLoad(t *testing.T) {
	numbers := exampleNumbers()
	require.NotEmpty(t, numbers)
	for _, number := range numbers {
		zones := phone.Zones(number)
		assert.NotEmpty(t, zones, number)
		for _, zone := range zones {
			_, err := time.LoadLocation(zone)
			require.NoError(t, err, number)
		}
	}
}

// exampleNumbers is the library's example fixed-line and mobile number of every country it knows,
// in E.164, each that it has.
func exampleNumbers() []string {
	var out []string
	for region := range phonenumbers.GetSupportedRegions() {
		for _, kind := range []phonenumbers.PhoneNumberType{phonenumbers.FIXED_LINE, phonenumbers.MOBILE} {
			if n := phonenumbers.GetExampleNumberForType(region, kind); n != nil {
				out = append(out, phonenumbers.Format(n, phonenumbers.E164))
			}
		}
	}
	return out
}
