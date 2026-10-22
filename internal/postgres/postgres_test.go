package postgres_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/postgres"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestServerMajorIsASupportedVersion(t *testing.T) {
	db := testhelper.RealDB(t)

	major, err := postgres.ServerMajor(t.Context(), db)

	require.NoError(t, err)
	require.GreaterOrEqual(t, major, 14)
}

func TestLockTxIsReleasedWhenTheTransactionEnds(t *testing.T) {
	db := testhelper.RealDB(t)
	first, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	require.NoError(t, postgres.LockTx(t.Context(), first, 7))
	require.NoError(t, first.Commit())

	second, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = second.Rollback() })

	require.NoError(t, postgres.LockTx(t.Context(), second, 7))
}
