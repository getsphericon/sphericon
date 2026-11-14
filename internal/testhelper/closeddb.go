package testhelper

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/app/entclient"
)

// ClosedDB is a database handle that is already closed: every statement on it fails, for a test
// of how code answers a storage outage (an error to the caller, never a mistaken empty result).
func ClosedDB(tb testing.TB) *sql.DB {
	tb.Helper()
	sqlDB, err := sql.Open("pgx", "postgres://closed.invalid/none")
	require.NoError(tb, err)
	require.NoError(tb, sqlDB.Close())
	return sqlDB
}

// ClosedClient is the ent client over ClosedDB.
func ClosedClient(tb testing.TB) *ent.Client {
	tb.Helper()
	return entclient.Core.New(ClosedDB(tb))
}
