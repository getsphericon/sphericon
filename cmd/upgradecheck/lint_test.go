package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lint writes the migrations (name -> SQL) to a temp dir and runs the real squawk over them.
func lint(t *testing.T, migrations map[string]string) []Finding {
	t.Helper()
	return lintWithDown(t, migrations, "")
}

// lintWithDown is lint with the same Down section under every migration.
func lintWithDown(t *testing.T, migrations map[string]string, down string) []Finding {
	t.Helper()
	dir := t.TempDir()
	var paths []string
	for name, sql := range migrations {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte("-- +goose Up\n"+sql+"\n-- +goose Down\n"+down), 0o600))
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
	// squawk warns about the 32-bit integer and the missing lock_timeout.
	findings := lint(t, map[string]string{"20260105000000_advice.sql": `
ALTER TABLE t ADD COLUMN IF NOT EXISTS a int NULL;`})

	assert.Empty(t, findings)
	require.NoError(t, CheckMigrations("feat: a", findings))
}

func TestEveryBlockingRuleFailsTheCheckUnderAnyTitle(t *testing.T) {
	for _, tc := range []struct{ rule, sql string }{
		{"require-concurrent-index-creation", "CREATE INDEX IF NOT EXISTS t_a ON t (a);"},
		{"require-concurrent-index-deletion", "DROP INDEX IF EXISTS t_a;"},
		{"require-concurrent-reindex", "REINDEX TABLE t;"},
		{"disallowed-unique-constraint", "ALTER TABLE t ADD CONSTRAINT t_u UNIQUE (a);"},
		{"adding-foreign-key-constraint", "ALTER TABLE t ADD CONSTRAINT t_f FOREIGN KEY (a) REFERENCES o (id);"},
		{"constraint-missing-not-valid", "ALTER TABLE t ADD CONSTRAINT t_c CHECK (a > 0);"},
		{"adding-not-nullable-field", "ALTER TABLE t ALTER COLUMN a SET NOT NULL;"},
		{"adding-field-with-default", "ALTER TABLE t ADD COLUMN IF NOT EXISTS c double precision NOT NULL DEFAULT random();"},
		{"adding-serial-primary-key-field", "ALTER TABLE t ADD PRIMARY KEY (id);"},
	} {
		t.Run(tc.rule, func(t *testing.T) {
			findings := lint(t, map[string]string{"20260109000000_lock.sql": tc.sql})

			codes := make([]string, 0, len(findings))
			for _, f := range findings {
				codes = append(codes, f.Code)
			}
			assert.Contains(t, codes, tc.rule)
			require.ErrorContains(t, CheckMigrations("feat: a", findings), "20260109000000_lock.sql")
			require.ErrorContains(t, CheckMigrations("feat!: a", findings), "20260109000000_lock.sql")
		})
	}
}

func TestIndexBuiltConcurrentlyWithoutATransactionPasses(t *testing.T) {
	findings := lint(t, map[string]string{"20260110000000_concurrently.sql": `-- +goose NO TRANSACTION
CREATE INDEX CONCURRENTLY IF NOT EXISTS t_a ON t (a);`})

	assert.Empty(t, findings)
}

func TestIndexAndConstraintsOnATableTheMigrationCreatesPass(t *testing.T) {
	findings := lint(t, map[string]string{"20260111000000_new_table.sql": `
CREATE TABLE IF NOT EXISTS w (id bigint NOT NULL, a bigint NULL);
CREATE INDEX IF NOT EXISTS w_a ON w (a);
ALTER TABLE w ADD CONSTRAINT w_f FOREIGN KEY (a) REFERENCES o (id);
ALTER TABLE w ADD CONSTRAINT w_u UNIQUE (a);`})

	assert.Empty(t, findings)
}

func TestSquawkIgnoreAllowsOneCheckedStatement(t *testing.T) {
	findings := lint(t, map[string]string{"20260112000000_ignore.sql": `
-- squawk-ignore require-concurrent-index-creation
CREATE INDEX IF NOT EXISTS t_a ON t (a);
CREATE INDEX IF NOT EXISTS t_b ON t (b);`})

	require.Len(t, findings, 1)
	assert.Equal(t, Blocking, findings[0].Class)
}

func TestABreakingTitleClearsTheDestructiveFindingButNotTheBlockingOne(t *testing.T) {
	findings := lint(t, map[string]string{"20260113000000_both.sql": `
CREATE INDEX IF NOT EXISTS t_a ON t (a);
ALTER TABLE t DROP COLUMN IF EXISTS b;`})

	err := CheckMigrations("feat!: a", findings)

	require.ErrorContains(t, err, "require-concurrent-index-creation")
	assert.NotContains(t, err.Error(), "ban-drop-column")
}

func TestGeneratedRiverMigrationsAreSkipped(t *testing.T) {
	findings := lint(t, map[string]string{"20260101000000_river_001.sql": "DROP TABLE IF EXISTS river_job;"})

	assert.Empty(t, findings)
}

func TestFindingsRejectsGarbage(t *testing.T) {
	_, err := Findings([]byte("not json"))

	require.Error(t, err)
}

func TestOnlyTheUpSectionIsLinted(t *testing.T) {
	// The generator writes a Down that reverses the Up: a safe additive Up has a DROP COLUMN Down.
	safeUp := map[string]string{"20260106000000_add.sql": "ALTER TABLE t ADD COLUMN IF NOT EXISTS a bigint NULL;"}
	destructiveUp := map[string]string{"20260107000000_drop.sql": "ALTER TABLE t DROP COLUMN IF EXISTS a;"}

	assert.Empty(t, lintWithDown(t, safeUp, "ALTER TABLE t DROP COLUMN IF EXISTS a;"))
	assert.NotEmpty(t, lintWithDown(t, destructiveUp, ""))
	assert.NotEmpty(t, lintWithDown(t, destructiveUp, "ALTER TABLE t ADD COLUMN IF NOT EXISTS a bigint NULL;"))
}

func TestFindingsKeepTheMigrationFileName(t *testing.T) {
	findings := lintWithDown(t, map[string]string{"20260108000000_drop.sql": "DROP TABLE IF EXISTS t;"}, "SELECT 1;")

	require.Len(t, findings, 1)
	assert.Equal(t, "20260108000000_drop.sql", findings[0].File)
}
