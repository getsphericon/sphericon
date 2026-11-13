package entgen

import (
	"fmt"
	"strings"

	"entgo.io/ent/entc/gen"
	"entgo.io/ent/schema/field"

	"github.com/getsphericon/sphericon/ent/schema"
)

// Rule names a rule of the generation a Problem breaks.
type Rule string

// The rules of the generation.
const (
	// RuleAnnotation: an annotation cannot be decoded into its ent/schema type.
	RuleAnnotation Rule = "annotation"
	// RuleMarkerWithoutMixin: a kind's marker comes with its mixin, which brings the column the
	// generated code reads.
	RuleMarkerWithoutMixin Rule = "marker-without-mixin"
	// RuleArchivedAndDiscarded: an entity is archived or discarded, never both (ADR 0100).
	RuleArchivedAndDiscarded Rule = "archived-and-discarded"
	// RuleVersionedKind: a Versioned definition is Workspace-owned and archivable or discardable.
	RuleVersionedKind Rule = "versioned-kind"
	// RuleVersionedLiveEdge: a Versioned definition has VersionedMixin's live_version edge.
	RuleVersionedLiveEdge Rule = "versioned-live-edge"
	// RuleVersionedOwner: its versions edge reaches a version that names its owner (VersionMixin).
	RuleVersionedOwner Rule = "versioned-owner"
	// RuleVersionedFields: a Versioned definition has a name and a revision, which its
	// VersionState carries.
	RuleVersionedFields Rule = "versioned-fields"
	// RuleEnumWithoutCheck: every enum field has the CHECK constraint named after its table and
	// column (schema.EnumCheckName), which lists its values.
	RuleEnumWithoutCheck Rule = "enum-without-check"
	// RuleGivenUpNotBool: a field a Discard gives up (schema.GivenUpOnDiscard) is a bool.
	RuleGivenUpNotBool Rule = "given-up-not-bool"
	// RuleGivenUpWithoutDiscard: schema.GivenUpOnDiscard is on a Workspace-owned discardable entity,
	// whose generated Discard gives it up.
	RuleGivenUpWithoutDiscard Rule = "given-up-without-discard"
	// RuleBookkeepingWithoutRevision: schema.Bookkeeping is on an entity with a revision (ADR 0119).
	RuleBookkeepingWithoutRevision Rule = "bookkeeping-without-revision"
	// RuleUncheckedRefReason: an unchecked schema.ScopedRef says why it has no key (rule 8).
	RuleUncheckedRefReason Rule = "unchecked-ref-reason"
	// RuleRefWithoutKey: an int64 id column is an edge field or carries schema.ScopedRef (rule 8).
	RuleRefWithoutKey Rule = "ref-without-key"
	// RuleRefUnknownEntity: schema.ScopedRef names a Workspace-owned entity of the schema.
	RuleRefUnknownEntity Rule = "ref-unknown-entity"
	// RuleAuditAction: schema.Audited has an Action (ADR 0022).
	RuleAuditAction Rule = "audit-action"
	// RuleAuditNameField: schema.Audited.NameField names a field of the entity.
	RuleAuditNameField Rule = "audit-name-field"
	// RuleAuditAlways: schema.Audited.Always names fields of the entity.
	RuleAuditAlways Rule = "audit-always"
	// RuleStateWithoutForm: every state-shaped field declares its form (ADR 0109, rule 7).
	RuleStateWithoutForm Rule = "state-without-form"
	// RuleStateUnknownForm: a declared form is a machine, a latch or an attribute.
	RuleStateUnknownForm Rule = "state-unknown-form"
	// RuleAttributeReason: an attribute says why no move depends on it.
	RuleAttributeReason Rule = "attribute-reason"
	// RuleUniqueNotPartial: a unique index or field of an archivable or discardable entity is
	// partial, or says with schema.UniqueOverAll why it holds over the rows put aside too.
	RuleUniqueNotPartial Rule = "unique-not-partial"
	// RuleWorkspaceOutsideMixin: the Workspace column comes from WorkspaceMixin, or says with
	// schema.NotWorkspaceOwned why it does not make its entity Workspace-owned.
	RuleWorkspaceOutsideMixin Rule = "workspace-outside-mixin"
)

// Problem is one rule the schema breaks: where, which rule and what to do.
type Problem struct {
	// Entity is the schema name of the entity, and Field or Index what of it the problem is about.
	Entity, Field, Index string
	Rule                 Rule
	// Message says what is wrong and how to fix it.
	Message string
}

// Where names what the problem is about: Entity, Entity.field or Entity index <name>.
func (p Problem) Where() string {
	switch {
	case p.Field != "":
		return p.Entity + "." + p.Field
	case p.Index != "":
		return p.Entity + " index " + p.Index
	default:
		return p.Entity
	}
}

func (p Problem) String() string { return fmt.Sprintf("%s: %s (%s)", p.Where(), p.Message, p.Rule) }

// check checks n against the rules of its kind and of its fields and indexes.
func (c *checker) check(n *gen.Type) {
	e := c.s.entities[n]
	k := e.kind
	c.checkMarkers(n, k)
	if k.Archivable && k.Discardable {
		c.add(Problem{Entity: n.Name, Rule: RuleArchivedAndDiscarded,
			Message: "an entity is archived or discarded, never both (ADR 0100): drop ArchiveMixin or DiscardMixin"})
	}
	if k.Versioned {
		c.checkVersioned(n, e)
	}
	for _, f := range n.Fields {
		c.checkField(n, e, f)
	}
	if k.PutAside() {
		for _, idx := range n.Indexes {
			if idx.Unique && !c.partial(n, idx) {
				mark, _ := decode[schema.UniqueOverAll](c, n.Name, idx.Annotations)
				if strings.TrimSpace(mark.Reason) == "" {
					c.add(Problem{Entity: n.Name, Index: idx.Name, Rule: RuleUniqueNotPartial, Message: uniqueNotPartial})
				}
			}
		}
	}
}

const uniqueNotPartial = "a unique index of an archivable or discardable entity holds the value of a row put aside for ever: " +
	"make it partial (entsql.IndexWhere(\"archived_at IS NULL\") or \"discarded_at IS NULL\"), or say why it holds over every row " +
	"with schema.UniqueOverAll{Reason: ...}"

// checkMarkers checks that each marker of n came with the column its mixin brings: a marker declared
// by hand, without its mixin, gives n a kind whose column the generated code reads.
func (c *checker) checkMarkers(n *gen.Type, k schema.Kind) {
	for _, m := range []struct {
		marked                bool
		marker, mixin, column string
	}{
		{k.WorkspaceOwned, schema.WorkspaceOwned{}.Name(), "WorkspaceMixin", schema.WorkspaceColumn},
		{k.Archivable, schema.Archivable{}.Name(), "ArchiveMixin", schema.ArchivedColumn},
		{k.Discardable, schema.Discardable{}.Name(), "DiscardMixin", schema.DiscardedColumn},
		{k.Revised, schema.Revised{}.Name(), "RevisionMixin", schema.RevisionColumn},
	} {
		if m.marked && mixedInField(n, m.column) == nil {
			c.add(Problem{Entity: n.Name, Rule: RuleMarkerWithoutMixin, Message: fmt.Sprintf(
				"the marker schema.%s comes with %s, which brings its %s column: take the mixin rather than its marker",
				m.marker, m.mixin, m.column)})
		}
	}
}

func (c *checker) checkVersioned(n *gen.Type, e *entity) {
	k := e.kind
	if !k.WorkspaceOwned || !k.PutAside() {
		c.add(Problem{Entity: n.Name, Rule: RuleVersionedKind,
			Message: "a Versioned definition needs WorkspaceMixin and either ArchiveMixin or DiscardMixin (ADR 0100)"})
	}
	name := fieldNamed(n, schema.NameColumn)
	if name == nil || name.Type.Type != field.TypeString || !k.Revised {
		c.add(Problem{Entity: n.Name, Rule: RuleVersionedFields,
			Message: "a Versioned definition needs a string field name and RevisionMixin: its VersionState carries both"})
	}
}

func (c *checker) checkField(n *gen.Type, e *entity, f *gen.Field) {
	a := c.s.fields[f]
	problem := func(rule Rule, message string) {
		c.add(Problem{Entity: n.Name, Field: f.Name, Rule: rule, Message: message})
	}
	c.checkForm(f, a, problem)
	if _, checked := e.checks[schema.EnumCheckName(n.Table(), f.StorageKey())]; f.IsEnum() && !checked {
		problem(RuleEnumWithoutCheck, fmt.Sprintf("an enum field is checked by the CHECK constraint %s: annotate the entity with "+
			"schema.TableWithEnumChecks(%q, ...) over its fields and its mixins' enums", schema.EnumCheckName(n.Table(), f.StorageKey()), n.Table()))
	}
	if a.givenUp && !f.IsBool() {
		problem(RuleGivenUpNotBool, "a field a Discard gives up (schema.GivenUpOnDiscard) is a bool, which the Discard sets to false")
	}
	if a.givenUp && (!e.kind.WorkspaceOwned || !e.kind.Discardable) {
		problem(RuleGivenUpWithoutDiscard, "schema.GivenUpOnDiscard is read by the generated Discard of a Workspace-owned entity "+
			"with DiscardMixin: take the mixin or drop the annotation")
	}
	if a.bookkeeping && !e.kind.Revised {
		problem(RuleBookkeepingWithoutRevision, "schema.Bookkeeping marks a column of an entity with a revision (ADR 0119): "+
			"take RevisionMixin or drop the annotation")
	}
	c.checkRef(f, a, problem)
	if e.kind.PutAside() && f.Unique && (a.unique == nil || strings.TrimSpace(a.unique.Reason) == "") {
		problem(RuleUniqueNotPartial, "a unique field of an archivable or discardable entity holds the value of a row put aside for ever: "+
			"make it a partial unique index, or say why it holds over every row with schema.UniqueOverAll{Reason: ...}")
	}
	if f.Name == schema.WorkspaceColumn && f != e.workspace && (a.notOwned == nil || strings.TrimSpace(a.notOwned.Reason) == "") {
		problem(RuleWorkspaceOutsideMixin, "a Workspace-owned entity takes WorkspaceMixin; a Workspace column outside it says why "+
			"it does not make its entity Workspace-owned with schema.NotWorkspaceOwned{Reason: ...}")
	}
}

// checkForm checks the form a state field declares (ADR 0109, review rule 7).
func (c *checker) checkForm(f *gen.Field, a *fieldAnnotations, problem func(Rule, string)) {
	if a.state == nil {
		if stateShaped(f) {
			problem(RuleStateWithoutForm, fmt.Sprintf("a %s field declares its form: annotate it with schema.Machine(), "+
				"schema.Latch() or schema.Attribute(reason)", f.Type))
		}
		return
	}
	switch a.state.Form {
	case schema.FormMachine, schema.FormLatch:
	case schema.FormAttribute:
		if strings.TrimSpace(a.state.Reason) == "" {
			problem(RuleAttributeReason, "an attribute says why no move depends on it: schema.Attribute(reason)")
		}
	default:
		problem(RuleStateUnknownForm, fmt.Sprintf("the form %q is none of machine, latch and attribute", a.state.Form))
	}
}

// stateShaped reports whether a field must declare its form (ADR 0109): a moment, a status or state
// enum, or a bool.
func stateShaped(f *gen.Field) bool {
	switch {
	case f.Type.Type == field.TypeTime && strings.HasSuffix(f.Name, "_at"):
		return true
	case f.Type.Type == field.TypeEnum && (f.Name == "status" || f.Name == "state"):
		return true
	default:
		return f.Type.Type == field.TypeBool
	}
}

// checkRef checks that a reference column has a foreign key or a written reason (rule 8, ADR 0033):
// every int64 id column is an edge field (a key) or carries schema.ScopedRef, Entity (a scoped client
// check) or Unchecked with a Reason (a tombstone or a snapshot that outlives its target).
func (c *checker) checkRef(f *gen.Field, a *fieldAnnotations, problem func(Rule, string)) {
	if a.ref != nil && a.ref.Unchecked {
		if strings.TrimSpace(a.ref.Reason) == "" {
			problem(RuleUncheckedRefReason, "an unchecked reference with no foreign key needs a Reason in schema.ScopedRef "+
				"(a tombstone or a snapshot)")
		}
		return
	}
	if f.IsEdgeField() || f.Type.Type != field.TypeInt64 || !strings.HasSuffix(f.Name, "_id") || a.ref != nil && a.ref.Entity != "" {
		return
	}
	problem(RuleRefWithoutKey, "a reference with no foreign key: add an edge, or annotate it with schema.ScopedRef{Entity: ...} "+
		"(scoped check) or {Unchecked: true, Reason: ...} (tombstone or snapshot)")
}
