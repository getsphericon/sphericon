package schema

import "entgo.io/ent/schema"

// Audited opts a Workspace-owned entity into the Audit log (ADR 0022): the generated
// scoped client wraps its create, update and delete in a transaction and publishes an
// `audit.entry` with a before/after diff for every write made under a User, API token,
// Staff member or system actor. Scopes built from a secret (ingest) are never audited.
type Audited struct {
	// Action is the entity's part of the action name `<Action>.<verb>`, e.g. "tag".
	Action string
	// NameField is the field whose value is snapshotted as the target's name, so an
	// entry stays readable after the target is deleted. Leave empty to snapshot none
	// (a Contact is identified by id only: an email or name is personal data).
	NameField string
	// NameIsURL snapshots NameField as a URL stripped to scheme, host and path: a
	// query string or userinfo may carry a secret, so it never reaches the log.
	NameIsURL bool
	// NamesOnly records the names of changed fields and never a value, on create, update
	// and delete alike (a Contact: its values are personal data, so an immutable log must
	// never hold them and erasing a Contact needs no rewrite of the log).
	NamesOnly bool
	// Always lists fields an update's diff carries even when they did not change, as
	// {from: v, to: v}: they say what kind of thing the entry is about (a Hold's reason
	// code), so a reader can classify an update that touched only another field.
	Always []string
}

func (Audited) Name() string { return "Audited" }

var _ schema.Annotation = Audited{}

// Sensitive marks a field of an Audited entity whose value must never reach the log
// (token hashes, passwords, Integration credentials, webhook signing secrets): an
// entry records only that it changed.
type Sensitive struct{}

func (Sensitive) Name() string { return "Sensitive" }

var _ schema.Annotation = Sensitive{}

// nameField is the column of the `name` that named entities declare and that Audited.NameField
// snapshots.
const nameField = "name"
