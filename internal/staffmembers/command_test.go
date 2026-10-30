package staffmembers_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/staffmembers"
)

const newStaffEmail = "new-staff@sphericon.test"

func TestReportCreatedShowsTheOneTimePasswordOnce(t *testing.T) {
	var out bytes.Buffer

	err := staffmembers.ReportCreated(&out, newStaffEmail, "one-time-pw", nil)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "password (shown once): one-time-pw")
	assert.Contains(t, out.String(), "enrols a TOTP at first login")
}

func TestReportCreatedExplainsARefusal(t *testing.T) {
	duplicate := staffmembers.ReportCreated(&bytes.Buffer{}, newStaffEmail, "", fmt.Errorf("create: %w", staffmembers.ErrDuplicate))
	off := staffmembers.ReportCreated(&bytes.Buffer{}, newStaffEmail, "", staffmembers.ErrSignInOff)

	require.ErrorContains(t, duplicate, "already exists; nothing changed")
	require.ErrorContains(t, off, "set SPHERICON_STAFF_JWT_SECRET")
}

func TestReportTOTPResetSaysTheStaffReenrols(t *testing.T) {
	var out bytes.Buffer

	err := staffmembers.ReportTOTPReset(&out, newStaffEmail, nil)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "re-enrol")
}

func TestReportTOTPResetExplainsARefusal(t *testing.T) {
	unknown := staffmembers.ReportTOTPReset(&bytes.Buffer{}, "nobody@sphericon.test", staffmembers.ErrNotFound)

	require.ErrorContains(t, unknown, "no staff with email")
}
