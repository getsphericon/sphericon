package contactimports_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	entgo "entgo.io/ent"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/contactimport"
	"github.com/getsphericon/sphericon/ent/contactimportchunk"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contactimports"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/csvimport"
	"github.com/getsphericon/sphericon/internal/kernel/fieldname"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A file of more than one chunk and more than one batch is read whole: a row that crosses a
// chunk's end is read as one, every row is written, the checkpoint ends at the file's end and the
// file is gone once the run is done.
func TestARunReadsAFileOfSeveralChunksInBatches(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	imports := testhelper.Module[*contactimports.Module](env)
	const rows = 1800
	var file strings.Builder
	file.WriteString("email,notes\n")
	notes := strings.Repeat("n", 600)
	for i := range rows {
		fmt.Fprintf(&file, "row%d@example.com,%s\n", i, notes)
	}

	uploaded, err := imports.Upload(ctx, acme, contactimports.UploadInput{File: strings.NewReader(file.String()), MembershipID: fixtures.AcmeOwnerMembershipID})
	require.NoError(t, err)
	_, err = imports.SaveMapping(ctx, acme, uploaded.Import.ID, contactimports.MappingInput{Mapping: csvimport.Mapping{
		Columns:          []csvimport.Column{{Kind: csvimport.KindKey, Key: fieldname.Email}, {Kind: csvimport.KindIgnore}},
		DecimalSeparator: csvimport.DecimalPoint,
	}})
	require.NoError(t, err)
	_, err = imports.Run(ctx, acme, uploaded.Import.ID, membership.RoleOwner)
	require.NoError(t, err)
	env.PerformEnqueuedJobs(t)
	done, err := imports.Get(ctx, acme, uploaded.Import.ID)
	require.NoError(t, err)
	chunks, err := acme.ContactImportChunk().Query().Where(contactimportchunk.ContactImportID(uploaded.Import.ID)).Count(ctx)
	require.NoError(t, err)

	assert.Greater(t, uploaded.Import.Size, int64(contactimports.ChunkSize), "the file is more than one chunk")
	assert.Equal(t, contactimport.StateDone, done.Import.State)
	assert.Equal(t, rows, done.Import.Processed)
	assert.Equal(t, rows, done.Import.Created)
	assert.Zero(t, done.ReportRows)
	assert.Equal(t, rows, done.Import.CheckpointRow)
	assert.Equal(t, uploaded.Import.Size, done.Import.CheckpointOffset)
	assert.Equal(t, int(uploaded.Import.Size/contactimports.ChunkSize), done.Import.CheckpointChunk)
	assert.Zero(t, chunks, "the file is deleted when the run ends")
}

// runningImport uploads file as Acme's import by the Membership, maps its one column to the email
// key and runs it, its job queued: the import's id.
func runningImport(t *testing.T, env *testhelper.TestEnv, file string, membershipID int64) int64 {
	t.Helper()
	return runningImportMapped(t, env, file, membershipID, []csvimport.Column{{Kind: csvimport.KindKey, Key: fieldname.Email}})
}

// runningImportMapped is runningImport with the columns of a file of several.
func runningImportMapped(t *testing.T, env *testhelper.TestEnv, file string, membershipID int64, columns []csvimport.Column) int64 {
	t.Helper()
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	imports := testhelper.Module[*contactimports.Module](env)
	uploaded, err := imports.Upload(ctx, acme, contactimports.UploadInput{File: strings.NewReader(file), MembershipID: membershipID})
	require.NoError(t, err)
	_, err = imports.SaveMapping(ctx, acme, uploaded.Import.ID, contactimports.MappingInput{Mapping: csvimport.Mapping{
		Columns:          columns,
		DecimalSeparator: csvimport.DecimalPoint,
	}})
	require.NoError(t, err)
	ms, err := env.DB.Membership.Get(ctx, membershipID)
	require.NoError(t, err)
	_, err = imports.Run(ctx, acme, uploaded.Import.ID, ms.Role)
	require.NoError(t, err)
	return uploaded.Import.ID
}

// emailFile is a file of one email column with an address per row: r<n>@example.com for row n.
func emailFile(rows int, cell func(row int) string) string {
	var file strings.Builder
	file.WriteString("email\n")
	for row := 1; row <= rows; row++ {
		file.WriteString(cell(row) + "\n")
	}
	return file.String()
}

// failContactWriteOnce fails the first write of the Contact of address, as a crash in the middle of
// a batch would leave it: the rows before it in the batch written, the batch's checkpoint not.
func failContactWriteOnce(env *testhelper.TestEnv, address string) {
	failed := false
	env.Bus.Use(func(next entgo.Mutator) entgo.Mutator {
		return entgo.MutateFunc(func(ctx context.Context, m entgo.Mutation) (entgo.Value, error) {
			if cm, ok := m.(*ent.ContactMutation); ok && !failed {
				if email, set := cm.Email(); set && email == address {
					failed = true
					return nil, errors.New("injected crash")
				}
			}
			return next.Mutate(ctx, m)
		})
	})
}

// resumedFile is a file of 2500 addresses and their status on the old platform: its row 1 is an
// address the Workspace has, row 1200 has none, row 1300 repeats row 1100's, rows 5 and 1050
// unsubscribed and row 1060 bounced.
func resumedFile() string {
	var file strings.Builder
	file.WriteString("email,status\n")
	for row := 1; row <= 2500; row++ {
		email, status := fmt.Sprintf("r%d@example.com", row), ""
		switch row {
		case 1:
			email = fixtures.ContactAliceEmail
		case 5, 1050:
			status = "unsubscribed"
		case 1060:
			status = "bounced"
		case 1200:
			email = ""
		case 1300:
			email = "r1100@example.com"
		}
		fmt.Fprintf(&file, "%s,%s\n", email, status)
	}
	return file.String()
}

// A run stopped in the middle of its second batch resumes from its checkpoint when its job runs
// again: the batch is run again, each Contact is written once, and the counts are those of an
// uninterrupted run (an existing Contact updated, a key repeated in the batch updated, a row
// without a key refused and reported once, the opt-outs and Suppressions of the batch run again
// counted once).
func TestARunStoppedMidFileResumesWithTheCountsOfAnUninterruptedRun(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	imports := testhelper.Module[*contactimports.Module](env)
	id := runningImportMapped(t, env, resumedFile(), fixtures.AcmeOwnerMembershipID, []csvimport.Column{
		{Kind: csvimport.KindKey, Key: fieldname.Email},
		{Kind: csvimport.KindRefusal, Fact: &csvimport.Fact{Channel: "email", Refusals: []csvimport.RefusalValue{
			{Value: "unsubscribed", Refusal: csvimport.RefusalEverything},
			{Value: "bounced", Refusal: csvimport.RefusalSuppression},
		}}},
	})
	failContactWriteOnce(env, "r1500@example.com")

	// The first try stops in the second batch; river tries the job again.
	env.TryPerformEnqueuedJobs(t, contactimports.RunArgs{})
	done, err := imports.Get(ctx, acme, id)
	require.NoError(t, err)
	made, err := acme.Contact().Query().Where(contact.SourceEQ(contact.SourceImport)).Count(ctx)
	require.NoError(t, err)

	assert.Equal(t, contactimport.StateDone, done.Import.State)
	assert.Equal(t, 2500, done.Import.Processed)
	assert.Equal(t, 2497, done.Import.Created)
	assert.Equal(t, 2, done.Import.Updated)
	assert.Equal(t, 1, done.Import.Refused)
	assert.Equal(t, 2, done.Import.OptedOut)
	assert.Equal(t, 1, done.Import.Suppressed)
	assert.Equal(t, 1, done.ReportRows)
	assert.Equal(t, 2497, made, "each Contact written once")
}

// A try past its time yields after the batch it wrote, the import still running with its
// checkpoint after that batch, and the next try ends it.
func TestATryPastItsTimeYieldsAfterABatchAndTheNextResumes(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	imports := testhelper.Module[*contactimports.Module](env)
	id := runningImport(t, env, emailFile(1500, func(row int) string { return fmt.Sprintf("r%d@example.com", row) }), fixtures.AcmeOwnerMembershipID)

	first, err := imports.Work(ctx, acme, contactimports.WorkInput{ImportID: id, Until: env.FixturesNow().Add(-time.Hour)})
	require.NoError(t, err)
	paused, err := imports.Get(ctx, acme, id)
	require.NoError(t, err)
	second, err := imports.Work(ctx, acme, contactimports.WorkInput{ImportID: id})
	require.NoError(t, err)
	done, err := imports.Get(ctx, acme, id)
	require.NoError(t, err)

	assert.Equal(t, contactimports.Paused, first)
	assert.Equal(t, contactimport.StateRunning, paused.Import.State)
	assert.Equal(t, 1000, paused.Import.CheckpointRow)
	assert.Equal(t, contactimports.Ended, second)
	assert.Equal(t, contactimport.StateDone, done.Import.State)
	assert.Equal(t, 1500, done.Import.Created)
}

// pausedAfterABatch is an import of 2500 addresses by the Membership whose first try wrote its
// first batch and yielded; its row 10 has no key, so its report has a row.
func pausedAfterABatch(t *testing.T, env *testhelper.TestEnv, membershipID int64) int64 {
	t.Helper()
	file := emailFile(2500, func(row int) string {
		if row == 10 {
			return `""`
		}
		return fmt.Sprintf("r%d@example.com", row)
	})
	id := runningImport(t, env, file, membershipID)
	out, err := testhelper.Module[*contactimports.Module](env).Work(t.Context(), env.DB.Scoped(fixtures.AcmeID),
		contactimports.WorkInput{ImportID: id, Until: env.FixturesNow().Add(-time.Hour)})
	require.NoError(t, err)
	require.Equal(t, contactimports.Paused, out)
	return id
}

// Cancel stops the run before its next batch: it ends cancelled, the Contacts it wrote stay, the
// file is gone and the report still downloads.
func TestACancelledRunStopsAfterItsBatchAndKeepsWhatItWrote(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	imports := testhelper.Module[*contactimports.Module](env)
	id := pausedAfterABatch(t, env, fixtures.AcmeOwnerMembershipID)

	asked, err := imports.Cancel(ctx, acme, id)
	require.NoError(t, err)
	_, again := imports.Cancel(ctx, acme, id)
	out, err := imports.Work(ctx, acme, contactimports.WorkInput{ImportID: id})
	require.NoError(t, err)
	ended, err := imports.Get(ctx, acme, id)
	require.NoError(t, err)
	chunks, err := acme.ContactImportChunk().Query().Where(contactimportchunk.ContactImportID(id)).Count(ctx)
	require.NoError(t, err)
	_, linkErr := imports.ReportLink(ctx, acme, id, "a-session")

	assert.Equal(t, contactimport.StateRunning, asked.Import.State, "the job stops it")
	require.ErrorIs(t, again, contactimports.ErrNotRunning)
	assert.Equal(t, contactimports.Ended, out)
	assert.Equal(t, contactimport.StateCancelled, ended.Import.State)
	assert.Equal(t, 1000, ended.Import.Processed)
	assert.Equal(t, 999, ended.Import.Created)
	assert.Equal(t, 1, ended.ReportRows)
	assert.Zero(t, chunks, "the file is deleted")
	require.NoError(t, linkErr, "the report downloads")
	assert.Equal(t, 1, countByEmail(t, env, "r1000@example.com"))
	assert.Zero(t, countByEmail(t, env, "r1001@example.com"))
}

// A Member who loses contacts:import while the run goes on stops it at its next batch.
func TestARunWhoseMemberLostContactsImportEndsCancelled(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	imports := testhelper.Module[*contactimports.Module](env)
	id := pausedAfterABatch(t, env, fixtures.AcmeAdminMembershipID)
	_, err := testhelper.Module[*memberships.Module](env).ChangeRole(ctx, acme,
		memberships.Caller{Actor: events.Actor{Kind: events.ActorUser}, Role: membership.RoleOwner},
		memberships.ChangeRoleInput{ID: fixtures.AcmeAdminMembershipID, Role: membership.RoleMember})
	require.NoError(t, err)

	_, err = imports.Work(ctx, acme, contactimports.WorkInput{ImportID: id})
	require.NoError(t, err)
	ended, err := imports.Get(ctx, acme, id)
	require.NoError(t, err)

	assert.Equal(t, contactimport.StateCancelled, ended.Import.State)
	assert.Equal(t, 1000, ended.Import.Processed)
	assert.Zero(t, countByEmail(t, env, "r1001@example.com"))
}

// A second import of the Workspace is not run while one runs.
func TestASecondImportIsNotRunWhileOneRuns(t *testing.T) {
	env := testhelper.Setup(t)
	runningImport(t, env, emailFile(1, func(int) string { return "first@example.com" }), fixtures.AcmeOwnerMembershipID)
	imports := testhelper.Module[*contactimports.Module](env)
	acme := env.DB.Scoped(fixtures.AcmeID)
	uploaded, err := imports.Upload(t.Context(), acme, contactimports.UploadInput{File: strings.NewReader("email\nsecond@example.com\n"), MembershipID: fixtures.AcmeOwnerMembershipID})
	require.NoError(t, err)
	_, err = imports.SaveMapping(t.Context(), acme, uploaded.Import.ID, contactimports.MappingInput{Mapping: csvimport.Mapping{
		Columns:          []csvimport.Column{{Kind: csvimport.KindKey, Key: fieldname.Email}},
		DecimalSeparator: csvimport.DecimalPoint,
	}})
	require.NoError(t, err)

	_, err = imports.Run(t.Context(), acme, uploaded.Import.ID, membership.RoleOwner)

	require.ErrorIs(t, err, contactimports.ErrAlreadyRunning)
}

// countByEmail is how many Contacts of Acme have address.
func countByEmail(t *testing.T, env *testhelper.TestEnv, address string) int {
	t.Helper()
	n, err := env.DB.Scoped(fixtures.AcmeID).Contact().Query().Where(contact.Email(address)).Count(t.Context())
	require.NoError(t, err)
	return n
}

// A batch that fails every try ends the import failed once its job's tries are spent: what it
// wrote before stays, its file is gone and its report downloads.
func TestARunWhoseBatchKeepsFailingEndsFailedAfterItsTries(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	imports := testhelper.Module[*contactimports.Module](env)
	file := emailFile(1500, func(row int) string {
		if row == 10 {
			return `""`
		}
		return fmt.Sprintf("r%d@example.com", row)
	})
	id := runningImport(t, env, file, fixtures.AcmeOwnerMembershipID)
	env.Bus.Use(func(next entgo.Mutator) entgo.Mutator {
		return entgo.MutateFunc(func(ctx context.Context, m entgo.Mutation) (entgo.Value, error) {
			if cm, ok := m.(*ent.ContactMutation); ok {
				if email, set := cm.Email(); set && email == "r1200@example.com" {
					return nil, errors.New("injected fault")
				}
			}
			return next.Mutate(ctx, m)
		})
	})

	// Each try fails; river tries the job again until its tries are spent.
	for range contactimports.RunAttempts {
		env.TryPerformEnqueuedJobs(t, contactimports.RunArgs{})
		env.RetryJobs(t, contactimports.RunArgs{})
	}
	ended, err := imports.Get(ctx, acme, id)
	require.NoError(t, err)
	chunks, err := acme.ContactImportChunk().Query().Where(contactimportchunk.ContactImportID(id)).Count(ctx)
	require.NoError(t, err)
	_, linkErr := imports.ReportLink(ctx, acme, id, "a-session")

	assert.Equal(t, contactimport.StateFailed, ended.Import.State)
	assert.Equal(t, 1000, ended.Import.Processed)
	assert.Equal(t, 999, ended.Import.Created)
	assert.Zero(t, chunks, "the file is deleted")
	require.NoError(t, linkErr, "the report downloads")
	assert.Equal(t, 1, countByEmail(t, env, "r1000@example.com"))
}

// A refusal is never retried (the job runner ends a refused job), so a try that a refusal stops
// fails the import at once instead of leaving it running.
func TestARunStoppedByARefusalEndsFailedAtOnce(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	id := runningImport(t, env, emailFile(3, func(row int) string { return fmt.Sprintf("r%d@example.com", row) }), fixtures.AcmeOwnerMembershipID)
	refusal := validation.Conflict("injected refusal", i18n.ErrorsContactImportNotRunning)
	env.Bus.Use(func(next entgo.Mutator) entgo.Mutator {
		return entgo.MutateFunc(func(ctx context.Context, m entgo.Mutation) (entgo.Value, error) {
			if cm, ok := m.(*ent.ContactImportMutation); ok {
				if _, set := cm.CheckpointRow(); set {
					return nil, refusal
				}
			}
			return next.Mutate(ctx, m)
		})
	})

	_, err := testhelper.Module[*contactimports.Module](env).Work(ctx, acme, contactimports.WorkInput{ImportID: id})
	ended, getErr := testhelper.Module[*contactimports.Module](env).Get(ctx, acme, id)
	require.NoError(t, getErr)

	require.ErrorIs(t, err, refusal)
	assert.Equal(t, contactimport.StateFailed, ended.Import.State)
}
