package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/oklog/ulid/v2"
)

// StepsPathsVersion is the version of the Go migration that rewrites every Automation version's
// steps as a root path with ids (ADR 0122), between 20261122020712_steps_path_default, which drops
// the CHECK that they are an array, and 20261122020714_steps_path_check, which checks they are a
// path. It rides typedConditionsMajor: no release runs between the two.
const StepsPathsVersion = 20261122020713

// stepsIDSpace is the namespace of the ids the migration gives: an id is the name-based UUID of
// its Automation and position, so every version of an Automation gives the Step at a position, and
// its root path, the same id, as the report and the Enrollments named them by position.
var stepsIDSpace = uuid.MustParse("9f1d3c5e-2b7a-4e8f-a6d0-5c4b3a291807")

// storedSteps is one version's steps as the migration reads them.
type storedSteps struct {
	id, automationID int64
	doc              []byte
}

// RewriteStepsPaths is the Go migration StepsPathsVersion: every version whose steps are still an
// array is rewritten as the root path {"id", "steps"} holding that array, each Step given an id
// (stepIDOf). A version already a path is left as it is, so a second run changes nothing (ADR
// 0033), which its test runs it again to see.
func RewriteStepsPaths(ctx context.Context, tx *sql.Tx) error {
	stored, err := storedStepsOfVersions(ctx, tx)
	if err != nil {
		return err
	}
	for _, s := range stored {
		doc, err := stepsPathOf(s)
		if err != nil {
			return fmt.Errorf("rewrite the steps of automation_versions %d: %w", s.id, err)
		}
		if doc == nil {
			continue
		}
		// raw sql: a Go migration runs on the frozen schema, see phonesToE164.
		if _, err := tx.ExecContext(ctx, `UPDATE "automation_versions" SET "steps" = $1 WHERE "id" = $2`, string(doc), s.id); err != nil {
			return fmt.Errorf("rewrite the steps of automation_versions %d: %w", s.id, err)
		}
	}
	return nil
}

// stepsPathOf is the root path a version's array of steps becomes, nil when its steps are a path
// already.
func stepsPathOf(s storedSteps) ([]byte, error) {
	var steps []map[string]json.RawMessage
	if err := json.Unmarshal(s.doc, &steps); err != nil {
		var path map[string]json.RawMessage
		if json.Unmarshal(s.doc, &path) == nil && path["steps"] != nil {
			return nil, nil
		}
		return nil, err
	}
	if steps == nil {
		steps = []map[string]json.RawMessage{}
	}
	for pos, st := range steps {
		id, err := json.Marshal(stepIDOf(s.automationID, pos, st))
		if err != nil {
			return nil, err
		}
		st["id"] = id
	}
	root := uuid.NewSHA1(stepsIDSpace, fmt.Appendf(nil, "automation %d root", s.automationID)).String()
	return json.Marshal(map[string]any{"id": root, "steps": steps})
}

// stepIDOf is the id the step at pos of the Automation's version gets: an HTTP step keeps its id,
// a ULID written as the UUID of the same 128 bits; any other step gets the id of its position.
func stepIDOf(automationID int64, pos int, st map[string]json.RawMessage) string {
	var held string
	if raw, ok := st["id"]; ok && json.Unmarshal(raw, &held) == nil {
		if u, err := ulid.ParseStrict(held); err == nil {
			return uuid.UUID(u).String()
		}
		if u, err := uuid.Parse(held); err == nil {
			return u.String()
		}
	}
	return uuid.NewSHA1(stepsIDSpace, fmt.Appendf(nil, "automation %d step %d", automationID, pos)).String()
}

// storedStepsOfVersions is every Automation version's steps, by id.
func storedStepsOfVersions(ctx context.Context, tx *sql.Tx) ([]storedSteps, error) {
	// raw sql: a Go migration runs on the frozen schema, see phonesToE164.
	rows, err := tx.QueryContext(ctx, `SELECT "id", "automation_id", "steps" FROM "automation_versions" ORDER BY "id"`)
	if err != nil {
		return nil, fmt.Errorf("read the steps of automation_versions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []storedSteps
	for rows.Next() {
		var s storedSteps
		if err := rows.Scan(&s.id, &s.automationID, &s.doc); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
