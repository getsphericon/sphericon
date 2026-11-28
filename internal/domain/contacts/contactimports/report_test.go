package contactimports_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/contacts/contactimports"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A report whose chunks were read whole is still kept: only the driver knows the file reached the
// client, and the report is deleted when it says so.
func TestAReportIsDeletedOnlyOnceItsDownloadIsDelivered(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	imports := testhelper.Module[*contactimports.Module](env)
	link, err := imports.ReportLink(ctx, acme, fixtures.ContactImportAcmeDoneID, "a-session")
	require.NoError(t, err)
	d, err := imports.Report(ctx, acme, contactimports.ReportInput{ImportID: fixtures.ContactImportAcmeDoneID, Token: link.Token, Session: "a-session"})
	require.NoError(t, err)

	var chunks int
	for _, err := range d.Chunks {
		require.NoError(t, err)
		chunks++
	}
	read, err := imports.Get(ctx, acme, fixtures.ContactImportAcmeDoneID)
	require.NoError(t, err)
	require.NoError(t, d.Delivered(ctx))
	delivered, err := imports.Get(ctx, acme, fixtures.ContactImportAcmeDoneID)
	require.NoError(t, err)

	assert.Positive(t, chunks)
	assert.Positive(t, read.ReportRows, "read whole, the report is kept until it is delivered")
	assert.Zero(t, delivered.ReportRows)
	assert.Nil(t, delivered.Import.ReportExpiresAt)
}
