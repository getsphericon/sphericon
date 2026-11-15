package testhelper

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// ForeignKeyOrphans checks every foreign key of sqlDB against the rows it holds, as VALIDATE
// CONSTRAINT would, and describes each key some row breaks: rows loaded with the keys switched off
// (the fixtures) are checked by nothing else. It does not change the database.
func ForeignKeyOrphans(t *testing.T, sqlDB *sql.DB) []string {
	t.Helper()
	type foreignKey struct{ name, child, parent, childCols, parentCols string }
	var keys []foreignKey
	// raw sql: the foreign keys and their columns are only in the catalog (pg_constraint).
	rows, err := sqlDB.QueryContext(t.Context(), `SELECT con.conname, con.conrelid::regclass::text, con.confrelid::regclass::text,
	  array_to_string(ARRAY(SELECT quote_ident(a.attname) FROM unnest(con.conkey) WITH ORDINALITY AS k(num, ord)
	    JOIN pg_attribute a ON a.attrelid = con.conrelid AND a.attnum = k.num ORDER BY k.ord), ','),
	  array_to_string(ARRAY(SELECT quote_ident(a.attname) FROM unnest(con.confkey) WITH ORDINALITY AS k(num, ord)
	    JOIN pg_attribute a ON a.attrelid = con.confrelid AND a.attnum = k.num ORDER BY k.ord), ',')
	FROM pg_constraint con JOIN pg_namespace n ON n.oid = con.connamespace
	WHERE n.nspname = 'public' AND con.contype = 'f' ORDER BY con.conname`)
	require.NoError(t, err)
	defer func() {
		closeErr := rows.Close()
		require.NoError(t, closeErr)
	}()
	for rows.Next() {
		var k foreignKey
		scanErr := rows.Scan(&k.name, &k.child, &k.parent, &k.childCols, &k.parentCols)
		require.NoError(t, scanErr)
		keys = append(keys, k)
	}
	rowsErr := rows.Err()
	require.NoError(t, rowsErr)
	require.NotEmpty(t, keys, "the database has foreign keys to check")

	var broken []string
	for _, k := range keys {
		parentCols := strings.Split(k.parentCols, ",")
		var present, match []string
		for i, c := range strings.Split(k.childCols, ",") {
			// MATCH SIMPLE: a key with any column null references nothing.
			present = append(present, "c."+c+" IS NOT NULL")
			match = append(match, "p."+parentCols[i]+" = c."+c)
		}
		var orphans int
		// raw sql: one anti-join per foreign key, built from the catalog; ent models no arbitrary key.
		err := sqlDB.QueryRowContext(t.Context(), fmt.Sprintf(
			"SELECT count(*) FROM %s c WHERE %s AND NOT EXISTS (SELECT 1 FROM %s p WHERE %s)",
			k.child, strings.Join(present, " AND "), k.parent, strings.Join(match, " AND "))).Scan(&orphans)
		require.NoError(t, err)
		if orphans > 0 {
			broken = append(broken, fmt.Sprintf("%s: %d row(s) of %s name no row of %s", k.name, orphans, k.child, k.parent))
		}
	}
	return broken
}
