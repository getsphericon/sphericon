package jobs_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/contactimport"
	"github.com/getsphericon/sphericon/ent/contactimportreportrow"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contactimports"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/csvimport"
	"github.com/getsphericon/sphericon/internal/kernel/fieldname"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// sweepContactImports works the periodic sweep of the CSV imports, as river runs it, and the
// checks of the running imports' jobs it queues.
func sweepContactImports(t *testing.T, env *testhelper.TestEnv) {
	t.Helper()
	env.EnqueueJob(t, jobs.SweepContactImportsArgs{})
	env.PerformEnqueuedJobs(t, jobs.SweepContactImportsArgs{}, jobs.RecoverContactImportArgs{})
}

// What a CSV import keeps of personal data expires (ADR 0138): an upload that never ran after a
// day, a report seven days after its run ended; a younger upload and an unexpired report stay,
// and an import whose report expired keeps its row.
func TestTheSweepDeletesAbandonedUploadsAndOldReports(t *testing.T) {
	at := time.Now()
	env := testhelper.Setup(t, testhelper.WithClock(func() time.Time { return at }))
	ctx := t.Context()
	imports := testhelper.Module[*contactimports.Module](env)
	young, err := imports.Upload(ctx, env.DB.Scoped(fixtures.AcmeID), contactimports.UploadInput{File: strings.NewReader("email\na@example.com\n"), MembershipID: fixtures.AcmeOwnerMembershipID})
	require.NoError(t, err)
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

	at = env.FixturesNow().Add(time.Hour)
	sweepContactImports(t, env)

	assert.False(t, exists(fixtures.ContactImportGlobexUploadedID), "an upload a day old that never ran")
	assert.True(t, exists(young.Import.ID), "an upload of the hour")
	assert.Equal(t, 1, reportRows(fixtures.ContactImportAcmeDoneID), "a report that expires in four days")

	at = env.FixturesNow().Add(5 * 24 * time.Hour)
	sweepContactImports(t, env)

	assert.Zero(t, reportRows(fixtures.ContactImportAcmeDoneID), "a report seven days old")
	assert.True(t, exists(fixtures.ContactImportAcmeDoneID), "the import that ended stays")
}

// runningImportOf uploads rows addresses as Acme's import by its owner, maps them to the email key
// and runs it, its job queued: the import's id.
func runningImportOf(t *testing.T, env *testhelper.TestEnv, rows int) int64 {
	t.Helper()
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	imports := testhelper.Module[*contactimports.Module](env)
	var file strings.Builder
	file.WriteString("email\n")
	for row := 1; row <= rows; row++ {
		fmt.Fprintf(&file, "r%d@example.com\n", row)
	}
	uploaded, err := imports.Upload(ctx, acme, contactimports.UploadInput{File: strings.NewReader(file.String()), MembershipID: fixtures.AcmeOwnerMembershipID})
	require.NoError(t, err)
	_, err = imports.SaveMapping(ctx, acme, uploaded.Import.ID, contactimports.MappingInput{Mapping: csvimport.Mapping{
		Columns:          []csvimport.Column{{Kind: csvimport.KindKey, Key: fieldname.Email}},
		DecimalSeparator: csvimport.DecimalPoint,
	}})
	require.NoError(t, err)
	_, err = imports.Run(ctx, acme, uploaded.Import.ID, membership.RoleOwner)
	require.NoError(t, err)
	return uploaded.Import.ID
}

// importOf is Acme's import id as it stands.
func importOf(t *testing.T, env *testhelper.TestEnv, id int64) contactimports.View {
	t.Helper()
	v, err := testhelper.Module[*contactimports.Module](env).Get(t.Context(), env.DB.Scoped(fixtures.AcmeID), id)
	require.NoError(t, err)
	return v
}

// A run whose job is gone (its process killed on the job's last try, river gave up on it unseen)
// is queued again by the sweep and resumes from its checkpoint to the end.
func TestTheSweepResumesARunWhoseJobIsGone(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	id := runningImportOf(t, env, 1500)
	// A first try wrote a batch, then its process died and its job went with it.
	_, err := testhelper.Module[*contactimports.Module](env).Work(ctx, env.DB.Scoped(fixtures.AcmeID),
		contactimports.WorkInput{ImportID: id, Until: env.FixturesNow().Add(-time.Hour)})
	require.NoError(t, err)
	env.DeleteJobs(t, contactimports.RunArgs{})

	sweepContactImports(t, env)
	env.PerformEnqueuedJobs(t, contactimports.RunArgs{})
	done := importOf(t, env, id)

	assert.Equal(t, contactimport.StateDone, done.Import.State)
	assert.Equal(t, 1500, done.Import.Processed)
	assert.Equal(t, 1500, done.Import.Created)
}

// A run the sweep queued again is cancelled as any running import is.
func TestARunTheSweepResumedIsCancelled(t *testing.T) {
	env := testhelper.Setup(t)
	id := runningImportOf(t, env, 10)
	env.DeleteJobs(t, contactimports.RunArgs{})

	sweepContactImports(t, env)
	_, err := testhelper.Module[*contactimports.Module](env).Cancel(t.Context(), env.DB.Scoped(fixtures.AcmeID), id)
	require.NoError(t, err)
	env.PerformEnqueuedJobs(t, contactimports.RunArgs{})

	assert.Equal(t, contactimport.StateCancelled, importOf(t, env, id).Import.State)
}

// A run whose job is still waiting or working is left to it: the sweep queues no second one.
func TestTheSweepLeavesARunWhoseJobIsLive(t *testing.T) {
	env := testhelper.Setup(t)
	id := runningImportOf(t, env, 10)

	sweepContactImports(t, env)

	assert.Len(t, env.JobRowsOf(t, contactimports.RunArgs{}.Kind()), 1)
	assert.Zero(t, importOf(t, env, id).Import.Recoveries)
}

// A run whose job keeps vanishing is queued again a bounded number of times, then fails, keeping
// what it wrote, so it no longer holds the Workspace's one run.
func TestARunWhoseJobKeepsVanishingEndsFailed(t *testing.T) {
	env := testhelper.Setup(t)
	id := runningImportOf(t, env, 10)

	for range contactimports.RunRecoveries + 1 {
		env.DeleteJobs(t, contactimports.RunArgs{})
		sweepContactImports(t, env)
	}

	assert.Equal(t, contactimport.StateFailed, importOf(t, env, id).Import.State)
	assert.Equal(t, contactimports.RunRecoveries, importOf(t, env, id).Import.Recoveries)
}
