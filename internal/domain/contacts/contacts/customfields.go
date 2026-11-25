package contacts

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/localcalendar"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// ErrReservedKey means a Custom field was given the key of a core attribute or an alias key.
var ErrReservedKey = validation.Refuse("contacts: a custom field key names a contact attribute", i18n.ErrorsCustomFieldKeyReserved)

// EnsureCustomFields implements declared-by-use Custom fields (ADR 0006): for each
// key in the incoming attribute map it auto-creates a typed CustomField definition
// (idempotent upsert on (workspace_id, key)) and returns the values to merge into a
// Contact's custom_fields. There is no schemaless "trait" — every attribute is a
// first-class, typed, renameable definition from the first sight.
//
// Type is inferred from the value's JSON shape on first sight (inferFieldType). A later value of a
// conflicting type does NOT retype or reject (declared-by-use must not break ingest): the stored
// value keeps its own JSON type, the definition's type is left as-is, and a Condition leaf of the
// field's type is false on it. A person changes the type (the use case
// internal/usecase/customfieldretyping). A key that names a core attribute or an alias key is
// refused (ADR 0072).
func (m *Module) EnsureCustomFields(ctx context.Context, s *ent.Scoped, kv map[string]any) (map[string]any, error) {
	if len(kv) == 0 {
		return nil, nil
	}
	keys := lo.Without(lo.Keys(kv), "")
	if slices.ContainsFunc(keys, func(k string) bool { return reservedKeys[k] }) {
		return nil, ErrReservedKey
	}
	// Only the keys not defined yet are inserted: an ingest of known keys reads once and writes
	// nothing, rather than an insert per key that only conflicts.
	var defined []string
	if err := s.CustomField().Query().Where(customfield.KeyIn(keys...)).Select(customfield.FieldKey).Scan(ctx, &defined); err != nil {
		return nil, err
	}
	known := lo.SliceToMap(defined, func(k string) (string, bool) { return k, true })
	out := make(map[string]any, len(keys))
	for _, key := range keys {
		val := kv[key]
		out[key] = val
		if known[key] {
			continue
		}
		// DO NOTHING, not Ignore: an insert is the audited create of the definition under
		// a User or API token (ADR 0022); a conflict (a concurrent first use) writes nothing.
		err := s.CustomField().Create().
			SetKey(key).
			SetName(key).
			SetType(inferFieldType(val)).
			OnConflictColumns(customfield.FieldWorkspaceID, customfield.FieldKey).
			DoNothing().
			Exec(ctx)
		_, err = db.Inserted(err)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// KnownCustomFields is the part of kv whose keys are defined Custom fields: what an Identify through
// the public collect key may write, since it creates no definition (ADR 0171). The rest is dropped,
// as an invalid value of a core attribute is (ADR 0072).
func (m *Module) KnownCustomFields(ctx context.Context, s *ent.Scoped, kv map[string]any) (map[string]any, error) {
	if len(kv) == 0 {
		return nil, nil
	}
	var defined []string
	if err := s.CustomField().Query().Where(customfield.KeyIn(lo.Keys(kv)...)).Select(customfield.FieldKey).Scan(ctx, &defined); err != nil {
		return nil, err
	}
	return lo.PickByKeys(kv, defined), nil
}

// Domain errors of a Custom field's edit. Callers match with errors.Is.
var (
	// ErrCustomFieldNotFound means no such Custom field in this Workspace.
	ErrCustomFieldNotFound = validation.NotFound("contacts: custom field not found", i18n.ErrorsCustomFieldNotFound)
	// ErrCustomFieldRevisionChanged: the edit was made on a revision the Custom field has moved past
	// (ADR 0119): someone wrote it since the client read it. Nothing is written.
	ErrCustomFieldRevisionChanged = validation.RevisionChanged("contacts: custom field changed since it was read", i18n.ErrorsCustomFieldRevisionChanged)
)

// CustomFieldUpdateInput is an edit of a Custom field: its display name and its type. The key never
// changes: Contacts' values and Condition leaves address the field by it.
type CustomFieldUpdateInput struct {
	// Revision is the Custom field's revision the edit was made on (ADR 0119).
	Revision int64
	Name     patch.Field[string]
	// Type is a value of the contract's enum, which is ent's: a type a field cannot have is refused
	// at the edge and never reaches the module.
	Type patch.Field[string]
}

// CustomField returns the Custom field with the given id, ErrCustomFieldNotFound when the Workspace
// has none.
func (m *Module) CustomField(ctx context.Context, s *ent.Scoped, id int64) (*ent.CustomField, error) {
	field, err := s.CustomField().Get(ctx, id)
	if ent.IsNotFound(err) {
		return nil, ErrCustomFieldNotFound
	}
	return field, err
}

// CustomFields returns the Workspace's whole Custom field catalogue, ascending by key.
// The catalogue is small and unpaginated.
func (m *Module) CustomFields(ctx context.Context, s *ent.Scoped) ([]*ent.CustomField, error) {
	return s.CustomField().Query().Order(ent.Asc(customfield.FieldKey)).All(ctx)
}

// HasCustomField reports whether the Workspace's Custom field catalogue has a field of the key.
func (m *Module) HasCustomField(ctx context.Context, s *ent.Scoped, key string) (bool, error) {
	ok, err := s.CustomField().Query().Where(customfield.Key(key)).Exist(ctx)
	if err != nil {
		return false, fmt.Errorf("look up custom field %q: %w", key, err)
	}
	return ok, nil
}

// inferFieldType maps a JSON-decoded value to a CustomField type. Numbers decode to
// float64 (encoding/json) or json.Number; booleans to bool; a string in one of the ISO forms of a
// date is a datetime (localcalendar.IsISODate); everything else (other strings, objects, arrays,
// null) is treated as a string definition.
func inferFieldType(v any) customfield.Type {
	switch val := v.(type) {
	case bool:
		return customfield.TypeBool
	case float64, float32, int, int64, json.Number:
		return customfield.TypeNumber
	case string:
		if localcalendar.IsISODate(val) {
			return customfield.TypeDatetime
		}
		return customfield.TypeString
	default:
		return customfield.TypeString
	}
}
