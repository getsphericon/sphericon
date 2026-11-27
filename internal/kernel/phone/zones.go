package phone

import (
	"bufio"
	_ "embed"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/nyaruka/phonenumbers"
)

// zoneTab is IANA's zone.tab (public domain), copied as it is from the time zone database 2026f:
// each row names a zone and the one country it lies in. It is the one source of every zone of a
// country, which the phone number library does not give. Refresh it from a newer tzdata release
// (its zone.tab), never by hand.
//
//go:embed zone.tab
var zoneTab string

// countryZones is zoneTab read: the zones of each country, by its ISO 3166-1 alpha-2 code, in the
// order of the table.
var countryZones = sync.OnceValue(func() map[string][]string {
	out := map[string][]string{}
	rows := bufio.NewScanner(strings.NewReader(zoneTab))
	for rows.Scan() {
		row := rows.Text()
		if row == "" || strings.HasPrefix(row, "#") {
			continue
		}
		// country code, coordinates, zone, comments
		cols := strings.Split(row, "\t")
		if len(cols) >= 3 {
			out[cols[0]] = append(out[cols[0]], cols[2])
		}
	}
	return out
})

// Zones is the IANA time zones the E.164 number may be in (ADR 0134): those the phone number
// library maps the number's prefix to, or, when it maps the prefix to none, every zone of the
// number's country. The library maps none when it knows no zone of the number, and when it knows
// only its calling code: its answer is then every zone of each country sharing the code (all of
// +1's, from Guam to Bermuda, for a US toll-free number), not the number's own. A country the zone
// table does not list keeps the library's answer. A number of no country whose prefix maps none,
// or one that does not parse, is in none.
func Zones(number string) []string {
	n, err := phonenumbers.Parse(number, "")
	if err != nil {
		return nil
	}
	zones, err := phonenumbers.GetTimezonesForNumber(n)
	if err != nil {
		return nil
	}
	known := !slices.Equal(zones, []string{phonenumbers.UNKNOWN_TIMEZONE})
	if known && !callingCodeOnly(zones, n.GetCountryCode()) {
		return zones
	}
	if country, ok := countryZones()[Country(number)]; ok {
		return slices.Clone(country)
	}
	if known {
		return zones
	}
	return nil
}

// callingCodeOnly reports whether zones are those the library keeps for the calling code itself:
// what it answers for a number none of whose longer prefixes it maps.
func callingCodeOnly(zones []string, code int32) bool {
	byCode, err := phonenumbers.GetTimezonesForPrefix("+" + strconv.Itoa(int(code)))
	return err == nil && slices.Equal(zones, byCode)
}
