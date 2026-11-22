package db_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The project carries no legacy (ADR 0047): a database holding an Automation version whose Steps
// have no ids is refused before a column goes, rather than losing its Enrollments' places.
func TestTheStepTreeMigrationRefusesADatabaseWithAutomations(t *testing.T) {
	sqlDB := testhelper.SchemaBefore(t, "automation_step_tree", filepath.Join("testdata", "automation_step_tree"))

	_, err := db.Migrate(t.Context(), sqlDB)

	require.ErrorContains(t, err, "reset it, the project carries no legacy")
}
