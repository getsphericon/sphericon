package db_test

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// storedStepsPath is a version's steps as the migration leaves them, read as plain JSON so the
// test reads the stored shape, not the fields of a Go type.
type storedStepsPath struct {
	ID    string           `json:"id"`
	Steps []map[string]any `json:"steps"`
}

func stepsPathOf(t *testing.T, sqlDB *sql.DB, versionID int64) storedStepsPath {
	t.Helper()
	v, err := entclient.Core.New(sqlDB).AutomationVersion.Get(t.Context(), versionID)
	require.NoError(t, err)
	var p storedStepsPath
	require.NoError(t, json.Unmarshal([]byte(testhelper.Document(t, v.Steps)), &p))
	return p
}

func idsOf(p storedStepsPath) []string {
	ids := make([]string, len(p.Steps))
	for i, st := range p.Steps {
		ids[i], _ = st["id"].(string)
	}
	return ids
}

func isUUID(id string) bool {
	parsed, err := uuid.Parse(id)
	return err == nil && parsed.String() == id
}

// Every stored version's steps are rewritten as a root path holding the old array (ADR 0122):
// the path and every Step get a UUID, the same one in every version of the Automation for the same
// position, and an HTTP step keeps its id, its ULID written as the UUID of the same 128 bits. The
// rest of each Step is left as it was.
func TestTheStepsPathMigrationRewritesEachVersionAsARootPathWithIDs(t *testing.T) {
	sqlDB := testhelper.SchemaBefore(t, "steps_path_default", filepath.Join("testdata", "steps_path"))

	_, err := db.Migrate(t.Context(), sqlDB)
	require.NoError(t, err)

	live, draft, empty := stepsPathOf(t, sqlDB, 1), stepsPathOf(t, sqlDB, 2), stepsPathOf(t, sqlDB, 3)
	assert.True(t, isUUID(live.ID), "the root path gets a UUID: %q", live.ID)
	assert.Equal(t, live.ID, draft.ID, "every version of an Automation has the same root path")
	assert.NotEqual(t, live.ID, empty.ID, "each Automation has its own root path")
	assert.Empty(t, empty.Steps)
	require.Len(t, live.Steps, 3)
	require.Len(t, draft.Steps, 4)
	liveIDs, draftIDs := idsOf(live), idsOf(draft)
	for _, id := range append(liveIDs, draftIDs...) {
		assert.True(t, isUUID(id), "every Step gets a UUID: %q", id)
	}
	assert.Equal(t, liveIDs[:2], draftIDs[:2], "a position keeps its id across versions")
	assert.Equal(t, "01900000-0000-0000-0000-000000000001", liveIDs[2], "an HTTP step keeps its id")
	assert.Equal(t, "01900000-0000-0000-0000-000000000001", draftIDs[3], "an HTTP step keeps its id where it moved")
	assert.Len(t, map[string]bool{draftIDs[0]: true, draftIDs[1]: true, draftIDs[2]: true, draftIDs[3]: true, draft.ID: true}, 5,
		"no two ids of a version are the same")
	assert.Equal(t, map[string]any{"id": liveIDs[0], "type": "email", "subject": "Hi", "body": "<p>Hi</p>"}, live.Steps[0])
	assert.Equal(t, map[string]any{"id": draftIDs[2], "type": "apply_tag", "tag": "vip"}, draft.Steps[2])
}

// Running the rewrite again over what it wrote changes nothing (ADR 0033: a Go migration is
// idempotent, so a re-run after an interrupted Upgrade finishes it).
func TestTheStepsPathRewriteRunAgainChangesNothing(t *testing.T) {
	sqlDB := testhelper.SchemaBefore(t, "steps_path_default", filepath.Join("testdata", "steps_path"))
	ctx := t.Context()
	_, err := db.Migrate(ctx, sqlDB)
	require.NoError(t, err)
	first := []storedStepsPath{stepsPathOf(t, sqlDB, 1), stepsPathOf(t, sqlDB, 2), stepsPathOf(t, sqlDB, 3)}

	tx, err := sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	require.NoError(t, db.RewriteStepsPaths(ctx, tx))
	require.NoError(t, tx.Commit())

	second := []storedStepsPath{stepsPathOf(t, sqlDB, 1), stepsPathOf(t, sqlDB, 2), stepsPathOf(t, sqlDB, 3)}
	assert.Equal(t, first, second)
}
