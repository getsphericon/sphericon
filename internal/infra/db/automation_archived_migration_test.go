package db_test

import (
	"path/filepath"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/auditentry"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An Automation is archived, never discarded (ADR 0100): the migration keeps an archived
// Automation archived, and the Audit log reads archive and unarchive for what it recorded as
// discard and restore. Another action, and another kind's discard, keep their names.
func TestTheAutomationArchiveMigrationKeepsTheStampAndRenamesTheAuditActions(t *testing.T) {
	sqlDB := testhelper.SchemaBefore(t, "automation_archived", filepath.Join("testdata", "automation_archived"))

	_, err := db.Migrate(t.Context(), sqlDB)
	require.NoError(t, err)
	client := entclient.New(sqlDB)
	archived, err := client.Automation.Query().Where(automation.Name("Archived nudge")).Only(t.Context())
	require.NoError(t, err)
	kept, err := client.Automation.Query().Where(automation.Name("Kept welcome")).Only(t.Context())
	require.NoError(t, err)
	entries, err := client.AuditEntry.Query().Order(ent.Asc(auditentry.FieldOccurredAt)).All(t.Context())
	require.NoError(t, err)

	assert.NotNil(t, archived.ArchivedAt)
	assert.Nil(t, kept.ArchivedAt)
	actions := lo.Map(entries, func(e *ent.AuditEntry, _ int) string { return e.Action })
	assert.Equal(t, []string{"automation.archive", "automation.unarchive", "automation.update", "rule.discard"}, actions)
}
