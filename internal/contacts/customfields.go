package contacts

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/inuse"
	"github.com/getsphericon/sphericon/internal/patch"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/segments"
	"github.com/getsphericon/sphericon/internal/validation"
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
// field's type is false on it. A person changes the type (UpdateCustomField). A key that names a
// core attribute or an alias key is refused (ADR 0072).
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

// UpdateCustomField renames a Custom field or changes its type, an audited change (ADR 0022). A type
// change rewrites no stored value: the leaves of the new type read the values as they are, and are
// false on one of another shape. It is refused with an *inuse.Error while a Segment, a kept Rule or
// an armed Broadcast tests the field with a leaf the new type does not take, as their Conditions
// would stop compiling; a leaf every type takes (notNull) does not hold the type. An unknown id is
// ErrCustomFieldNotFound.
func UpdateCustomField(ctx context.Context, s *ent.Scoped, id int64, in CustomFieldUpdateInput) (*ent.CustomField, error) {
	field, err := s.CustomField().Get(ctx, id)
	if ent.IsNotFound(err) {
		return nil, ErrCustomFieldNotFound
	}
	if err != nil {
		return nil, err
	}
	upd := s.CustomField().UpdateOneID(id)
	if raw, ok := in.Name.Get(); ok {
		name := strings.TrimSpace(raw)
		if name == "" {
			return nil, ErrCustomFieldNameRequired
		}
		upd.SetName(name)
	}
	if raw, ok := in.Type.Get(); ok && customfield.Type(raw) != field.Type {
		t := customfield.Type(raw)
		users, err := unfitUsers(ctx, s, field.Key, t)
		if err != nil {
			return nil, err
		}
		if len(users) > 0 {
			return nil, &inuse.Error{What: inuse.SubjectCustomField, Users: users}
		}
		upd.SetType(t)
	}
	return upd.Save(ctx)
}

// unfitUsers is what tests the Custom field key with a Condition leaf a field of type t does not
// take: Segments, kept Rules and armed Broadcasts. An Automation holds no Condition.
func unfitUsers(ctx context.Context, s *ent.Scoped, key string, t customfield.Type) ([]inuse.User, error) {
	segs, err := segments.UnfitForCustomFieldType(ctx, s, key, t)
	if err != nil {
		return nil, err
	}
	kept, err := rules.UnfitForCustomFieldType(ctx, s, key, t)
	if err != nil {
		return nil, err
	}
	armed, err := broadcasts.UnfitForCustomFieldType(ctx, s, key, t)
	if err != nil {
		return nil, err
	}
	users := lo.Map(segs, func(seg *ent.Segment, _ int) inuse.User {
		return inuse.User{Kind: inuse.Segment, ID: seg.ID, Name: seg.Name}
	})
	users = append(users, inuse.Rules(kept)...)
	return append(users, inuse.Broadcasts(armed)...), nil
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
