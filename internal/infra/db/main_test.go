package db_test

import (
	"testing"

	"go.uber.org/goleak"
)

// A test that leaves a goroutine running fails the package: the report of a migration's wait for
// its lock (reportingLocker) has to stop before the lock is returned.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
