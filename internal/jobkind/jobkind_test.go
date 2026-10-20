package jobkind_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/jobkind"
)

// Erasure purges exactly these kinds of queued job; each must declare the argument it is
// matched by. A new job that carries personal data is added here and to jobkind.Purged.
func TestEveryPurgedKindDeclaresItsKey(t *testing.T) {
	wanted := []string{
		"deliver_webhook",
		"automation_evaluate_trigger",
		"automation_run_step",
		"automation_http_step",
		"send_broadcast_recipient",
	}

	declared := map[string]string{}
	for _, k := range jobkind.Purged {
		declared[k.Kind] = k.Arg
	}
	for _, kind := range wanted {
		assert.NotEmpty(t, declared[kind], "kind %q has no declared purge key", kind)
	}
	assert.Len(t, jobkind.Purged, len(wanted))
}
