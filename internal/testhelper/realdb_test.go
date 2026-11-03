package testhelper_test

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The baseline's worker is held by a session of the lease, which the server drops when the
// process ends. A lease nothing refers to is closed by the garbage collector, which hands the
// worker to another process: that one loads its fixtures into it and drops the clones of the
// baseline, mid-run.
func TestBaselineWorkerStaysLeasedAfterGarbageCollection(t *testing.T) {
	require.True(t, testhelper.BaselineLeaseHeld(t), "the lease is held once the baseline is open")

	for range 3 {
		runtime.GC()
	}

	require.True(t, testhelper.BaselineLeaseHeld(t), "the lease was dropped with its session")
}
