package segments

// This file is the application-side adapter that binds the generic engine to the
// project's Contact entity. It is the only file in the package that imports the
// generated ent code; when extracting segments.go into a standalone library,
// this file stays behind in the app.

import (
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/tag"
)

// ContactSchema whitelists the contact fields a segment rule may target and maps
// them to columns. Tags are the "tag" field (has / doesNotHave). Custom fields are addressed as "custom:<key>"; behavioral
// conditions as "event:<action>" (joined to the events log on the stable identity
// link contact_id ↔ id + workspace, per ADR 0002 — never the email string).
func ContactSchema() Schema {
	return Schema{
		Columns: map[string]string{
			"subject_id": contact.FieldSubjectID,
			"email":      contact.FieldEmail,
			"phone":      contact.FieldPhone,
			"first_name": contact.FieldFirstName,
			"last_name":  contact.FieldLastName,
			"time_zone":  contact.FieldTimeZone,
		},
		JSONColumns: map[string]string{
			"custom:": contact.FieldCustomFields,
		},
		Events: &EventSchema{
			Table:             event.Table,
			JoinCol:           event.FieldContactID,
			WorkspaceCol:      event.FieldWorkspaceID,
			ActionCol:         event.FieldAction,
			OccurredCol:       event.FieldOccurredAt,
			OuterJoinCol:      contact.FieldID,
			OuterWorkspaceCol: contact.FieldWorkspaceID,
		},
		Tags: &TagSchema{
			JoinTable:         contact.TagsTable,
			JoinSubjectCol:    contact.TagsPrimaryKey[0],
			JoinTagCol:        contact.TagsPrimaryKey[1],
			TagTable:          tag.Table,
			TagIDCol:          tag.FieldID,
			TagNameCol:        tag.FieldName,
			TagWorkspaceCol:   tag.FieldWorkspaceID,
			OuterJoinCol:      contact.FieldID,
			OuterWorkspaceCol: contact.FieldWorkspaceID,
		},
	}
}

// ContactPredicate compiles a stored segment definition into a contact predicate
// for use in Contact.Query().Where(...).
func ContactPredicate(def string) (predicate.Contact, error) {
	g, err := Parse(def)
	if err != nil {
		return nil, err
	}
	p, err := Compile(g, ContactSchema())
	if err != nil {
		return nil, err
	}
	return predicate.Contact(p), nil
}

// ValidateContactDefinition checks a definition against the contact schema.
func ValidateContactDefinition(def string) error {
	return Validate(def, ContactSchema())
}
