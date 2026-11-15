package integrations_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// failingQueue is a job queue whose every insert fails with err.
type failingQueue struct{ err error }

func (q failingQueue) InsertTx(context.Context, *sql.Tx, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	return nil, q.err
}

func (q failingQueue) InsertManyTx(context.Context, *sql.Tx, []river.InsertManyParams) ([]*rivertype.JobInsertResult, error) {
	return nil, q.err
}

func (q failingQueue) JobDeleteManyTx(context.Context, *sql.Tx, *river.JobDeleteManyParams) (*river.JobDeleteManyResult, error) {
	return nil, q.err
}

// A miss of an edit is told at its write (ADR 0119): an edit whose quota lookup, queued in the
// edit's transaction after the write, fails with a not found is that failure, not a revision the
// Integration moved past.
func TestAnEditWhoseQuotaLookupIsNotFoundIsNotAStaleRevision(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.AcmeID)
	_, notFound := env.DB.Integration.Get(ctx, 999999)
	require.True(t, ent.IsNotFound(notFound))
	bus := events.New(env.SQLDB, entclient.Core.NewTx, time.Now)
	bus.UseJobs(failingQueue{err: notFound})
	ses := &testhelper.FakeSES{}
	module := integrations.New(bus, env.Cipher, ses.Catalog(), nil, "", time.Now)
	row := reload(t, s, fixtures.IntegrationAcmeSesID)

	_, err := module.Update(ctx, s, row.ID, integrations.UpdateInput{
		Revision: row.Revision,
		Config: patch.Set(integrations.ConfigInput{SES: &integrations.SESConfig{
			Region: "eu-west-1", AccessKeyID: "AKIA1234", SecretAccessKey: "secret", From: "noreply@acme.test",
		}}),
	})

	require.ErrorIs(t, err, notFound)
	assert.NotErrorIs(t, err, integrations.ErrRevisionChanged)
	assert.Equal(t, row.Revision, reload(t, s, row.ID).Revision, "the edit rolled back")
}
