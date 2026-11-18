package skipreason_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/kernel/skipreason"
)

// The list holds the reasons Send-eligibility gives and the ones Outbound send adds, each once.
func TestTheSkippedReasonsAreOneClosedList(t *testing.T) {
	assert.Equal(t, []string{
		"suppressed", "unsubscribed_everything", "unsubscribed_source", "unconfirmed",
		"no_destination", "contact_erased", "no_local_time", "frequency_cap",
	}, skipreason.Reason("").Values())
}
