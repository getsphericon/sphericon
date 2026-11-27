package fixtures

import (
	"context"
	"database/sql"
	"time"

	"github.com/go-testfixtures/testfixtures/v3"

	"github.com/getsphericon/sphericon/internal/kernel/secrets"
)

// Load replaces the contents of every table the YAML files of dir name with their rows, through
// the fixture library and its templating (relative dates, load-time encryption with cipher). Every
// sequence is then moved to the larger of 100000 and the highest id of its table, so the explicit
// fixture ids stay clear of the rows an app inserts, and so do the rows that survive the load in a
// table the fixtures do not own (keepSequencesAheadOfRows).
//
// The rows are dated from the database's clock (DatabaseNow), read when Load starts.
func Load(ctx context.Context, sqlDB *sql.DB, cipher *secrets.Cipher, dir string) error {
	now, err := DatabaseNow(ctx, sqlDB)
	if err != nil {
		return err
	}
	return load(ctx, sqlDB, cipher, dir, now)
}

// LoadAt is Load with the rows dated from now: a caller that loads more than one database names
// one instant for all of them, so a row's calendar day is the same in each, and a test can read
// the day a fixture fell on from that instant, not from a clock that may have passed midnight
// since the load.
func LoadAt(ctx context.Context, sqlDB *sql.DB, cipher *secrets.Cipher, dir string, now time.Time) error {
	return load(ctx, sqlDB, cipher, dir, now)
}

// DatabaseNow is the database's clock, which dates the fixtures, not Go's: the first test of a
// package may run inside a testing/synctest bubble, where time.Now() starts in the year 2000, and
// every fixture loaded there would be dated around it.
func DatabaseNow(ctx context.Context, sqlDB *sql.DB) (time.Time, error) {
	var now time.Time
	// raw sql: the database's clock as a bare value, read before any fixture exists.
	if err := sqlDB.QueryRowContext(ctx, `SELECT now()`).Scan(&now); err != nil {
		return time.Time{}, err
	}
	return now, nil
}

// Seed is Load into a database that is not a test one (the development database), which the
// fixture library refuses otherwise.
func Seed(ctx context.Context, sqlDB *sql.DB, cipher *secrets.Cipher, dir string) error {
	now, err := DatabaseNow(ctx, sqlDB)
	if err != nil {
		return err
	}
	return load(ctx, sqlDB, cipher, dir, now, testfixtures.DangerousSkipTestDatabaseCheck())
}

// load is LoadAt with the extra options of the fixture library opts.
func load(ctx context.Context, sqlDB *sql.DB, cipher *secrets.Cipher, dir string, now time.Time, opts ...func(*testfixtures.Loader) error) error {
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
	if err := loader.Load(); err != nil {
		return err
	}
	// raw sql: sequences are database state, which ent has no API for
	_, err = sqlDB.ExecContext(ctx, keepSequencesAheadOfRows)
	return err
}

// keepSequencesAheadOfRows undoes what ResetSequencesTo does to a table the fixtures do not own, such as
// River's job queue: a running dev stack, or a test that committed its jobs and died before its
// cleanup, leaves rows there that survive the load, and a sequence set back to 100000 would hand out
// ids they already hold ("duplicate key river_job_pkey"). Every sequence ends up at the larger of
// 100000 and the highest id in its table.
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
