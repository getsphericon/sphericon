package fixtures

import (
	"context"
	"database/sql"
	"time"

	"github.com/go-testfixtures/testfixtures/v3"

	"github.com/getsphericon/sphericon/internal/kernel/secrets"
)

// Load replaces the contents of every table the YAML files of dir name with their rows, through
// the fixture library and its templating (relative dates, load-time encryption with cipher). The
// sequences are reset to 100000 afterwards, so the explicit fixture ids stay clear of the rows an
// app inserts.
//
// "Now" is the database's clock, not Go's: the first test of a package may run inside a
// testing/synctest bubble, where time.Now() starts in the year 2000, and every fixture loaded
// there would be dated around it.
func Load(ctx context.Context, sqlDB *sql.DB, cipher *secrets.Cipher, dir string) error {
	return load(ctx, sqlDB, cipher, dir)
}

// Seed is Load into a database that is not a test one (the development database), which the
// fixture library refuses otherwise. A running dev stack keeps rows in tables the fixtures do not
// own, so every sequence is then moved past the highest id of its table.
func Seed(ctx context.Context, sqlDB *sql.DB, cipher *secrets.Cipher, dir string) error {
	if err := load(ctx, sqlDB, cipher, dir, testfixtures.DangerousSkipTestDatabaseCheck()); err != nil {
		return err
	}
	// raw sql: sequences are database state, which ent has no API for
	_, err := sqlDB.ExecContext(ctx, keepSequencesAheadOfRows)
	return err
}

// load is Load with the extra options of the fixture library opts.
func load(ctx context.Context, sqlDB *sql.DB, cipher *secrets.Cipher, dir string, opts ...func(*testfixtures.Loader) error) error {
	var now time.Time
	// raw sql: the database's clock as a bare value, read before any fixture exists.
	if err := sqlDB.QueryRowContext(ctx, `SELECT now()`).Scan(&now); err != nil {
		return err
	}

	loader, err := testfixtures.New(append([]func(*testfixtures.Loader) error{
		testfixtures.Database(sqlDB),
		testfixtures.Dialect("postgres"),
		// A date the YAML writes without a zone is UTC, whatever the machine's zone: the
		// library parses it in time.Local otherwise, a shift of the machine's UTC offset.
		testfixtures.Location(time.UTC),
		// Template options MUST precede Directory: Directory eagerly reads and
		// renders the files when its option is applied.
		testfixtures.Template(),
		testfixtures.TemplateFuncs(TemplateFuncs(cipher, now)),
		testfixtures.Directory(dir),
		testfixtures.ResetSequencesTo(100000),
	}, opts...)...)
	if err != nil {
		return err
	}
	return loader.Load()
}

// keepSequencesAheadOfRows undoes what ResetSequencesTo does to a table the fixtures do not own, such as
// River's job queue: a running dev stack has rows there that survive the load, and a sequence set back
// to 100000 would hand out ids they already hold ("duplicate key river_job_pkey"), so no periodic job
// could be enqueued. Every sequence ends up at the larger of 100000 and the highest id in its table.
const keepSequencesAheadOfRows = `DO $$
DECLARE r record;
BEGIN
	FOR r IN
		SELECT t.relname AS tbl, a.attname AS col, s.relname AS seq
		FROM pg_class s
		JOIN pg_depend d ON d.objid = s.oid AND d.deptype IN ('a', 'i')
		JOIN pg_class t ON t.oid = d.refobjid
		JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = d.refobjsubid
		WHERE s.relkind = 'S'
	LOOP
		EXECUTE format('SELECT setval(%L, GREATEST(100000, (SELECT MAX(%I) FROM %I)))', r.seq, r.col, r.tbl);
	END LOOP;
END $$`
