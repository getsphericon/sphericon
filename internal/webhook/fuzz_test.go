package webhook_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/webhook"
)

// Retry-After is a header of a receiver we do not control: whatever it says, the wait read from it
// is never negative, an unreadable value is no wait at all, and the snooze a 429 takes stays
// within its bounds.
func FuzzRetryAfterIsABoundedWait(f *testing.F) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC).Unix()
	f.Add("120", now)
	f.Add(" 0 ", now)
	f.Add("Thu, 01 Jan 2026 12:05:00 GMT", now)
	f.Add("Wed, 31 Dec 2025 12:00:00 GMT", now)
	f.Add("Fri, 31 Dec 9999 23:59:59 GMT", now)
	f.Add("2147483648", now)
	f.Add("-5", now)
	f.Add("soon", now)

	f.Fuzz(func(t *testing.T, value string, unix int64) {
		at := time.Unix(unix, 0)

		wait, ok := webhook.ParseRetryAfter(value, at)
		snooze := webhook.HookSnooze(value, at)

		assert.GreaterOrEqualf(t, wait, time.Duration(0), "%q", value)
		assert.Truef(t, ok || wait == 0, "%q: an unreadable value waits %s", value, wait)
		assert.Positivef(t, snooze, "%q", value)
		assert.LessOrEqualf(t, snooze, webhook.HookSnoozeMax, "%q", value)
	})
}
