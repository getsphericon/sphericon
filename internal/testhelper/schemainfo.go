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

// UnvalidatedConstraints is the names of the constraints of table that were added NOT VALID and
// are not validated yet, sorted.
func UnvalidatedConstraints(t *testing.T, sqlDB *sql.DB, table string) []string {
	t.Helper()
	// raw sql: reads pg_constraint, which is not an ent entity.
	rows, err := sqlDB.QueryContext(t.Context(),
		`SELECT conname FROM pg_constraint WHERE conrelid = $1::regclass AND NOT convalidated ORDER BY conname`, table)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var names []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		names = append(names, name)
	}
	require.NoError(t, rows.Err())
	return names
}

// UniqueIndexes maps each unique index of the public schema that is no primary key to its table. A
// unique constraint is an index too, and Postgres reports a violation by the index's name.
func UniqueIndexes(t *testing.T, sqlDB *sql.DB) map[string]string {
	t.Helper()
	// raw sql: reads pg_index, which is not an ent entity.
	rows, err := sqlDB.QueryContext(t.Context(), `
		SELECT i.relname, c.relname FROM pg_index x
		JOIN pg_class i ON i.oid = x.indexrelid
		JOIN pg_class c ON c.oid = x.indrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE x.indisunique AND NOT x.indisprimary AND n.nspname = 'public'`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var index, table string
		require.NoError(t, rows.Scan(&index, &table))
		out[index] = table
	}
	require.NoError(t, rows.Err())
	require.NotEmpty(t, out)
	return out
}
