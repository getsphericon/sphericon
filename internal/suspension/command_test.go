package suspension_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/suspension"
)

func TestReportSuspendedSaysSendingIsFrozenAndTheOwnerTold(t *testing.T) {
	var out bytes.Buffer

	err := suspension.ReportSuspended(&out, fixtures.AcmePublicID, true, nil)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "suspended: outbound sending is frozen and the owner has been notified")
}

func TestReportSuspendedOfASuspendedWorkspaceSaysNothingChanged(t *testing.T) {
	var out bytes.Buffer

	err := suspension.ReportSuspended(&out, fixtures.AcmePublicID, false, nil)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "is already suspended; nothing changed")
}

func TestReportUnsuspendedSaysHeldSendsResume(t *testing.T) {
	var out bytes.Buffer

	err := suspension.ReportUnsuspended(&out, fixtures.AcmePublicID, true, nil)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "held sends will resume")
}

func TestReportUnsuspendedOfAWorkspaceNotSuspendedSaysNothingChanged(t *testing.T) {
	var out bytes.Buffer

	err := suspension.ReportUnsuspended(&out, fixtures.AcmePublicID, false, nil)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "is not suspended; nothing changed")
}

func TestReportOfAFailedChangePrintsNothing(t *testing.T) {
	failed := errors.New("workspace not found")
	var out bytes.Buffer

	suspendErr := suspension.ReportSuspended(&out, fixtures.AcmePublicID, false, failed)
	unsuspendErr := suspension.ReportUnsuspended(&out, fixtures.AcmePublicID, false, failed)

	require.ErrorIs(t, suspendErr, failed)
	require.ErrorIs(t, unsuspendErr, failed)
	assert.Empty(t, out.String())
}
