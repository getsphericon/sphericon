package commandline_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/commandline"
)

const workspaceID = "ws_acme"

func TestSuspensionReportSaysSendingIsFrozenAndTheOwnerTold(t *testing.T) {
	var out bytes.Buffer

	err := commandline.Suspension.Report(&out, workspaceID, true, nil)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "suspended: outbound sending is frozen and the owner has been notified")
}

func TestSuspensionReportOfASuspendedWorkspaceSaysNothingChanged(t *testing.T) {
	var out bytes.Buffer

	err := commandline.Suspension.Report(&out, workspaceID, false, nil)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "is already suspended; nothing changed")
}

func TestUnsuspensionReportSaysHeldSendsResume(t *testing.T) {
	var out bytes.Buffer

	err := commandline.Unsuspension.Report(&out, workspaceID, true, nil)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "held sends will resume")
}

func TestUnsuspensionReportOfAWorkspaceNotSuspendedSaysNothingChanged(t *testing.T) {
	var out bytes.Buffer

	err := commandline.Unsuspension.Report(&out, workspaceID, false, nil)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "is not suspended; nothing changed")
}

func TestReportOfAFailedSuspensionChangePrintsNothing(t *testing.T) {
	failed := errors.New("workspace not found")
	var out bytes.Buffer

	suspendErr := commandline.Suspension.Report(&out, workspaceID, false, failed)
	unsuspendErr := commandline.Unsuspension.Report(&out, workspaceID, false, failed)

	require.ErrorIs(t, suspendErr, failed)
	require.ErrorIs(t, unsuspendErr, failed)
	assert.Empty(t, out.String())
}
