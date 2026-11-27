package contactimports

import (
	"context"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/kernel/csvimport"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// A column mapped to a new Custom field (ADR 0138): a Member's review of the field, with the key,
// the label and the type they choose, rather than ingest's declaration by use. Mapping one needs
// custom_fields:manage, asked when the mapping is saved and again when the import runs, which
// creates the fields through the Contacts module before its job starts. Until then the preview
// reads the column in the chosen type as the run will.

// mayCreateFields is nil when the mapping creates no Custom field or role holds
// custom_fields:manage, and that Permission's refusal otherwise.
func mayCreateFields(role membership.Role, mapping csvimport.Mapping) error {
	if len(newCustomFields(mapping)) == 0 {
		return nil
	}
	return memberships.Require(role, memberships.PermCustomFieldsManage)
}

// newCustomFields are the Custom fields the mapping's columns create, in the file's order.
func newCustomFields(mapping csvimport.Mapping) []contacts.NewCustomField {
	return lo.FilterMap(mapping.Columns, func(c csvimport.Column, _ int) (contacts.NewCustomField, bool) {
		return contacts.NewCustomField{Key: c.Key, Name: c.Label, Type: customfield.Type(c.FieldType)}, c.Kind == csvimport.KindNewCustomField
	})
}

// checkedNewCustomField is a column mapped to a new Custom field as a mapping keeps it and the value
// it writes, or its refusal: a key a new field may take (contacts.CheckNewCustomFieldKey) and the
// Workspace has no field of (contacts.ErrCustomFieldKeyTaken; fields are the types of the fields
// the mapping names that the Workspace has), a label by the name rule, a type of a Custom field,
// and a datetime field's date format. Its value is the Custom field's, so a second column of the
// key is refused as any repeated target is.
func checkedNewCustomField(col csvimport.Column, fields map[string]customfield.Type) (csvimport.Column, string, error) {
	_, taken := fields[col.Key]
	if err := contacts.CheckNewCustomFieldKey(col.Key, taken); err != nil {
		return col, "", err
	}
	label, err := validation.Name(col.Label)
	if err != nil {
		return col, "", err
	}
	t := customfield.Type(col.FieldType)
	if customfield.TypeValidator(t) != nil {
		return col, "", ErrUnknownTarget
	}
	out := csvimport.Column{Kind: col.Kind, Key: col.Key, Label: label, FieldType: col.FieldType}
	if t == customfield.TypeDatetime {
		if col.DateFormat.Layout() == "" {
			return col, "", ErrDateFormatRequired
		}
		out.DateFormat = col.DateFormat
	}
	return out, "customField:" + col.Key, nil
}

// withNewFields adds to fields the type of each Custom field the mapping creates, the one its
// cells are read in, whether the run has created it yet or not.
func withNewFields(fields map[string]customfield.Type, mapping csvimport.Mapping) map[string]customfield.Type {
	for _, f := range newCustomFields(mapping) {
		fields[f.Key] = f.Type
	}
	return fields
}

// createNewCustomFields creates the Custom fields the mapping names, as role, in the transaction
// of ts: refused whole when role lacks custom_fields:manage, or when the Workspace has taken a key
// since the mapping was saved (contacts.ErrCustomFieldKeyTaken).
func (m *Module) createNewCustomFields(ctx context.Context, ts *ent.Scoped, role membership.Role, mapping csvimport.Mapping) error {
	if err := mayCreateFields(role, mapping); err != nil {
		return err
	}
	for _, f := range newCustomFields(mapping) {
		if _, err := m.contacts.CreateCustomFieldIn(ctx, ts, f); err != nil {
			return err
		}
	}
	return nil
}
