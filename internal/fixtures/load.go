package fixtures

import (
	"context"
	"database/sql"
	"time"

	"github.com/go-testfixtures/testfixtures/v3"

	"github.com/getsphericon/sphericon/internal/secrets"
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
	var now time.Time
	// raw sql: the database's clock as a bare value, read before any fixture exists.
	if err := sqlDB.QueryRowContext(ctx, `SELECT now()`).Scan(&now); err != nil {
		return err
	}

	loader, err := testfixtures.New(
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
	)
	if err != nil {
		return err
	}
	return loader.Load()
}
