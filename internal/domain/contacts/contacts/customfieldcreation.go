package contacts

import (
	"context"
	"strings"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/kernel/fieldname"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// Refusals of a Custom field a Member creates (NewCustomField).
var (
	// ErrCustomFieldKeyBlank means a new Custom field was given no key.
	ErrCustomFieldKeyBlank = validation.RefuseField(fieldname.Key, "contacts: a custom field needs a key", i18n.ErrorsCustomFieldKeyBlank)
	// ErrCustomFieldKeyTaken means a new Custom field was given the key of a field the Workspace
	// has: the refusal names the key.
	ErrCustomFieldKeyTaken = validation.ConflictField(fieldname.Key, "contacts: the workspace has a custom field of the key", i18n.ErrorsCustomFieldKeyTaken)
)

// NewCustomField is a Custom field a Member creates with the key, the display name and the type
// they choose (ADR 0138's CSV import), unlike ingest's declaration by use, which infers the type.
type NewCustomField struct {
	Key  string
	Name string
	Type customfield.Type
}

// CheckNewCustomFieldKey refuses a key no new Custom field may take: a blank one
// (ErrCustomFieldKeyBlank), one naming a core attribute or an alias key (ErrReservedKey), or, when
// taken says the Workspace has a field of it, a taken one (ErrCustomFieldKeyTaken, naming it).
func CheckNewCustomFieldKey(key string, taken bool) error {
	switch {
	case strings.TrimSpace(key) == "":
		return ErrCustomFieldKeyBlank
	case reservedKeys[key]:
		return ErrReservedKey
	case taken:
		return ErrCustomFieldKeyTaken.Args(map[string]any{"Key": key})
	}
	return nil
}

// CreateCustomFieldIn creates the Custom field the input names in the Workspace of s, an audited
// create (ADR 0022) in the transaction s is bound to. Its key is checked as CheckNewCustomFieldKey
// checks it, its name by the name rule, and a key the Workspace has a field of already, even one
// declared a moment before, is ErrCustomFieldKeyTaken: nothing is retyped.
func (m *Module) CreateCustomFieldIn(ctx context.Context, s *ent.Scoped, in NewCustomField) (*ent.CustomField, error) {
	if err := CheckNewCustomFieldKey(in.Key, false); err != nil {
		return nil, err
	}
	name, err := validation.Name(in.Name)
	if err != nil {
		return nil, err
	}
	// DO NOTHING, as EnsureCustomFields: a key taken by a concurrent first use is a refusal, not a
	// violation of the unique key.
	err = s.CustomField().Create().
		SetKey(in.Key).
		SetName(name).
		SetType(in.Type).
		OnConflictColumns(customfield.FieldWorkspaceID, customfield.FieldKey).
		DoNothing().
		Exec(ctx)
	inserted, err := db.Inserted(err)
	if err != nil {
		return nil, err
	}
	if !inserted {
		return nil, CheckNewCustomFieldKey(in.Key, true)
	}
	return m.CustomFieldByKey(ctx, s, in.Key)
}
