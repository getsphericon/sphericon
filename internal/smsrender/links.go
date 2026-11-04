package smsrender

import (
	"regexp"
	"strings"
)

// link is an absolute http(s) URL as a phone finds it in a text: the scheme, then everything up to
// the next whitespace. A Liquid tag inside it (`{{ contact.id }}`, which holds spaces) is part of
// it, so a written text and its rendered form have the same links. A lone `{` ends it.
// src/utils/smsParts.ts finds the same links, and both agree on testdata/links.json.
// Whitespace is spelled out: the two regexp engines differ on what `\s` covers.
var link = regexp.MustCompile(`https?://(?:\{\{.*?\}\}|\{%.*?%\}|[^\t\n\f\r {])+`)

// trailing is the punctuation that ends a sentence rather than a link: a phone leaves it out too.
const trailing = `.,;:!?'"`

// spans are the [start, end) byte offsets of the links of text, in order.
func spans(text string) [][2]int {
	var out [][2]int
	for _, m := range link.FindAllStringIndex(text, -1) {
		candidate := text[m[0]:m[1]]
		end := trim(candidate)
		if end <= strings.Index(candidate, "://")+len("://") {
			continue // nothing but punctuation after the scheme
		}
		out = append(out, [2]int{m[0], m[0] + end})
	}
	return out
}

// trim is the length of candidate once the punctuation that follows the link is cut: trailing
// sentence marks, and a `)` with no `(` to close inside the link.
func trim(candidate string) int {
	end := len(candidate)
	for end > 0 {
		c := candidate[end-1]
		switch {
		case strings.IndexByte(trailing, c) >= 0:
			end--
		case c == ')' && strings.Count(candidate[:end], "(") < strings.Count(candidate[:end], ")"):
			end--
		default:
			return end
		}
	}
	return end
}

// Links are the links of text, in order: what the Outbound send turns into short tracked links
// (ADR 0083).
func Links(text string) []string {
	found := spans(text)
	out := make([]string, 0, len(found))
	for _, s := range found {
		out = append(out, text[s[0]:s[1]])
	}
	return out
}

// RewriteLinks replaces every link of text with what short returns for it, the rest of the text as
// it is. The first error of short is returned.
func RewriteLinks(text string, short func(url string) (string, error)) (string, error) {
	var b strings.Builder
	at := 0
	for _, s := range spans(text) {
		url, err := short(text[s[0]:s[1]])
		if err != nil {
			return "", err
		}
		b.WriteString(text[at:s[0]])
		b.WriteString(url)
		at = s[1]
	}
	b.WriteString(text[at:])
	return b.String(), nil
}

// MeasureLinked counts text as it goes out when every link becomes a short link of linkLength
// characters (all of them GSM-7); a linkLength of 0 is a text whose links are not tracked, counted
// as written.
func MeasureLinked(text string, linkLength int) Size {
	if linkLength <= 0 {
		return Measure(text)
	}
	short := strings.Repeat("x", linkLength)
	var linked strings.Builder
	at := 0
	for _, s := range spans(text) {
		linked.WriteString(text[at:s[0]])
		linked.WriteString(short)
		at = s[1]
	}
	linked.WriteString(text[at:])
	return Measure(linked.String())
}
