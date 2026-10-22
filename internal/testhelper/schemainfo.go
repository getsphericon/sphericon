package testhelper

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
)

// ColumnsOf counts how many of the named columns the table has.
func ColumnsOf(t *testing.T, sqlDB *sql.DB, table string, columns ...string) int {
	t.Helper()
	n := 0
	for _, column := range columns {
		var found int
		// raw sql: reads information_schema, which is not an ent entity.
		require.NoError(t, sqlDB.QueryRowContext(t.Context(),
			`SELECT count(*) FROM information_schema.columns WHERE table_name = $1 AND column_name = $2`, table, column).Scan(&found))
		n += found
	}
	return n
}

// ColumnIsNullable reports whether the table's column allows NULL.
func ColumnIsNullable(t *testing.T, sqlDB *sql.DB, table, column string) bool {
	t.Helper()
	var nullable string
	// raw sql: reads information_schema, which is not an ent entity.
	require.NoError(t, sqlDB.QueryRowContext(t.Context(),
		`SELECT is_nullable FROM information_schema.columns WHERE table_name = $1 AND column_name = $2`, table, column).Scan(&nullable))
	return nullable == "YES"
}
