package commandline_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/commandline"
	"github.com/getsphericon/sphericon/internal/domain/identity/staffmembers"
)

const newStaffEmail = "new-staff@sphericon.test"

func TestReportStaffCreatedShowsTheOneTimePasswordOnce(t *testing.T) {
	var out bytes.Buffer

	err := commandline.ReportStaffCreated(&out, newStaffEmail, "one-time-pw", nil)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "password (shown once): one-time-pw")
	assert.Contains(t, out.String(), "enrols a TOTP at first login")
}

func TestReportStaffCreatedExplainsARefusal(t *testing.T) {
	duplicate := commandline.ReportStaffCreated(&bytes.Buffer{}, newStaffEmail, "", fmt.Errorf("create: %w", staffmembers.ErrDuplicate))
	off := commandline.ReportStaffCreated(&bytes.Buffer{}, newStaffEmail, "", staffmembers.ErrSignInOff)

	require.ErrorContains(t, duplicate, "already exists; nothing changed")
	require.ErrorContains(t, off, "set SPHERICON_STAFF_JWT_SECRET")
}

func TestReportTOTPResetSaysTheStaffReenrols(t *testing.T) {
	var out bytes.Buffer

	err := commandline.ReportTOTPReset(&out, newStaffEmail, nil)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "re-enrol")
}

func TestReportTOTPResetExplainsARefusal(t *testing.T) {
	unknown := commandline.ReportTOTPReset(&bytes.Buffer{}, "nobody@sphericon.test", staffmembers.ErrNotFound)

	require.ErrorContains(t, unknown, "no staff with email")
}
