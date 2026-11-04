// Package phone reads a phone number as the E.164 form a Contact's phone is stored in (ADR 0075):
// one person, one key, so one consent and one opt-out. It is pure: every door that sets a phone
// (the contacts module, its migration) calls Normalize.
package phone

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/nyaruka/phonenumbers"
)

var (
	// ErrInvalid is a value that is no phone number of any country.
	ErrInvalid = errors.New("phone: not a phone number")
	// ErrCountryRequired is a number without a country code read with no (known) country.
	ErrCountryRequired = errors.New("phone: a number without a country code needs a country")
	// ErrExtension is a number with an extension: E.164 has none, and a text cannot reach one.
	ErrExtension = errors.New("phone: a number with an extension")
)

// internationalPrefix is the ITU's international call prefix, used in most countries; a number
// written with it ("0049 30 …") is read as international whatever the Contact's country.
const internationalPrefix = "00"

// Normalize returns raw in E.164 (`+14155550100`). A number without a country code is read with
// country, an ISO 3166-1 alpha-2 code ("" when there is none). It refuses with ErrCountryRequired
// a number that needs a country it lacks, with ErrExtension one with an extension, and with
// ErrInvalid anything else that is not a valid number of its country.
func Normalize(raw, country string) (string, error) {
	raw = strings.TrimSpace(raw)
	region := strings.ToUpper(strings.TrimSpace(country))
	n, err := parse(raw, region)
	if err != nil || !phonenumbers.IsValidNumber(n) {
		if rest, ok := strings.CutPrefix(raw, internationalPrefix); ok {
			n, err = parse("+"+rest, "")
		}
	}
	switch {
	case errors.Is(err, phonenumbers.ErrInvalidCountryCode):
		return "", ErrCountryRequired
	case err != nil, !phonenumbers.IsValidNumber(n):
		return "", ErrInvalid
	case n.GetExtension() != "":
		return "", ErrExtension
	}
	return phonenumbers.Format(n, phonenumbers.E164), nil
}

// Hash is the form a number takes on the opt-outs and Suppressions that outlive the Contact whose
// phone it was (ADR 0021): the lowercase hex SHA-256 of the E.164 number, the value the Erasure
// record keeps as the phone's hash. The eligibility layers on sms match a stored number by it too,
// so the refusal holds when the person comes back with the number. A phone number is guessable, so
// the hash keeps the number out of the rows, not out of reach.
func Hash(number string) string {
	sum := sha256.Sum256([]byte(number))
	return hex.EncodeToString(sum[:])
}

func parse(raw, region string) (*phonenumbers.PhoneNumber, error) {
	if raw == "" {
		return nil, ErrInvalid
	}
	return phonenumbers.Parse(raw, region)
}
