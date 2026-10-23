package validation_test

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/validation"
)

func TestAccountEmail(t *testing.T) {
	cases := map[string]bool{
		"grace@example.com":                      true,
		"grace.hopper+news@mail.example.co.uk":   true,
		"o'brien@example.com":                    true,
		"a_@example.com":                         true,
		"user@xn--e1afmkfd.xn--p1ai":             true,
		"user@sub-domain.example.com":            true,
		strings.Repeat("a", 64) + "@example.com": true,

		"fdsadsf@fdsa":                           false, // a dotless domain
		"admin@localhost":                        false,
		"a@b c":                                  false,
		"a@.":                                    false,
		"a@-":                                    false,
		"a@ex..com":                              false,
		"a@-ex.com":                              false,
		"a@ex-.com":                              false,
		"a@1.2.3.4":                              false, // an IP address
		"a@[::1]":                                false,
		"a@example.123":                          false,
		".a@example.com":                         false,
		"a.@example.com":                         false,
		"a..b@example.com":                       false,
		`"a b"@example.com`:                      false, // a quoted local part
		"a@b@example.com":                        false,
		"@example.com":                           false,
		"a@":                                     false,
		"plain":                                  false,
		"":                                       false,
		"\u00e9mile@example.com":                 false,
		"user@b\u00fccher.example":               false, // Unicode is accepted as punycode only
		strings.Repeat("a", 65) + "@example.com": false,
		"a@" + strings.Repeat("b", 64) + ".com":  false,
		"a@" + strings.Repeat("b.", 126) + "com": false, // longer than 254
	}
	for email, want := range cases {
		assert.Equalf(t, want, validation.AccountEmail(email), "%q", email)
	}
}

func TestPasswordLength(t *testing.T) {
	cases := map[string]bool{
		"12345678":                    true,
		"correct horse battery":       true, // spaces are allowed
		"p\u00e4ssw\u00f6rd phrase":   true, // so is any script
		"|'\"<>/\\ \t~":               true,
		strings.Repeat("\u00e9", 256): true, // characters, not bytes
		"1234567":                     false,
		"1":                           false,
		"":                            false,
		strings.Repeat("a", 257):      false,
		"1234567\xff":                 false, // not UTF-8
	}
	for password, want := range cases {
		assert.Equalf(t, want, validation.PasswordLength(password), "%q", password)
	}
}

func FuzzAccountEmail(f *testing.F) {
	for _, seed := range []string{"grace@example.com", "fdsadsf@fdsa", "a@b c", `"a"@example.com`, "user@xn--p1ai.xn--p1ai", ""} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, s string) {
		ok := validation.AccountEmail(s)
		local, domain, _ := strings.Cut(s, "@")
		require.Truef(t, !ok || len(s) <= 254, "%q: accepted past 254 octets", s)
		require.Truef(t, !ok || strings.Count(s, "@") == 1, "%q: accepted without exactly one @", s)
		require.Truef(t, !ok || local != "" && strings.Contains(domain, "."), "%q: accepted an empty local part or a dotless domain", s)
		require.Truef(t, !ok || strings.IndexFunc(s, func(r rune) bool { return r > 0x7e || r <= ' ' }) < 0, "%q: accepted a space, a control or a non-ASCII character", s)
	})
}

func FuzzPassword(f *testing.F) {
	for _, seed := range []string{"12345678", "1", "", "        ", "p\u00e4ssw\u00f6rd phrase", "\xff\xfe\xfd\xfc\xfb\xfa\xf9\xf8"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, s string) {
		n := utf8.RuneCountInString(s)
		want := utf8.ValidString(s) && n >= validation.PasswordMinLength && n <= validation.PasswordMaxLength
		require.Equalf(t, want, validation.PasswordLength(s), "%q", s)
		blank := strings.TrimFunc(s, unicode.IsSpace) == ""
		require.Equalf(t, !blank, validation.PasswordNotBlank(s), "%q", s)
	})
}

func TestPasswordNotBlank(t *testing.T) {
	cases := map[string]bool{
		"12345678":                  true,
		" correct horse ":           true, // spaces around and inside are kept
		"        ":                  false,
		"\t\n\r \u00a0\u2003\u3000": false, // tabs, newlines and Unicode spaces too
		"":                          false,
	}
	for password, want := range cases {
		assert.Equalf(t, want, validation.PasswordNotBlank(password), "%q", password)
	}
}
