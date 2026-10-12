package contacts

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/customfield"
)

// EnsureCustomFields implements declared-by-use Custom fields (ADR 0006): for each
// key in the incoming attribute map it auto-creates a typed CustomField definition
// (idempotent upsert on (workspace_id, key)) and returns the values to merge into a
// Contact's custom_fields. There is no schemaless "trait" — every attribute is a
// first-class, typed, renameable definition from the first sight.
//
// Type is inferred from the value's JSON shape on first sight. A later value of a
// conflicting type does NOT retype or reject (declared-by-use must not break ingest);
// the stored value keeps its own JSON type and the definition's type is left as-is —
// widening/coercion policy is a later refinement.
func EnsureCustomFields(ctx context.Context, s *ent.Scoped, kv map[string]any) (map[string]any, error) {
	if len(kv) == 0 {
		return nil, nil
	}
	out := make(map[string]any, len(kv))
	for key, val := range kv {
		if key == "" {
			continue
		}
		// DO NOTHING, not Ignore: an insert is the audited create of the definition under
		// a User or API token (ADR 0022); a conflict writes nothing and reports sql.ErrNoRows.
		if err := s.CustomField().Create().
			SetKey(key).
			SetName(key).
			SetType(inferFieldType(val)).
			OnConflictColumns(customfield.FieldWorkspaceID, customfield.FieldKey).
			DoNothing().
			Exec(ctx); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		out[key] = val
	}
	return out, nil
}

// inferFieldType maps a JSON-decoded value to a CustomField type. Numbers decode to
// float64 (encoding/json) or json.Number; booleans to bool; everything else
// (strings, objects, arrays, null) is treated as a string definition.
func inferFieldType(v any) customfield.Type {
	switch v.(type) {
	case bool:
		return customfield.TypeBool
	case float64, float32, int, int64, json.Number:
		return customfield.TypeNumber
	default:
		return customfield.TypeString
	}
}
