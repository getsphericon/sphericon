package accounts_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestCreateOwnerRefusesAMalformedEmail(t *testing.T) {
	env := testhelper.Setup(t)
	acc := accounts.New(env.DB, env.Bus, testhelper.Passwords(), env.ConsentRecords)

	_, err := acc.CreateOwner(t.Context(), "not-an-email", "Ada")

	require.EqualError(t, err, `"not-an-email" is not an email address`)
	taken, err := acc.EmailTaken(t.Context(), "not-an-email")
	require.NoError(t, err)
	assert.False(t, taken, "a refusal creates no User")
}

func TestReportOwnerCreatedShowsTheFirstWorkspaceAndTheOneTimePassword(t *testing.T) {
	env := testhelper.Setup(t)
	acc := accounts.New(env.DB, env.Bus, testhelper.Passwords(), env.ConsentRecords)
	password, err := acc.CreateOwner(t.Context(), "ada@example.com", "Ada Lovelace")
	var out bytes.Buffer

	err = accounts.ReportOwnerCreated(&out, "ada@example.com", password, err)

	require.NoError(t, err)
	assert.Contains(t, out.String(), `its first workspace "`+accounts.FirstWorkspaceName+`"`)
	assert.Contains(t, out.String(), "password (shown once): "+password)
}

func TestReportOwnerCreatedOfATakenEmailSaysNothingChanged(t *testing.T) {
	env := testhelper.Setup(t)
	acc := accounts.New(env.DB, env.Bus, testhelper.Passwords(), env.ConsentRecords)
	password, err := acc.CreateOwner(t.Context(), fixtures.OwnerJohnEmail, "John")
	var out bytes.Buffer

	err = accounts.ReportOwnerCreated(&out, fixtures.OwnerJohnEmail, password, err)

	require.ErrorContains(t, err, "already exists; nothing changed")
	assert.Empty(t, out.String())
}
