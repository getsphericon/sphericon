package db_test

import (
	"database/sql"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/kernel/buildinfo"
	"github.com/getsphericon/sphericon/internal/kernel/cleanup"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Three Majors: the last release of Major 1 is v0.4.7, of Major 2 is v0.5.2.
var majors = []db.Major{
	{Major: 1, Floor: "v0.0.0"},
	{Major: 2, Floor: "v0.4.7"},
	{Major: 3, Floor: "v0.5.2"},
}

func migratedAt(t *testing.T, major int, floor string) *sql.DB {
	t.Helper()
	sqlDB := openEmpty(t)
	_, err := db.Migrate(t.Context(), sqlDB)
	require.NoError(t, err)
	require.NoError(t, entclient.Core.New(sqlDB).SchemaState.UpdateOneID(1).SetMajor(major).SetFloor(floor).Exec(t.Context()))
	return sqlDB
}

func guard(version string, autoMigrate bool) db.Guard {
	return db.Guard{
		Build:       buildinfo.Info{Version: version, Commit: "abc1234", Date: "2026-10-18T10:00:00Z"},
		AutoMigrate: autoMigrate,
		Majors:      majors,
	}
}

func heldState(t *testing.T, sqlDB *sql.DB) db.SchemaState {
	t.Helper()
	st, err := db.ReadState(t.Context(), sqlDB)
	require.NoError(t, err)
	return st
}

// The migrations leave the state row, and the binary's table of Majors ends at the Major and the
// floor they write.
func TestMigrateLeavesTheSchemaStateRowTheBinaryExpects(t *testing.T) {
	sqlDB := openEmpty(t)
	_, err := db.Migrate(t.Context(), sqlDB)
	require.NoError(t, err)

	st := heldState(t, sqlDB)
	last := db.Majors[len(db.Majors)-1]
	assert.Equal(t, db.SchemaState(last), st)
}

func TestGuardLetsABinaryOfTheDatabasesMajorStart(t *testing.T) {
	sqlDB := migratedAt(t, 3, "v0.5.2")

	require.NoError(t, guard("v0.6.0", false).Check(t.Context(), sqlDB))
	assert.Equal(t, db.SchemaState{Major: 3, Floor: "v0.5.2"}, heldState(t, sqlDB))
}

func TestGuardRefusesABinaryOlderThanTheFloorAndNamesTheRelease(t *testing.T) {
	sqlDB := migratedAt(t, 3, "v0.5.2")

	err := guard("v0.5.1", false).Check(t.Context(), sqlDB)

	var refusal *db.RefusalError
	require.ErrorAs(t, err, &refusal)
	assert.Contains(t, err.Error(), "sphericon v0.5.1 (commit abc1234, built 2026-10-18T10:00:00Z)")
	assert.Contains(t, err.Error(), "v0.5.2 or newer")
	assert.Contains(t, err.Error(), db.UpgradeGuideURL)
	assert.Equal(t, db.SchemaState{Major: 3, Floor: "v0.5.2"}, heldState(t, sqlDB))
}

// The last release of a Major is the floor of the next one: it runs on the next schema.
func TestGuardLetsTheLastReleaseOfAMajorStartOnTheNextMajorsSchema(t *testing.T) {
	sqlDB := migratedAt(t, 3, "v0.5.2")

	require.NoError(t, guard("v0.5.2", false).Check(t.Context(), sqlDB))
	assert.Equal(t, db.SchemaState{Major: 3, Floor: "v0.5.2"}, heldState(t, sqlDB))
}

func TestGuardRefusesASkippedMajorAndNamesTheReleaseToInstallFirst(t *testing.T) {
	sqlDB := migratedAt(t, 1, "v0.0.0")

	err := guard("v0.6.0", true).Check(t.Context(), sqlDB)

	var refusal *db.RefusalError
	require.ErrorAs(t, err, &refusal)
	assert.Contains(t, err.Error(), "install v0.4.7 first")
	assert.Contains(t, err.Error(), "sphericon v0.6.0 (commit abc1234")
	assert.Contains(t, err.Error(), db.UpgradeGuideURL)
	assert.Equal(t, db.SchemaState{Major: 1, Floor: "v0.0.0"}, heldState(t, sqlDB))
}

func TestGuardRefusesOneMajorAheadAndNamesMigrateUnlessAutoMigrateIsOn(t *testing.T) {
	sqlDB := migratedAt(t, 2, "v0.4.7")

	err := guard("v0.6.0", false).Check(t.Context(), sqlDB)
	var refusal *db.RefusalError
	require.ErrorAs(t, err, &refusal)
	assert.Contains(t, err.Error(), "sphericon migrate")
	assert.Contains(t, err.Error(), "sphericon v0.6.0 (commit abc1234")
	assert.Contains(t, err.Error(), db.UpgradeGuideURL)

	require.NoError(t, guard("v0.6.0", true).Check(t.Context(), sqlDB))
	assert.Equal(t, db.SchemaState{Major: 2, Floor: "v0.4.7"}, heldState(t, sqlDB))
}

func TestGuardRefusesPendingMigrationsAndNamesMigrateUnlessAutoMigrateIsOn(t *testing.T) {
	sqlDB := openEmpty(t)
	// The database has run one migration of its own, so every embedded one is still to run.
	_, err := db.MigrateFS(t.Context(), sqlDB, fstest.MapFS{
		"20200101000000_one.sql": {Data: []byte("-- +goose Up\nSELECT 1;\n")},
	})
	require.NoError(t, err)
	g := guard("v0.6.0", false)
	g.Majors = majors[:1]

	err = g.Check(t.Context(), sqlDB)
	var refusal *db.RefusalError
	require.ErrorAs(t, err, &refusal)
	assert.Contains(t, err.Error(), "migration(s) the database has not run")
	assert.Contains(t, err.Error(), "mise run db:migrate")

	g.AutoMigrate = true
	require.NoError(t, g.Check(t.Context(), sqlDB))

	// A development build starts anyway: its error page runs them.
	g.AutoMigrate, g.AllowPending = false, true
	require.NoError(t, g.Check(t.Context(), sqlDB))
}

// An empty database is behind no Major: migrating creates it at the binary's own, so a new
// Instance installs the latest release directly, with no release of an older Major first.
func TestGuardLetsAnyBinaryMigrateAnEmptyDatabase(t *testing.T) {
	sqlDB := openEmpty(t)

	err := guard("v0.6.0", true).Check(t.Context(), sqlDB)

	require.NoError(t, err)
}

// A database of a Major older than the first of the binary's table has no migration to the
// binary's schema (the history before it was collapsed): it is refused, auto-migrate or not, and
// left as it is, never migrated over.
func TestGuardRefusesADatabaseOfAMajorBeforeTheTableAndNamesANewDatabase(t *testing.T) {
	sqlDB := migratedAt(t, 2, "v0.4.7")
	collapsed := []db.Major{{Major: 3, Floor: "v0.5.2"}}

	for _, g := range []db.Guard{
		{Build: guard("v0.6.0", true).Build, AutoMigrate: true, Majors: collapsed},
		{Build: guard("dev", false).Build, AllowPending: true, Majors: collapsed},
	} {
		err := g.Check(t.Context(), sqlDB)

		var refusal *db.RefusalError
		require.ErrorAs(t, err, &refusal)
		assert.Contains(t, err.Error(), "it is at Major 2, and this binary's migrations begin at Major 3")
		assert.Contains(t, err.Error(), "mise run db:reset")
		assert.Contains(t, err.Error(), db.UpgradeGuideURL)
	}
	assert.Equal(t, db.SchemaState{Major: 2, Floor: "v0.4.7"}, heldState(t, sqlDB))
}

// A database a release before the collapse left (Major 3, the typed Conditions on main before it)
// is refused by this binary's own table, even one Major behind with auto-migrate on.
func TestGuardRefusesADatabaseFromBeforeTheCollapse(t *testing.T) {
	sqlDB := migratedAt(t, 3, "v0.7.0")

	err := db.Guard{Build: guard("v0.8.0", true).Build, AutoMigrate: true}.Check(t.Context(), sqlDB)

	var refusal *db.RefusalError
	require.ErrorAs(t, err, &refusal)
	assert.Contains(t, err.Error(), "it is at Major 3")
}

// A database from before the state table (the releases before it, v0.1.x) holds the app's tables
// and no state row: it is Major 1, not an empty database to migrate, so `migrate` refuses it and
// applies nothing.
func TestMigrateRefusesADatabaseFromBeforeTheStateTableAndAppliesNothing(t *testing.T) {
	url := testhelper.EmptyDBURL(t)
	sqlDB, err := sql.Open("pgx", url)
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Close(sqlDB, "sqlDB") })
	_, err = db.MigrateFS(t.Context(), sqlDB, fstest.MapFS{
		"20200101000000_v01.sql": {Data: []byte("-- +goose Up\nCREATE TABLE \"workspaces\" (\"id\" bigint PRIMARY KEY);\n")},
	})
	require.NoError(t, err)

	_, err = db.ApplyMigrations(t.Context(), url)

	var refusal *db.RefusalError
	require.ErrorAs(t, err, &refusal)
	assert.Contains(t, err.Error(), "it is at Major 1")
	st, err := db.Status(t.Context(), sqlDB)
	require.NoError(t, err)
	assert.Zero(t, st.Current, "no migration ran")
}

// A build with no release (go run, a dev image) is never older than a floor.
func TestGuardLetsADevelopmentBuildStart(t *testing.T) {
	sqlDB := migratedAt(t, 3, "v0.5.2")

	require.NoError(t, guard("dev", false).Check(t.Context(), sqlDB))
}

// A database from before the state row has no table: it is Major 1.
func TestReadStateOnADatabaseWithoutTheRowIsMajorOne(t *testing.T) {
	sqlDB := openEmpty(t)

	assert.Equal(t, db.SchemaState{Major: 1, Floor: "v0.0.0"}, heldState(t, sqlDB))
}
