package contacts

import (
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/language"

	"github.com/getsphericon/sphericon/internal/kernel/bounds"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

var countryCodePattern = regexp.MustCompile(bounds.CountryCodePattern)

// reservedKeys are the keys that name an alias key or a core attribute of a Contact, the
// contract's camelCase names (ADR 0072): a trait of one is no Custom field, and no Custom field
// takes one.
var reservedKeys = map[string]bool{
	"subjectId": true, "email": true, "phone": true,
	"firstName": true, "lastName": true, "timeZone": true, "country": true,
	"locale": true, "birthday": true, "city": true, "avatar": true,
}

// textTrait reads one text core attribute from a trait: where it goes in a Core, and the value a
// trait's string stands for (false when it is no valid value of the attribute).
type textTrait struct {
	field func(*Core) *patch.Field[string]
	value func(string) (string, bool)
}

var textTraits = map[string]textTrait{
	"firstName": {func(c *Core) *patch.Field[string] { return &c.FirstName }, text},
	"lastName":  {func(c *Core) *patch.Field[string] { return &c.LastName }, text},
	"city":      {func(c *Core) *patch.Field[string] { return &c.City }, text},
	"timeZone":  {func(c *Core) *patch.Field[string] { return &c.TimeZone }, timeZoneName},
	"country":   {func(c *Core) *patch.Field[string] { return &c.Country }, countryCode},
	"locale":    {func(c *Core) *patch.Field[string] { return &c.Locale }, localeTag},
	"avatar":    {func(c *Core) *patch.Field[string] { return &c.Avatar }, avatarURL},
}

// FromTraits splits the traits of an Identify into the core attributes they assert and the Custom
// fields (ADR 0072). A null trait clears its attribute, a value that is no valid value of it is
// dropped, and a trait of an alias key is dropped too: Identify carries those itself.
func FromTraits(traits map[string]any) (core Core, fields map[string]any) {
	fields = make(map[string]any, len(traits))
	for key, value := range traits {
		switch {
		case !reservedKeys[key]:
			fields[key] = value
		case key == "birthday":
			core.Birthday = traitOf(value, birthday)
		default:
			if t, ok := textTraits[key]; ok {
				*t.field(&core) = traitOf(value, t.value)
			}
		}
	}
	return core, fields
}

// traitOf is the patch a trait makes: null clears, a string read as a valid value sets, anything
// else keeps.
func traitOf[T any](value any, read func(string) (T, bool)) patch.Field[T] {
	if value == nil {
		return patch.Clear[T]()
	}
	s, ok := value.(string)
	if !ok {
		return patch.Keep[T]()
	}
	v, ok := read(s)
	if !ok {
		return patch.Keep[T]()
	}
	return patch.Set(v)
}

// guess is a browser's guess read as a valid value, nil when it is none.
func guess(s *string, read func(string) (string, bool)) *string {
	if s == nil {
		return nil
	}
	if v, ok := read(*s); ok {
		return &v
	}
	return nil
}

func (c Core) normalized() Core {
	c.Country = patch.Map(c.Country, func(s string) string { return strings.ToUpper(strings.TrimSpace(s)) })
	c.Locale = patch.Map(c.Locale, canonicalLocale)
	return c
}

// canonicalLocale is a BCP 47 tag in its canonical form (`pt-br` is `pt-BR`); a tag the language
// package cannot read stays as it was given.
func canonicalLocale(s string) string {
	tag, err := language.Parse(s)
	if err != nil {
		return s
	}
	return tag.String()
}

func text(s string) (string, bool) {
	s = strings.TrimSpace(s)
	// Identify's traits are an untyped object in the collect contract, so a core attribute read
	// from one is held to the bound the contacts contract declares for a text attribute.
	return s, s != "" && utf8.RuneCountInString(s) <= bounds.ContactCityMaxLength
}

func timeZoneName(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" || s == "Local" {
		return "", false
	}
	_, err := time.LoadLocation(s)
	return s, err == nil
}

func countryCode(s string) (string, bool) {
	s = strings.ToUpper(strings.TrimSpace(s))
	return s, countryCodePattern.MatchString(s)
}

func localeTag(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" || utf8.RuneCountInString(s) > bounds.LocaleTagMaxLength {
		return "", false
	}
	tag, err := language.Parse(s)
	if err != nil {
		return "", false
	}
	// The canonical form is held to the contract's pattern too, as the Contact's write is
	// (checked), so a trait the write would refuse is dropped instead.
	canonical := tag.String()
	return canonical, validation.Pattern("locale", canonical, bounds.LocaleTagPattern) == nil
}

func avatarURL(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > bounds.ImageURLMaxLength || strings.ContainsAny(s, " \t\r\n") {
		return "", false
	}
	u, err := url.Parse(s)
	return s, err == nil && u.Scheme == "https" && u.Host != "" &&
		validation.Pattern("avatar", s, bounds.ImageURLPattern) == nil
}

func birthday(s string) (time.Time, bool) {
	d, err := time.Parse(time.DateOnly, strings.TrimSpace(s))
	return d, err == nil
}
