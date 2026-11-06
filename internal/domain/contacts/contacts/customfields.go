package contacts

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
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
func EnsureCustomFields(ctx context.Context, s *ent.Scoped, kv map[string]any) (map[string]any, error) {
	if len(kv) == 0 {
		return nil, nil
	}
	out := make(map[string]any, len(kv))
	for key, val := range kv {
		if key == "" {
			continue
		}
		if reservedKeys[key] {
			return nil, ErrReservedKey
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

// Domain errors of a Custom field's edit. Callers match with errors.Is.
var (
	// ErrCustomFieldNotFound means no such Custom field in this Workspace.
	ErrCustomFieldNotFound = validation.NotFound("contacts: custom field not found", i18n.ErrorsCustomFieldNotFound)
	// ErrCustomFieldNameRequired means the new name of a Custom field is blank.
	ErrCustomFieldNameRequired = validation.RefuseField("name", "contacts: custom field name is required", i18n.ErrorsNameRequired)
)

// CustomFieldUpdateInput is an edit of a Custom field: its display name and its type. The key never
// changes: Contacts' values and Condition leaves address the field by it.
type CustomFieldUpdateInput struct {
	Name patch.Field[string]
	// Type is a value of the contract's enum, which is ent's: a type a field cannot have is refused
	// at the edge and never reaches the module.
	Type patch.Field[string]
}

// CustomField returns the Custom field with the given id, ErrCustomFieldNotFound when the Workspace
// has none.
func CustomField(ctx context.Context, s *ent.Scoped, id int64) (*ent.CustomField, error) {
	field, err := s.CustomField().Get(ctx, id)
	if ent.IsNotFound(err) {
		return nil, ErrCustomFieldNotFound
	}
	return field, err
}

// Check refuses an edit whose new name is blank (ErrCustomFieldNameRequired).
func (in CustomFieldUpdateInput) Check() error {
	if raw, ok := in.Name.Get(); ok && strings.TrimSpace(raw) == "" {
		return ErrCustomFieldNameRequired
	}
	return nil
}

// Retype returns the type the edit changes field to, and false when the edit keeps its type.
func (in CustomFieldUpdateInput) Retype(field *ent.CustomField) (customfield.Type, bool) {
	raw, ok := in.Type.Get()
	if !ok || customfield.Type(raw) == field.Type {
		return "", false
	}
	return customfield.Type(raw), true
}

// UpdateCustomFieldIn renames field or changes its type, an audited change (ADR 0022). A type
// change rewrites no stored value: the leaves of the new type read the values as they are, and are
// false on one of another shape. It is the unchecked retype (ADR 0102): it does not ask whether a
// Segment, a Rule or a Broadcast tests the field with a leaf the new type does not take. Editing a
// Custom field is the use case internal/usecase/customfieldretyping, which runs that check first;
// easement forbids every other package to call UpdateCustomFieldIn. A blank new name is
// ErrCustomFieldNameRequired.
func UpdateCustomFieldIn(ctx context.Context, s *ent.Scoped, field *ent.CustomField, in CustomFieldUpdateInput) (*ent.CustomField, error) {
	if err := in.Check(); err != nil {
		return nil, err
	}
	upd := s.CustomField().UpdateOneID(field.ID)
	if raw, ok := in.Name.Get(); ok {
		upd.SetName(strings.TrimSpace(raw))
	}
	if t, ok := in.Retype(field); ok {
		upd.SetType(t)
	}
	return upd.Save(ctx)
}

// CustomFields returns the Workspace's whole Custom field catalogue, ascending by key.
// The catalogue is small and unpaginated.
func CustomFields(ctx context.Context, s *ent.Scoped) ([]*ent.CustomField, error) {
	return s.CustomField().Query().Order(ent.Asc(customfield.FieldKey)).All(ctx)
}

// inferFieldType maps a JSON-decoded value to a CustomField type. Numbers decode to
// float64 (encoding/json) or json.Number; booleans to bool; a string in one of the ISO forms of a
// date is a datetime (isoDate); everything else (other strings, objects, arrays, null) is treated
// as a string definition.
func inferFieldType(v any) customfield.Type {
	switch val := v.(type) {
	case bool:
		return customfield.TypeBool
	case float64, float32, int, int64, json.Number:
		return customfield.TypeNumber
	case string:
		if isoDate(val) {
			return customfield.TypeDatetime
		}
		return customfield.TypeString
	default:
		return customfield.TypeString
	}
}

// dateLayouts are the ISO forms of a date a value makes a datetime field from (#817): a date-time
// with an offset (RFC 3339, "Z" or "+03:00", fractional seconds allowed), one without an offset and
// a bare date, the last two read in the Workspace time zone (ADR 0087). A slash form such as
// 01/15/2026 is read month-first by some tools and day-first by others, so it stays a string.
var dateLayouts = []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999", time.DateOnly}

// isoDate reports whether s is a valid date in one of dateLayouts.
func isoDate(s string) bool {
	return slices.ContainsFunc(dateLayouts, func(layout string) bool {
		_, err := time.Parse(layout, s)
		return err == nil
	})
}
