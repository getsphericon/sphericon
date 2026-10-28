// Command seed loads the committed fixtures into the development database.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/go-testfixtures/testfixtures/v3"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/cleanup"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/secrets"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
	log.Println("Fixtures loaded successfully")
}

func run() error {
	cfg, err := config.Load("development")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	sqlDB, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer cleanup.Close(sqlDB, "database")

	cipher, err := secrets.NewCipher(cfg.EncryptionKey)
	if err != nil {
		return fmt.Errorf("init cipher: %w", err)
	}

	loader, err := testfixtures.New(
		testfixtures.Database(sqlDB),
		testfixtures.Dialect("postgres"),
		testfixtures.DangerousSkipTestDatabaseCheck(),
		// A date the YAML writes without a zone is UTC, as in the test loader.
		testfixtures.Location(time.UTC),
		// Template options MUST precede Directory (it renders files eagerly). Same
		// templating + sequence handling as the test loader (internal/testhelper)
		// so dev and test load identical fixtures.
		testfixtures.Template(),
		testfixtures.TemplateFuncs(fixtures.TemplateFuncs(cipher, time.Now())),
		testfixtures.Directory("fixtures"),
		testfixtures.ResetSequencesTo(100000),
	)
	if err != nil {
		return fmt.Errorf("create fixtures loader: %w", err)
	}

	if err := loader.Load(); err != nil {
		return fmt.Errorf("load fixtures: %w", err)
	}
	// raw sql: sequences are database state, which ent has no API for
	if _, err := sqlDB.ExecContext(context.Background(), keepSequencesAheadOfRows); err != nil {
		return fmt.Errorf("move sequences past existing rows: %w", err)
	}
	return nil
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
		EXECUTE format('SELECT setval(%L, GREATEST(100000, COALESCE((SELECT MAX(%I) FROM %I), 0)))', r.seq, r.col, r.tbl);
	END LOOP;
END $$`
