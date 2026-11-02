package validation

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// The length of a password, in characters (NIST SP 800-63B counts characters, not bytes). Every
// character is allowed, spaces and non-ASCII included: length is the rule, composition is not.
const (
	PasswordMinLength = 8
	PasswordMaxLength = 256
)

// PasswordLength reports whether s is valid UTF-8 of an acceptable length (the
// `password_length` tag).
func PasswordLength(s string) bool {
	n := utf8.RuneCountInString(s)
	return utf8.ValidString(s) && n >= PasswordMinLength && n <= PasswordMaxLength
}

// PasswordNotBlank reports whether s has a character that is not white space (the
// `password_not_blank` tag). Spaces inside or around a password stay allowed and are kept as
// sent: sign-in compares the password as typed, so trimming it here would change it.
func PasswordNotBlank(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool { return !unicode.IsSpace(r) }) >= 0
}

// NameMaxLength is the most characters a User's name has. A name is shown in the app, in the
// mail sent to and about the User and in the Audit log, so it is a line of text, not a document.
// contract-copy: UserName's @maxLength, also held by a provider's name, which no contract carries
const NameMaxLength = 100

// DisplayName reports whether s is a name a User may have: valid UTF-8 of at most NameMaxLength
// characters, at least one of them visible, and none that breaks a line or hides. Control
// characters and line or paragraph separators split a mail header or a log line; a bidirectional
// override reorders the text around it, so a name could read as something else in a notice; a
// format or default-ignorable character (a zero-width space, a Hangul filler) draws nothing, so a
// name of them looks empty. Every script and every emoji with its joiners stays allowed: the
// zero-width non-joiner and joiner are part of a spelling, and variation selectors are not format
// characters. The tag characters of the subdivision flags are format characters, so those flags
// are refused.
func DisplayName(s string) bool {
	return utf8.ValidString(s) && utf8.RuneCountInString(s) <= NameMaxLength &&
		!strings.ContainsFunc(s, hidden) &&
		strings.ContainsFunc(s, func(r rune) bool { return !unicode.IsSpace(r) && !joiner(r) })
}

// hidden reports whether r breaks a line or draws nothing (see DisplayName).
func hidden(r rune) bool {
	return unicode.In(r, unicode.Cc, unicode.Zl, unicode.Zp, unicode.Other_Default_Ignorable_Code_Point) ||
		unicode.Is(unicode.Cf, r) && !joiner(r)
}

// joiner reports whether r is the zero-width non-joiner or joiner, the two format characters a
// name may hold.
func joiner(r rune) bool { return r == '\u200c' || r == '\u200d' }

// The limits of RFC 5321: a forward path of 256 octets holds an address of 254 between its angle
// brackets, a local part of 64 and a domain label of 63.
const (
	maxAddressLength = 254
	// contract-copy: RFC 5321's local-part limit, not a limit of the contract
	maxLocalLength = 64
	maxLabelLength = 63
)

// AccountEmail reports whether s is an address a person can receive the account's mail at (the
// `account_email` tag): an ASCII dot-atom local part and a domain of at least two labels of
// letters, digits and inner hyphens (punycode included), whose top-level label is not all digits.
// It is stricter than RFC 5322 on purpose: quoted local parts, address literals, IP addresses and
// dotless domains are valid syntax that no public mailbox uses.
func AccountEmail(s string) bool {
	if len(s) > maxAddressLength {
		return false
	}
	local, domain, ok := strings.Cut(s, "@")
	return ok && dotAtom(local) && hostname(domain)
}

// dotAtom is RFC 5322's dot-atom: runs of atext joined by single dots.
func dotAtom(s string) bool {
	if s == "" || len(s) > maxLocalLength {
		return false
	}
	for atom := range strings.SplitSeq(s, ".") {
		if atom == "" || !all(atom, isAtext) {
			return false
		}
	}
	return true
}

func isAtext(r rune) bool {
	return isLetter(r) || isDigit(r) || strings.ContainsRune("!#$%&'*+/=?^_`{|}~-", r)
}

// hostname is a domain of at least two LDH labels whose last one is not all digits.
func hostname(s string) bool {
	labels := strings.Split(s, ".")
	if len(labels) < 2 {
		return false
	}
	for _, l := range labels {
		if !ldhLabel(l) {
			return false
		}
	}
	return !all(labels[len(labels)-1], isDigit)
}

// ldhLabel is a label of letters, digits and hyphens that neither starts nor ends with a hyphen.
func ldhLabel(l string) bool {
	if l == "" || len(l) > maxLabelLength || l[0] == '-' || l[len(l)-1] == '-' {
		return false
	}
	return all(l, isLDH)
}

// all reports whether every rune of s satisfies ok.
func all(s string, ok func(rune) bool) bool {
	for _, r := range s {
		if !ok(r) {
			return false
		}
	}
	return true
}

func isLetter(r rune) bool { return 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' }
func isDigit(r rune) bool  { return '0' <= r && r <= '9' }
func isLDH(r rune) bool    { return isLetter(r) || isDigit(r) || r == '-' }
