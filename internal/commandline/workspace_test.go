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

func TestReportSuspendedSaysSendingIsFrozenAndTheOwnerTold(t *testing.T) {
	var out bytes.Buffer

	err := commandline.ReportSuspended(&out, workspaceID, true, nil)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "suspended: outbound sending is frozen and the owner has been notified")
}

func TestReportSuspendedOfASuspendedWorkspaceSaysNothingChanged(t *testing.T) {
	var out bytes.Buffer

	err := commandline.ReportSuspended(&out, workspaceID, false, nil)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "is already suspended; nothing changed")
}

func TestReportUnsuspendedSaysHeldSendsResume(t *testing.T) {
	var out bytes.Buffer

	err := commandline.ReportUnsuspended(&out, workspaceID, true, nil)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "held sends will resume")
}

func TestReportUnsuspendedOfAWorkspaceNotSuspendedSaysNothingChanged(t *testing.T) {
	var out bytes.Buffer

	err := commandline.ReportUnsuspended(&out, workspaceID, false, nil)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "is not suspended; nothing changed")
}

func TestReportOfAFailedSuspensionChangePrintsNothing(t *testing.T) {
	failed := errors.New("workspace not found")
	var out bytes.Buffer

	suspendErr := commandline.ReportSuspended(&out, workspaceID, false, failed)
	unsuspendErr := commandline.ReportUnsuspended(&out, workspaceID, false, failed)

	require.ErrorIs(t, suspendErr, failed)
	require.ErrorIs(t, unsuspendErr, failed)
	assert.Empty(t, out.String())
}
