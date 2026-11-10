package validation

import (
	"regexp"
	"sync"
	"unicode/utf8"

	"github.com/getsphericon/sphericon/internal/kernel/i18n"
)

// The refusals of a value outside a bound of the contract (package bounds), each reported against
// the request field it names and worded with the bound.
var (
	ErrTooShort        = Refuse("validation: too short", i18n.ErrorsTooShort)
	ErrTooLong         = Refuse("validation: too long", i18n.ErrorsTooLong)
	ErrOutOfRange      = Refuse("validation: out of range", i18n.ErrorsOutOfRange)
	ErrTooMany         = Refuse("validation: too many items", i18n.ErrorsTooMany)
	ErrTooFew          = Refuse("validation: too few items", i18n.ErrorsTooFew)
	ErrPatternMismatch = Refuse("validation: does not match the pattern", i18n.ErrorsPatternMismatch)
)

// Length refuses value on field when it has fewer than lo or more than hi characters (the
// contract's minLength and maxLength count characters); lo 0 sets no lower bound.
func Length(field, value string, lo, hi int) error {
	n := utf8.RuneCountInString(value)
	if n < lo {
		return ErrTooShort.On(field).Args(map[string]any{"Field": field, "Min": lo})
	}
	if n > hi {
		return ErrTooLong.On(field).Args(map[string]any{"Field": field, "Max": hi})
	}
	return nil
}

// MaxLength is Length with no lower bound.
func MaxLength(field, value string, hi int) error { return Length(field, value, 0, hi) }

// Range refuses value on field when it is below lo or above hi.
func Range(field string, value, lo, hi int) error {
	if value < lo || value > hi {
		return ErrOutOfRange.On(field).Args(map[string]any{"Field": field, "Min": lo, "Max": hi})
	}
	return nil
}

// Items refuses a list of n items on field when it holds fewer than lo or more than hi; lo 0 sets
// no lower bound.
func Items(field string, n, lo, hi int) error {
	if n < lo {
		return ErrTooFew.On(field).Args(map[string]any{"Field": field, "Min": lo})
	}
	if n > hi {
		return ErrTooMany.On(field).Args(map[string]any{"Field": field, "Max": hi})
	}
	return nil
}

// patterns are the compiled patterns of the contract, by their source.
var patterns sync.Map

// Pattern refuses value on field when the contract's pattern does not match it.
func Pattern(field, value, pattern string) error {
	re, ok := patterns.Load(pattern)
	if !ok {
		re, _ = patterns.LoadOrStore(pattern, regexp.MustCompile(pattern))
	}
	if !re.(*regexp.Regexp).MatchString(value) {
		return ErrPatternMismatch.On(field).Args(map[string]any{"Field": field})
	}
	return nil
}
