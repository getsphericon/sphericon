package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// A dollar-quoted body holds semicolons of its own, which goose would split at; such a
// statement is wrapped, a plain one is not.
func TestGooseStatementsWrapOnlyStatementsWithADollarQuotedBody(t *testing.T) {
	got := gooseStatements(`CREATE TABLE a (id int);

CREATE OR REPLACE FUNCTION f() RETURNS int AS $$
BEGIN
  RETURN 1;
END;
$$ LANGUAGE plpgsql;

DO
$body$
BEGIN
  PERFORM 1;
END;
$body$
LANGUAGE 'plpgsql';
`)

	assert.Equal(t, `CREATE TABLE a (id int);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION f() RETURNS int AS $$
BEGIN
  RETURN 1;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
DO
$body$
BEGIN
  PERFORM 1;
END;
$body$
LANGUAGE 'plpgsql';
-- +goose StatementEnd
`, got)
}

func TestSplitStatementsIgnoresSemicolonsInCommentsAndQuotedStrings(t *testing.T) {
	got := splitStatements(`-- it's a comment; with an apostrophe
SELECT 'a;b', 'it''s';
-- trailing comment only
`)

	assert.Equal(t, []string{"-- it's a comment; with an apostrophe\nSELECT 'a;b', 'it''s';"}, got)
}

func TestRiverSQLDropsTheSchemaTemplate(t *testing.T) {
	assert.Equal(t, "CREATE TABLE river_job(id int);", riverSQL("CREATE TABLE /* TEMPLATE: schema */river_job(id int);"))
}

func TestRiverRecordMatchesTheShapeOfRiverMigrationAtThatVersion(t *testing.T) {
	assert.Equal(t, "INSERT INTO river_migration (version) VALUES (4);", riverRecord(4))
	assert.Equal(t, "INSERT INTO river_migration (line, version) VALUES ('main', 5);", riverRecord(5))
}
