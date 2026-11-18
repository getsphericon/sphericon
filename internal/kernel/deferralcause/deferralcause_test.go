package deferralcause_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/kernel/deferralcause"
)

// The list holds the per-source causes and the per-recipient ones, each once.
func TestTheDeferralCausesAreOneClosedList(t *testing.T) {
	assert.Equal(t, []string{"provider_limit", "send_cap", "provider", "sending_window", "frequency_cap"},
		deferralcause.Cause("").Values())
}
