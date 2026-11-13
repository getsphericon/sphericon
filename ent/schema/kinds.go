package schema

import "entgo.io/ent/schema"

// The kinds of an entity: each mixin that gives one declares it with a marker of its own name in its
// Annotations(), which ent merges into the schema's. internal/entgen classifies every entity by its
// markers, never by its column names, checks the schema against the rules of each kind and writes
// the manifest of kinds into package ent (ent.Kinds). A singleton has no marker: it is the
// structure of the schema, a unique index on WorkspaceColumn alone.

// The columns and edges the mixins below declare, which internal/entgen reads once a marker has
// said the entity has them.
const (
	// WorkspaceColumn is WorkspaceMixin's column, the Workspace a row belongs to.
	WorkspaceColumn = "workspace_id"
	// ArchivedColumn is ArchiveMixin's stamp.
	ArchivedColumn = "archived_at"
	// DiscardedColumn is DiscardMixin's stamp.
	DiscardedColumn = "discarded_at"
	// RevisionColumn is RevisionMixin's revision.
	RevisionColumn = "revision"
	// CreatedColumn and UpdatedColumn are TimeMixin's moments.
	CreatedColumn = "created_at"
	UpdatedColumn = "updated_at"
	// VersionsEdge, LiveVersionEdge and DraftVersionEdge are VersionedMixin's edges into the
	// definition's versions.
	VersionsEdge     = "versions"
	LiveVersionEdge  = "live_version"
	DraftVersionEdge = "draft_version"
)

// WorkspaceOwned is the kind WorkspaceMixin gives: the entity belongs to a Workspace and is reached
// through the scoped client (ADR 0017).
type WorkspaceOwned struct{}

func (WorkspaceOwned) Name() string { return "WorkspaceOwned" }

// Archivable is the kind ArchiveMixin gives (ADR 0100).
type Archivable struct{}

func (Archivable) Name() string { return "Archivable" }

// Discardable is the kind DiscardMixin gives (ADR 0049, ADR 0100).
type Discardable struct{}

func (Discardable) Name() string { return "Discardable" }

// Versioned is the kind VersionedMixin gives: a Versioned definition (ADR 0065).
type Versioned struct{}

func (Versioned) Name() string { return "Versioned" }

// Revised is the kind RevisionMixin gives: the entity has a revision (ADR 0119).
type Revised struct{}

func (Revised) Name() string { return "Revised" }

// UniqueOverAll marks a unique index or a unique field of an archivable or discardable entity that
// holds over its archived or discarded rows too, and says why. Without it such an index is partial
// (entsql.IndexWhere("archived_at IS NULL"), or "discarded_at IS NULL"), so a row put aside does not
// hold its value for ever; internal/entgen refuses one that is neither.
type UniqueOverAll struct {
	Reason string
}

func (UniqueOverAll) Name() string { return "UniqueOverAll" }

// NotWorkspaceOwned marks a WorkspaceColumn an entity declares outside WorkspaceMixin, and says why
// it does not make the entity Workspace-owned: a tombstone of the Workspace that outlives its row,
// or a row that may belong to no Workspace. internal/entgen refuses the column without it.
type NotWorkspaceOwned struct {
	Reason string
}

func (NotWorkspaceOwned) Name() string { return "NotWorkspaceOwned" }

var (
	_ schema.Annotation = WorkspaceOwned{}
	_ schema.Annotation = Archivable{}
	_ schema.Annotation = Discardable{}
	_ schema.Annotation = Versioned{}
	_ schema.Annotation = Revised{}
	_ schema.Annotation = UniqueOverAll{}
	_ schema.Annotation = NotWorkspaceOwned{}
)
