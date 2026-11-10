package db_test

import (
	"path/filepath"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/auditentry"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A Broadcast is archived, never discarded (ADR 0100): the migration keeps an archived Broadcast
// archived, and the Audit log reads archive and unarchive for what it recorded as discard and
// restore, so old and new entries read the same. Another kind's discard keeps its name.
func TestTheBroadcastArchiveMigrationKeepsTheStampAndRenamesTheAuditActions(t *testing.T) {
	sqlDB := testhelper.SchemaBefore(t, "broadcast_archived", filepath.Join("testdata", "broadcast_archived"))

	_, err := db.Migrate(t.Context(), sqlDB)
	require.NoError(t, err)
	client := entclient.Core.New(sqlDB)
	archived, err := client.Broadcast.Query().Where(broadcast.Name("Archived newsletter")).Only(t.Context())
	require.NoError(t, err)
	kept, err := client.Broadcast.Query().Where(broadcast.Name("Kept draft")).Only(t.Context())
	require.NoError(t, err)
	entries, err := client.AuditEntry.Query().Order(ent.Asc(auditentry.FieldOccurredAt)).All(t.Context())
	require.NoError(t, err)

	assert.NotNil(t, archived.ArchivedAt)
	assert.Nil(t, kept.ArchivedAt)
	actions := lo.Map(entries, func(e *ent.AuditEntry, _ int) string { return e.Action })
	assert.Equal(t, []string{"broadcast.archive", "broadcast.unarchive", "integration.discard"}, actions)
}
