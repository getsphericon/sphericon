package contactimports_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/contactimport"
	"github.com/getsphericon/sphericon/ent/contactimportchunk"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contactimports"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/csvimport"
	"github.com/getsphericon/sphericon/internal/kernel/fieldname"
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
	_, err = imports.Run(ctx, acme, uploaded.Import.ID)
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
