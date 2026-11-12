package upgradecheck_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/upgradecheck"
)

// lint writes the migrations (name -> SQL) to a temp dir and runs the real squawk over them.
func lint(t *testing.T, migrations map[string]string) []upgradecheck.Finding {
	t.Helper()
	return lintWithDown(t, migrations, "")
}

// lintWithDown is lint with the same Down section under every migration.
func lintWithDown(t *testing.T, migrations map[string]string, down string) []upgradecheck.Finding {
	t.Helper()
	dir := t.TempDir()
	paths := make([]string, 0, len(migrations))
	for name, sql := range migrations {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte("-- +goose Up\n"+sql+"\n-- +goose Down\n"+down), 0o600))
		paths = append(paths, path)
	}
	findings, err := upgradecheck.LintFiles(t.Context(), paths)
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
			require.ErrorContains(t, upgradecheck.CheckMigrations("feat: a", findings), "20260103000000_change.sql")
			require.NoError(t, upgradecheck.CheckMigrations("feat!: a", findings))
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
	require.NoError(t, upgradecheck.CheckMigrations("feat: a", findings))
}

func TestAdviceOnlyFindingsNeverFail(t *testing.T) {
	// squawk warns about the 32-bit integer and the missing lock_timeout.
	findings := lint(t, map[string]string{"20260105000000_advice.sql": `
ALTER TABLE t ADD COLUMN IF NOT EXISTS a int NULL;`})

	assert.Empty(t, findings)
	require.NoError(t, upgradecheck.CheckMigrations("feat: a", findings))
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
			require.ErrorContains(t, upgradecheck.CheckMigrations("feat: a", findings), "20260109000000_lock.sql")
			require.ErrorContains(t, upgradecheck.CheckMigrations("feat!: a", findings), "20260109000000_lock.sql")
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
	assert.Equal(t, upgradecheck.Blocking, findings[0].Class)
}

func TestABreakingTitleClearsTheDestructiveFindingButNotTheBlockingOne(t *testing.T) {
	findings := lint(t, map[string]string{"20260113000000_both.sql": `
CREATE INDEX IF NOT EXISTS t_a ON t (a);
ALTER TABLE t DROP COLUMN IF EXISTS b;`})

	err := upgradecheck.CheckMigrations("feat!: a", findings)

	require.ErrorContains(t, err, "require-concurrent-index-creation")
	assert.NotContains(t, err.Error(), "ban-drop-column")
}

func TestGeneratedRiverMigrationsAreSkipped(t *testing.T) {
	findings := lint(t, map[string]string{"20260101000000_river_001.sql": "DROP TABLE IF EXISTS river_job;"})

	assert.Empty(t, findings)
}

func TestFindingsRejectsGarbage(t *testing.T) {
	_, err := upgradecheck.Findings([]byte("not json"))

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

func codesOf(findings []upgradecheck.Finding) []string {
	codes := make([]string, 0, len(findings))
	for _, f := range findings {
		codes = append(codes, f.Code)
	}
	return codes
}

// A NO TRANSACTION migration may hold the statements whose lock wait blocks no query.
func TestStatementsWhoseWaitBlocksNoQueryPassWithoutATransaction(t *testing.T) {
	findings := lint(t, map[string]string{"20260114000000_no_tx.sql": `-- +goose NO TRANSACTION
-- Builds the index the clearing reads.
CREATE INDEX CONCURRENTLY IF NOT EXISTS t_a ON t (a);
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS t_b ON t (b);
DROP INDEX CONCURRENTLY IF EXISTS t_c;
REINDEX INDEX CONCURRENTLY t_a;
ALTER TABLE "t" VALIDATE CONSTRAINT "t_check";
UPDATE t SET a = NULL
  WHERE b IS NULL;
DELETE FROM t WHERE a IS NULL;
INSERT INTO t (a) VALUES (1);`})

	assert.NotContains(t, codesOf(findings), upgradecheck.UnboundedWaitRule)
}

// A statement whose lock wait queues the table's queries fails in a NO TRANSACTION migration,
// under any title, and passes in one that runs in a transaction, where the runner bounds the wait.
func TestALockWaitThatQueuesQueriesFailsWithoutATransaction(t *testing.T) {
	for name, sql := range map[string]string{
		"constraint": "ALTER TABLE t ADD CONSTRAINT t_c CHECK (a > 0) NOT VALID;",
		"nullable":   "ALTER TABLE t ALTER COLUMN a DROP NOT NULL;",
		"column":     "ALTER TABLE t ADD COLUMN IF NOT EXISTS b bigint NULL;",
		"block":      "-- +goose StatementBegin\nDO $$ BEGIN PERFORM 1; END $$;\n-- +goose StatementEnd",
	} {
		t.Run(name, func(t *testing.T) {
			findings := lint(t, map[string]string{"20260115000000_no_tx.sql": "-- +goose NO TRANSACTION\n" + sql})

			assert.Contains(t, codesOf(findings), upgradecheck.UnboundedWaitRule)
			require.ErrorContains(t, upgradecheck.CheckMigrations("feat!: a", findings), "20260115000000_no_tx.sql")
			assert.NotContains(t, codesOf(lint(t, map[string]string{"20260115000000_tx.sql": sql})), upgradecheck.UnboundedWaitRule)
		})
	}
}

// A checked statement (on a table no query reads yet) carries the rule's squawk-ignore; the next
// statement is still held to the rule.
func TestSquawkIgnoreAllowsOneStatementWithAnUnboundedWait(t *testing.T) {
	findings := lint(t, map[string]string{"20260116000000_no_tx.sql": `-- +goose NO TRANSACTION
-- squawk-ignore unbounded-lock-wait
ALTER TABLE fresh ADD COLUMN IF NOT EXISTS a bigint NULL;
ALTER TABLE t ADD COLUMN IF NOT EXISTS a bigint NULL;`})

	require.Len(t, findings, 1)
	assert.Equal(t, upgradecheck.UnboundedWaitRule, findings[0].Code)
	assert.Contains(t, findings[0].Text, "ALTER TABLE t ADD COLUMN")
}

// Statements are split as Postgres reads them, so a statement whose wait queues queries cannot
// hide behind another one's head: after a trailing comment, on the same line, or as a second action
// of an ALTER TABLE that validates a constraint. A semicolon or a `--` inside a literal splits
// nothing.
func TestAStatementCannotHideBehindAnotherOnesHead(t *testing.T) {
	for name, sql := range map[string]string{
		"after a trailing comment": "CREATE INDEX CONCURRENTLY t_a ON t (a); -- the clearing reads it\nALTER TABLE t ADD COLUMN b bigint NULL;",
		"on the same line":         "UPDATE t SET a = 1; ALTER TABLE t ALTER COLUMN a DROP NOT NULL;",
		"a second action":          "ALTER TABLE t VALIDATE CONSTRAINT t_c, DROP CONSTRAINT t_d;",
	} {
		t.Run(name, func(t *testing.T) {
			findings := lint(t, map[string]string{"20260117000000_no_tx.sql": "-- +goose NO TRANSACTION\n" + sql})

			assert.Contains(t, codesOf(findings), upgradecheck.UnboundedWaitRule)
		})
	}

	findings := lint(t, map[string]string{"20260118000000_no_tx.sql": `-- +goose NO TRANSACTION
UPDATE t SET a = 'x; -- it''s
ALTER TABLE t DROP NOT NULL';`})
	assert.NotContains(t, codesOf(findings), upgradecheck.UnboundedWaitRule)
}
