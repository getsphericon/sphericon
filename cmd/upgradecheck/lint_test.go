package main

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lint writes the migrations (name -> SQL) to a temp dir and runs the real squawk over them.
func lint(t *testing.T, migrations map[string]string) []Finding {
	t.Helper()
	dir := t.TempDir()
	var paths []string
	for name, sql := range migrations {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte("-- +goose Up\n"+sql+"\n-- +goose Down\n"), 0o600))
		paths = append(paths, path)
	}
	findings, err := LintFiles(paths)
	require.NoError(t, err)
	return findings
}

func TestEveryDestructiveRuleFailsTheCheckUnderAPlainTitle(t *testing.T) {
	for _, tc := range []struct{ rule, sql string }{
		{"ban-drop-column", "ALTER TABLE t DROP COLUMN IF EXISTS a;"},
		{"ban-drop-table", "DROP TABLE IF EXISTS t;"},
		{"renaming-column", "ALTER TABLE t RENAME COLUMN a TO b;"},
		{"renaming-table", "ALTER TABLE t RENAME TO u;"},
		{"changing-column-type", "ALTER TABLE t ALTER COLUMN a TYPE bigint;"},
		{"adding-required-field", "ALTER TABLE t ADD COLUMN IF NOT EXISTS c bigint NOT NULL;"},
	} {
		t.Run(tc.rule, func(t *testing.T) {
			findings := lint(t, map[string]string{"20260103000000_change.sql": tc.sql})

			codes := make([]string, 0, len(findings))
			for _, f := range findings {
				codes = append(codes, f.Code)
			}
			assert.Contains(t, codes, tc.rule)
			require.ErrorContains(t, CheckMigrations("feat: a", findings), "20260103000000_change.sql")
			require.NoError(t, CheckMigrations("feat!: a", findings))
		})
	}
}

func TestDestructiveRuleTableCoversTheSpec(t *testing.T) {
	assert.ElementsMatch(t, []string{
		"ban-drop-column", "ban-drop-table", "renaming-column", "renaming-table", "changing-column-type", "adding-required-field",
	}, slices.Collect(maps.Keys(DestructiveRules)))
}

func TestSafeMigrationsPass(t *testing.T) {
	findings := lint(t, map[string]string{"20260104000000_safe.sql": `
CREATE TABLE IF NOT EXISTS t (id bigint NOT NULL);
ALTER TABLE t ADD COLUMN IF NOT EXISTS a bigint NULL;
ALTER TABLE t ADD COLUMN IF NOT EXISTS b bigint NOT NULL DEFAULT 1;
CREATE INDEX CONCURRENTLY IF NOT EXISTS t_a ON t (a);`})

	assert.Empty(t, findings)
	require.NoError(t, CheckMigrations("feat: a", findings))
}

func TestAdviceOnlyFindingsNeverFail(t *testing.T) {
	// squawk warns about the 32-bit integer, the missing lock_timeout and the blocking index.
	findings := lint(t, map[string]string{"20260105000000_advice.sql": `
ALTER TABLE t ADD COLUMN IF NOT EXISTS a int NULL;
CREATE INDEX IF NOT EXISTS t_a ON t (a);`})

	assert.Empty(t, findings)
	require.NoError(t, CheckMigrations("feat: a", findings))
}

func TestGeneratedRiverMigrationsAreSkipped(t *testing.T) {
	findings := lint(t, map[string]string{"20260101000000_river_001.sql": "DROP TABLE IF EXISTS river_job;"})

	assert.Empty(t, findings)
}

func TestDestructiveRejectsGarbage(t *testing.T) {
	_, err := Destructive([]byte("not json"))

	require.Error(t, err)
}
