package db_test

import (
	"database/sql"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/buildinfo"
	"github.com/getsphericon/sphericon/internal/db"
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
	require.NoError(t, db.NewEntClient(sqlDB).SchemaState.UpdateOneID(1).SetMajor(major).SetFloor(floor).Exec(t.Context()))
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

// The first migrations leave the state row: Major 1, every release runs on it. The binary's
// table of Majors ends at the Major its migrations write.
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
