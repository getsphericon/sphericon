package messaging

import (
	"errors"
	"strings"
)

// Provider replies that say the provider is busy, not that the message is bad
// (ADR 0023). An adapter wraps one of these around the provider's own error; the
// Outbound send turns it into a Deferral instead of a retryable failure.
var (
	// ErrBusy: the provider says we are sending too fast (SES Throttling, an SMTP
	// "too many messages" / "rate limit" 4xx reply). Back off briefly.
	ErrBusy = errors.New("provider is busy: sending too fast")
	// ErrQuotaExceeded: the provider's daily (24 hour) sending quota is spent. Capacity
	// returns gradually, so the backoff is long.
	ErrQuotaExceeded = errors.New("provider daily sending quota exceeded")
)

// ClassifyReply tells whether a provider reply is a "too fast" or "quota exceeded"
// one. text is the provider's message. It returns ErrQuotaExceeded, ErrBusy, or
// nil when the reply says nothing of the kind. Providers differ only in how they
// phrase it, so the wording heuristics live in one place; each adapter decides
// whether the reply is a transient one before asking.
func ClassifyReply(text string) error {
	t := strings.ToLower(text)
	switch {
	case strings.Contains(t, "quota") && (strings.Contains(t, "daily") || strings.Contains(t, "24")):
		return ErrQuotaExceeded
	case strings.Contains(t, "sending rate"), strings.Contains(t, "rate limit"),
		strings.Contains(t, "throttl"), strings.Contains(t, "too many messages"),
		strings.Contains(t, "too fast"):
		return ErrBusy
	default:
		return nil
	}
}
