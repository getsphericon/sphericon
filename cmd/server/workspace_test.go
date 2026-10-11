package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mokevnin/sphericon/internal/suspension"
)

// opsReporting is workspaceOps whose suspend and unsuspend both report changed.
func opsReporting(changed bool) *workspaceOpsMock {
	return &workspaceOpsMock{
		SuspendWorkspaceFunc:   func(context.Context, string, suspension.Actor, string) (bool, error) { return changed, nil },
		UnsuspendWorkspaceFunc: func(context.Context, string, suspension.Actor) (bool, error) { return changed, nil },
	}
}

func TestWorkspaceSuspendJoinsTheReasonWords(t *testing.T) {
	ops := opsReporting(true)
	var out bytes.Buffer

	require.NoError(t, runWorkspace(context.Background(), ops, []string{"suspend", "acme", "complaint", "rate", "too", "high"}, &out))

	calls := ops.SuspendWorkspaceCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "acme", calls[0].Slug)
	assert.Equal(t, suspension.CLI, calls[0].By)
	assert.Equal(t, "complaint rate too high", calls[0].Reason)
	assert.Contains(t, out.String(), "suspended")
}

func TestWorkspaceSuspendNeedsASlugAndAReason(t *testing.T) {
	ops := opsReporting(false)
	for _, args := range [][]string{{"suspend"}, {"suspend", "acme"}} {
		err := runWorkspace(context.Background(), ops, args, &bytes.Buffer{})
		require.Error(t, err, "args %v", args)
	}
	assert.Empty(t, ops.SuspendWorkspaceCalls())
}

func TestWorkspaceUnsuspendReportsNoChange(t *testing.T) {
	ops := opsReporting(false)
	var out bytes.Buffer

	require.NoError(t, runWorkspace(context.Background(), ops, []string{"unsuspend", "acme"}, &out))

	calls := ops.UnsuspendWorkspaceCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "acme", calls[0].Slug)
	assert.Contains(t, out.String(), "not suspended")
}

func TestWorkspaceUnknownSubcommand(t *testing.T) {
	require.Error(t, runWorkspace(context.Background(), opsReporting(false), []string{"delete", "acme"}, &bytes.Buffer{}))
	require.Error(t, runWorkspace(context.Background(), opsReporting(false), nil, &bytes.Buffer{}))
}
