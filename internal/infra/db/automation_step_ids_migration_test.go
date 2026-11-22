package db_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// migratedMessage is what the migration leaves of an Automation message: its key, version and Step.
type migratedMessage struct {
	ID        int64
	Key       string
	VersionID *int64
	StepID    *string
}

// An Automation message gets the version and the Step id that sent it through its run (ADR 0122):
// the Step at its position in the run's version, and its key names the Step by that id. A message
// without a run, or at a position its version does not have, names no version and goes, and so do
// the pruned figures, which name none either. Other messages stay as they were.
func TestTheAutomationStepIDsMigrationNamesEachMessagesVersionAndStep(t *testing.T) {
	sqlDB := testhelper.SchemaBefore(t, "automation_step_ids", filepath.Join("testdata", "automation_step_ids"))
	ctx := t.Context()

	_, err := db.Migrate(ctx, sqlDB)
	require.NoError(t, err)

	client := entclient.Core.New(sqlDB)
	rows, err := client.OutboundMessage.Query().Order(ent.Asc(outboundmessage.FieldID)).All(ctx)
	require.NoError(t, err)
	messages := make([]migratedMessage, len(rows))
	for i, m := range rows {
		messages[i] = migratedMessage{m.ID, m.IdempotencyKey, m.AutomationVersionID, m.AutomationStepID}
	}
	lastEmail := "aaaaaaaa-0000-4000-8000-000000000003"
	assert.Equal(t, []migratedMessage{
		{1, "automation:1:" + lastEmail, new(int64(1)), new(lastEmail)},
		{2, "automation:2:" + lastEmail, new(int64(2)), new(lastEmail)},
		{5, "transactional:k1", nil, nil},
	}, messages)
	figures, err := client.AutomationFigure.Query().Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, figures, "no pruned figure names its version")
}
