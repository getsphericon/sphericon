package domainname_test

import (
	"errors"
	"fmt"
	"net"
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

// A missing record is an answer, wrapped or not; a resolver that timed out or failed gave none.
func TestNotFoundIsOnlyADefiniteAnswer(t *testing.T) {
	missing := &net.DNSError{Err: "no such host", Name: "example.com", IsNotFound: true}
	wrapped := fmt.Errorf("lookup: %w", missing)
	timedOut := &net.DNSError{Err: "i/o timeout", Name: "example.com", IsNotFound: true, IsTimeout: true}
	failed := &net.DNSError{Err: "server misbehaving", Name: "example.com", IsTemporary: true}
	other := errors.New("refused")

	assert.True(t, domainname.NotFound(missing))
	assert.True(t, domainname.NotFound(wrapped))
	assert.False(t, domainname.NotFound(timedOut))
	assert.False(t, domainname.NotFound(failed))
	assert.False(t, domainname.NotFound(other))
	assert.False(t, domainname.NotFound(nil))
}
