package webhooks_test

import (
	"testing"

	"go.uber.org/goleak"
)

// A test that leaves a goroutine running fails the package, so a server or worker that does not
// stop on shutdown shows up here instead of as a slow, flaky suite.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
