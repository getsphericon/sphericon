package jobs_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/contactimport"
	"github.com/getsphericon/sphericon/ent/contactimportreportrow"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contactimports"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// What a CSV import keeps of personal data expires (ADR 0138): an upload that never ran after a
// day, a report seven days after its run ended; a younger upload and an unexpired report stay,
// and an import whose report expired keeps its row.
func TestTheExpiryDeletesAbandonedUploadsAndOldReports(t *testing.T) {
	at := time.Now()
	env := testhelper.Setup(t, testhelper.WithClock(func() time.Time { return at }))
	ctx := t.Context()
	imports := testhelper.Module[*contactimports.Module](env)
	young, err := imports.Upload(ctx, env.DB.Scoped(fixtures.AcmeID), contactimports.UploadInput{File: strings.NewReader("email\na@example.com\n"), MembershipID: fixtures.AcmeOwnerMembershipID})
	require.NoError(t, err)
	expire := func(now time.Time) {
		t.Helper()
		at = now
		w := jobs.NewExpireContactImportsWorker(imports, env.DB)
		require.NoError(t, w.Work(ctx, job(jobs.ExpireContactImportsArgs{})))
	}
	exists := func(id int64) bool {
		ok, err := env.DB.ContactImport.Query().Where(contactimport.ID(id)).Exist(ctx)
		require.NoError(t, err)
		return ok
	}
	reportRows := func(id int64) int {
		n, err := env.DB.ContactImportReportRow.Query().Where(contactimportreportrow.ContactImportID(id)).Count(ctx)
		require.NoError(t, err)
		return n
	}

	expire(env.FixturesNow().Add(time.Hour))

	assert.False(t, exists(fixtures.ContactImportGlobexUploadedID), "an upload a day old that never ran")
	assert.True(t, exists(young.Import.ID), "an upload of the hour")
	assert.Equal(t, 1, reportRows(fixtures.ContactImportAcmeDoneID), "a report that expires in four days")

	expire(env.FixturesNow().Add(5 * 24 * time.Hour))

	assert.Zero(t, reportRows(fixtures.ContactImportAcmeDoneID), "a report seven days old")
	assert.True(t, exists(fixtures.ContactImportAcmeDoneID), "the import that ended stays")
}
