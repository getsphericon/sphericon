package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// TypedConditionsVersion is the version of the Go migration that rewrites every stored Condition
// to the typed tree of its place (ADR 0181), after 20261121182751_condition_entries_default, which
// gives the columns the typed empty group as their default.
const TypedConditionsVersion = 20261121205037

// typedConditionsMajor is the Major the typed Conditions start, with its floor, the last release
// before them: a release older than that refuses to start on the rewritten Conditions (ADR 0033).
// Majors lists it, and the migration writes it to the state row.
var typedConditionsMajor = Major{Major: 3, Floor: "v0.7.0"}

// conditionColumn is a table whose `condition` column holds a Condition of one place, with the
// statements that read every Condition it holds, by id, and rewrite one.
type conditionColumn struct {
	table       string
	place       conditionPlace
	read, write string
}

// conditionColumns is every stored Condition: a Segment's (an archived one too), every Rule
// version's (past, live and draft) and an armed Broadcast's copy of its Segment's. An Audit
// entry keeps the Condition in the shape it had: the log is immutable.
var conditionColumns = []conditionColumn{
	{
		table: "segments", place: segmentConditions,
		read:  `SELECT "id", "workspace_id", "condition" FROM "segments" ORDER BY "id"`,
		write: `UPDATE "segments" SET "condition" = $1 WHERE "id" = $2`,
	},
	{
		table: "rule_versions", place: ruleConditions,
		read:  `SELECT "id", "workspace_id", "condition" FROM "rule_versions" ORDER BY "id"`,
		write: `UPDATE "rule_versions" SET "condition" = $1 WHERE "id" = $2`,
	},
	{
		// A Broadcast to all Contacts, or a draft, holds no Condition.
		table: "broadcasts", place: segmentConditions,
		read:  `SELECT "id", "workspace_id", "condition" FROM "broadcasts" WHERE "condition" IS NOT NULL ORDER BY "id"`,
		write: `UPDATE "broadcasts" SET "condition" = $1 WHERE "id" = $2`,
	},
}

// UnmappableCondition is a stored Condition the migration cannot rewrite to the typed tree, and
// why.
type UnmappableCondition struct {
	Table       string
	ID          int64
	WorkspaceID int64
	Reason      string
}

// UnmappableConditionsError stops the migration when any stored Condition cannot be rewritten:
// it names every such row, and nothing is rewritten. The Self-hoster edits or archives each on the
// running release and runs the Upgrade again.
type UnmappableConditionsError struct {
	Conditions []UnmappableCondition
}

func (e *UnmappableConditionsError) Error() string {
	rows := make([]string, len(e.Conditions))
	for i, c := range e.Conditions {
		rows[i] = fmt.Sprintf("%s %d (Workspace %d): %s", c.Table, c.ID, c.WorkspaceID, c.Reason)
	}
	return fmt.Sprintf("%d stored Conditions cannot be rewritten to the typed tree, so none was: %s. "+
		"Edit or archive each on the running release, then run the Upgrade again",
		len(e.Conditions), strings.Join(rows, "; "))
}

// storedCondition is one row's Condition as the migration reads it.
type storedCondition struct {
	id, workspaceID int64
	doc             []byte
}

// rewrite is one row's typed Condition.
type rewrite struct {
	col conditionColumn
	id  int64
	doc []byte
}

// RewriteTypedConditions is the Go migration TypedConditionsVersion: it rewrites every stored
// Condition to the typed tree (ADR 0181) and moves the schema to typedConditionsMajor, since an
// older release cannot read the typed tree. It maps every row before it writes one, so a tree it
// cannot map stops it with nothing rewritten. A Condition typed at every depth is left as it is, so
// a second run changes nothing (ADR 0033), which its test runs it again to see.
func RewriteTypedConditions(ctx context.Context, tx *sql.Tx) error {
	var rewrites []rewrite
	var unmappable []UnmappableCondition
	for _, col := range conditionColumns {
		stored, err := storedConditions(ctx, tx, col)
		if err != nil {
			return err
		}
		for _, s := range stored {
			doc, err := typedCondition(s.doc, col.place)
			switch {
			case err != nil:
				unmappable = append(unmappable, UnmappableCondition{Table: col.table, ID: s.id, WorkspaceID: s.workspaceID, Reason: err.Error()})
			case doc != nil:
				rewrites = append(rewrites, rewrite{col: col, id: s.id, doc: doc})
			}
		}
	}
	if len(unmappable) > 0 {
		return &UnmappableConditionsError{Conditions: unmappable}
	}
	for _, r := range rewrites {
		// raw sql: a Go migration runs on the frozen schema, see phonesToE164.
		if _, err := tx.ExecContext(ctx, r.col.write, string(r.doc), r.id); err != nil {
			return fmt.Errorf("rewrite the condition of %s %d: %w", r.col.table, r.id, err)
		}
	}
	// raw sql: a Go migration runs on the frozen schema, see phonesToE164.
	if _, err := tx.ExecContext(ctx, `UPDATE "schema_state" SET "major" = $1, "floor" = $2, "updated_at" = now() WHERE "id" = 1`,
		typedConditionsMajor.Major, typedConditionsMajor.Floor); err != nil {
		return fmt.Errorf("write the schema state: %w", err)
	}
	return nil
}

// storedConditions is every Condition col holds, by id.
func storedConditions(ctx context.Context, tx *sql.Tx, col conditionColumn) ([]storedCondition, error) {
	// raw sql: a Go migration runs on the frozen schema, see phonesToE164.
	rows, err := tx.QueryContext(ctx, col.read)
	if err != nil {
		return nil, fmt.Errorf("read the conditions of %s: %w", col.table, err)
	}
	defer func() { _ = rows.Close() }()
	var out []storedCondition
	for rows.Next() {
		var s storedCondition
		if err := rows.Scan(&s.id, &s.workspaceID, &s.doc); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
