package events_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/events"
)

// An action in one of the system's namespaces is reserved however it is cased or padded; one
// outside them never is.
func TestReservedActionIgnoresCaseAndPadding(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		prefix := rapid.SampledFrom([]string{"email.", "marketing.", "contact."}).Draw(t, "prefix")
		rest := rapid.StringMatching(`[a-z_]{1,12}`).Draw(t, "rest")
		pad := rapid.StringMatching(`[ \t]{0,3}`).Draw(t, "pad")
		cased := rapid.SampledFrom([]func(string) string{strings.ToLower, strings.ToUpper}).Draw(t, "case")

		assert.True(t, events.ReservedAction(pad+cased(prefix+rest)+pad))
	})
}

func TestCustomerActionsAreNotReserved(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		action := rapid.StringMatching(`[a-z_]{1,12}(\.[a-z_]{1,12})?`).Draw(t, "action")
		for _, prefix := range []string{"email.", "marketing.", "contact."} {
			if strings.HasPrefix(action, prefix) {
				t.Skip()
			}
		}
		assert.False(t, events.ReservedAction(action))
	})
}
