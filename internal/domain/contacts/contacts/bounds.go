package contacts

import (
	"github.com/getsphericon/sphericon/internal/kernel/bounds"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// checked refuses a Contact's address and the bounded core attributes a write sets, whoever made
// the write (ADR 0107): the address must be one bare address of the contract's length
// (validation.Email), the city, the locale and the avatar within the contract's lengths, and the
// locale and the avatar of the contract's patterns. A nil value is not set by the write. The
// address comes back in its Email case.
func checked(email, city, country, locale, avatar *string) (*string, error) {
	if email != nil {
		address, err := validation.Email(FieldEmail, *email)
		if err != nil {
			return nil, err
		}
		email = &address
	}
	for _, f := range []struct {
		field, pattern string
		value          *string
		max            int
	}{
		{"city", "", city, bounds.ContactCityMaxLength},
		{"country", bounds.CountryCodePattern, country, len("US")},
		{"locale", bounds.LocaleTagPattern, locale, bounds.LocaleTagMaxLength},
		{"avatar", bounds.ImageURLPattern, avatar, bounds.ImageURLMaxLength},
	} {
		if f.value == nil {
			continue
		}
		if err := validation.MaxLength(f.field, *f.value, f.max); err != nil {
			return nil, err
		}
		if f.pattern == "" {
			continue
		}
		if err := validation.Pattern(f.field, *f.value, f.pattern); err != nil {
			return nil, err
		}
	}
	return email, nil
}
