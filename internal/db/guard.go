package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/buildinfo"
	"github.com/jackc/pgerrcode"
	"golang.org/x/mod/semver"
)

// UpgradeGuideURL is where every refusal sends the reader.
const UpgradeGuideURL = "https://getsphericon.com/docs/operations/upgrading"

// Major is one Major of the schema (ADR 0033): its number, a counter that starts at 1 and grows
// by one with each destructive migration, and its Floor, the last release of the previous
// Major, which is the oldest release that still runs on this schema.
type Major struct {
	Major int
	Floor string
}

// Majors lists every Major this binary knows, oldest first; the last is the Major of its own
// schema. A destructive migration adds an entry here and writes the same pair to the state
// row (a test compares the two). `sphericon upgrade` walks it: the last release of Major N is
// the Floor of Major N+1.
var Majors = []Major{
	{Major: 1, Floor: "v0.0.0"},
	// workspaces.ingest_key dropped: every Integration has its own hook key (ADR 0037).
	{Major: 2, Floor: "v0.1.2"},
}

// SchemaState is the state row: the database's Major and the oldest release that still runs
// on it. Only a destructive migration changes it.
type SchemaState struct {
	Major int
	Floor string
}

// initialState is what a database from before the row holds, in effect: Major 1, any release.
var initialState = SchemaState{Major: 1, Floor: "v0.0.0"}

// ReadState reads the state row. A database with no row yet (empty, or migrated before the row
// existed) is Major 1.
func ReadState(ctx context.Context, sqlDB *sql.DB) (SchemaState, error) {
	row, err := NewEntClient(sqlDB).SchemaState.Get(ctx, 1)
	switch {
	case err == nil:
		return SchemaState{Major: row.Major, Floor: row.Floor}, nil
	case ent.IsNotFound(err) || isUndefinedTable(err):
		return initialState, nil
	default:
		return SchemaState{}, fmt.Errorf("read schema state: %w", err)
	}
}

// RefusalError is a start that the guard stops; its message names the remedy.
type RefusalError struct {
	message string
}

func (r *RefusalError) Error() string { return r.message }

// Guard decides at start whether this binary may run on the database (ADR 0033).
type Guard struct {
	Build buildinfo.Info
	// AutoMigrate says a binary one Major ahead will migrate before it serves.
	AutoMigrate bool
	// Majors is the binary's table; nil means the package's Majors.
	Majors []Major
}

// Check reads the state row and refuses (with a *RefusalError) when the binary cannot run:
//   - it is older than the floor (a rollback past a removal), whatever the Major;
//   - the database is two or more Majors behind it (a skipped Major): install the last
//     release of the database's Major first;
//   - the database is one Major behind and AutoMigrate is off: run `sphericon migrate`.
//
// A binary of the database's Major, or one Major behind it but not older than the floor (the
// last release of a Major), starts. A build with no release number is never older than a floor.
func (g Guard) Check(ctx context.Context, sqlDB *sql.DB) error {
	state, err := ReadState(ctx, sqlDB)
	if err != nil {
		return err
	}
	majors := g.Majors
	if majors == nil {
		majors = Majors
	}
	own := majors[len(majors)-1].Major
	who := fmt.Sprintf("sphericon %s (commit %s, built %s)", g.Build.Version, g.Build.Commit, g.Build.Date)
	tail := " Upgrade guide: " + UpgradeGuideURL

	if semver.IsValid(g.Build.Version) && semver.IsValid(state.Floor) && semver.Compare(g.Build.Version, state.Floor) < 0 {
		return &RefusalError{message: fmt.Sprintf(
			"%s is older than the database allows: its schema is Major %d and needs %s or newer.%s",
			who, state.Major, state.Floor, tail)}
	}
	switch behind := own - state.Major; {
	case behind >= 2:
		needs := ""
		for _, m := range majors {
			if m.Major == state.Major+1 {
				needs = m.Floor
			}
		}
		return &RefusalError{message: fmt.Sprintf(
			"%s skipped a Major: the database is at Major %d and this binary at Major %d; install %s first, then upgrade again.%s",
			who, state.Major, own, needs, tail)}
	case behind == 1 && !g.AutoMigrate:
		return &RefusalError{message: fmt.Sprintf(
			"%s is one Major ahead of the database (Major %d, binary Major %d): run `sphericon migrate`, or set SPHERICON_DB_AUTO_MIGRATE=true.%s",
			who, state.Major, own, tail)}
	}
	if !g.AutoMigrate {
		pending, err := pendingMigrations(ctx, sqlDB)
		if err != nil {
			return err
		}
		if pending > 0 {
			return &RefusalError{message: fmt.Sprintf(
				"%s needs %d migration(s) the database has not run: run `sphericon migrate` (in development `mise run db:migrate`), or set SPHERICON_DB_AUTO_MIGRATE=true.%s",
				who, pending, tail)}
		}
	}
	return nil
}

func pendingMigrations(ctx context.Context, sqlDB *sql.DB) (int, error) {
	st, err := Status(ctx, sqlDB)
	if err != nil {
		return 0, fmt.Errorf("read pending migrations: %w", err)
	}
	return len(st.Pending), nil
}

func isUndefinedTable(err error) bool {
	var pgErr interface{ SQLState() string }
	return errors.As(err, &pgErr) && pgErr.SQLState() == pgerrcode.UndefinedTable
}
