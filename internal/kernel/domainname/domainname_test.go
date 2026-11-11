package domainname_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/kernel/domainname"
)

// Whatever Normalize accepts, it returns in Pattern, the form the database checks, and reading its
// own output again changes nothing.
func TestNormalizeReturnsThePattern(t *testing.T) {
	pattern := regexp.MustCompile(domainname.Pattern)
	label := rapid.StringMatching(`[A-Za-z0-9äöüßé_-]{0,12}`)
	rapid.Check(t, func(t *rapid.T) {
		labels := rapid.SliceOfN(label, 1, 4).Draw(t, "labels")
		raw := rapid.SampledFrom([]string{"", " ", "."}).Draw(t, "pad") +
			strings.Join(labels, ".") +
			rapid.SampledFrom([]string{"", ".", " "}).Draw(t, "suffix")
		got, ok := domainname.Normalize(raw)
		again, againOK := domainname.Normalize(got)
		assert.True(t, !ok || pattern.MatchString(got), "Normalize(%q) = %q, not in the pattern", raw, got)
		assert.True(t, !ok || (againOK && again == got), "Normalize(%q) = %q: not its own normal form", got, again)
	})
}

func TestNormalizeExamples(t *testing.T) {
	for _, c := range []struct{ raw, want string }{
		{"Mail.Example.COM.", "mail.example.com"},
		{"\tbücher.example ", "xn--bcher-kva.example"},
	} {
		got, ok := domainname.Normalize(c.raw)
		assert.True(t, ok, c.raw)
		assert.Equal(t, c.want, got)
	}
	for _, raw := range []string{"localhost", "127.0.0.1", "a..b", "under_score.example"} {
		_, ok := domainname.Normalize(raw)
		assert.False(t, ok, raw)
	}
}
