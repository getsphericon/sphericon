package commandline_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/commandline"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
)

const ownerEmail = "ada@example.com"

func TestReportOwnerCreatedShowsTheFirstWorkspaceAndTheOneTimePassword(t *testing.T) {
	var out bytes.Buffer

	err := commandline.ReportOwnerCreated(&out, ownerEmail, "one-time-pw", nil)

	require.NoError(t, err)
	assert.Contains(t, out.String(), `its first workspace "`+users.FirstWorkspaceName+`"`)
	assert.Contains(t, out.String(), "password (shown once): one-time-pw")
}

func TestReportOwnerCreatedOfATakenEmailSaysNothingChanged(t *testing.T) {
	var out bytes.Buffer

	err := commandline.ReportOwnerCreated(&out, ownerEmail, "", users.ErrEmailTaken)

	require.ErrorContains(t, err, "already exists; nothing changed")
	assert.Empty(t, out.String())
}
