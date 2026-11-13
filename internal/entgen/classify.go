package entgen

import (
	"encoding/json"
	"fmt"
	"slices"

	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/entc/gen"
	entschema "entgo.io/ent/schema"

	"github.com/getsphericon/sphericon/ent/schema"
)

// Kind is what the schema makes of an entity: the kinds its mixins declare with their markers
// (ent/schema/kinds.go), and Singleton, which its structure gives.
type Kind struct {
	// WorkspaceOwned: the entity takes WorkspaceMixin and is reached through the scoped client.
	WorkspaceOwned bool
	// Archivable: it takes ArchiveMixin (ADR 0100).
	Archivable bool
	// Discardable: it takes DiscardMixin (ADR 0049, ADR 0100).
	Discardable bool
	// Versioned: it takes VersionedMixin, a Versioned definition (ADR 0065).
	Versioned bool
	// Revised: it takes RevisionMixin and has a revision (ADR 0119).
	Revised bool
	// Singleton: a Workspace-owned entity with a whole unique index on its Workspace column alone,
	// so a Workspace holds at most one of its rows.
	Singleton bool
}

// Entity is one entity of the schema, its table and its kind: a line of the manifest of kinds.
type Entity struct {
	Kind

	Name, Table string
}

// Audit is the decoded schema.Audited annotation of an entity.
type Audit struct {
	Action    string
	NameField *gen.Field
	NamesOnly bool
	NameIsURL bool
	// Always lists field names an update's diff carries even when unchanged.
	Always []string
}

// ScopedRef is a plain field that references another Workspace-owned entity.
type ScopedRef struct {
	Field  *gen.Field
	Target *gen.Type
}

// Schema is the classification of a graph: every entity's kind and its decoded annotations, which
// the templates read through Funcs.
type Schema struct {
	graph    *gen.Graph
	entities map[*gen.Type]*entity
	fields   map[*gen.Field]*fieldAnnotations
}

// entity is what the classification knows of one node.
type entity struct {
	node *gen.Type
	kind Kind
	// workspace is the Workspace column of a Workspace-owned entity, revision the revision of one
	// with a revision, timestamps the moments of TimeMixin.
	workspace  *gen.Field
	revision   *gen.Field
	timestamps []*gen.Field
	audit      *Audit
	// version and versionOwner are, for a Versioned definition, its version entity and the
	// version's column that names its owner.
	version      *gen.Type
	versionOwner *gen.Field
	refs         []ScopedRef
}

// fieldAnnotations are the annotations of one field, decoded into their ent/schema types.
type fieldAnnotations struct {
	state       *schema.State
	ref         *schema.ScopedRef
	unique      *schema.UniqueOverAll
	notOwned    *schema.NotWorkspaceOwned
	givenUp     bool
	bookkeeping bool
	sensitive   bool
	derived     bool
}

// Classify classifies every entity of the graph and checks the graph against every rule of the
// generation, returning each problem it finds.
func Classify(g *gen.Graph) (*Schema, []Problem) {
	c := &checker{s: &Schema{
		graph:    g,
		entities: map[*gen.Type]*entity{},
		fields:   map[*gen.Field]*fieldAnnotations{},
	}}
	for _, n := range g.Nodes {
		c.s.entities[n] = c.classify(n)
		for _, f := range n.Fields {
			c.s.fields[f] = c.fieldAnnotations(n, f)
		}
	}
	for _, n := range g.Nodes {
		c.relate(n)
	}
	for _, n := range g.Nodes {
		c.check(n)
	}
	return c.s, c.problems
}

// Entities lists every entity of the schema with its table and kind, in the graph's order.
func (s *Schema) Entities() []Entity {
	out := make([]Entity, 0, len(s.graph.Nodes))
	for _, n := range s.graph.Nodes {
		out = append(out, Entity{Name: n.Name, Table: n.Table(), Kind: s.entities[n].kind})
	}
	return out
}

// checker gathers the problems of a classification.
type checker struct {
	s        *Schema
	problems []Problem
}

func (c *checker) add(p Problem) { c.problems = append(c.problems, p) }

// classify reads the kind of n from its markers and its structure.
func (c *checker) classify(n *gen.Type) *entity {
	e := &entity{node: n}
	e.kind.WorkspaceOwned = marked(n, schema.WorkspaceOwned{})
	e.kind.Archivable = marked(n, schema.Archivable{})
	e.kind.Discardable = marked(n, schema.Discardable{})
	e.kind.Versioned = marked(n, schema.Versioned{})
	e.kind.Revised = marked(n, schema.Revised{})
	for _, f := range n.Fields {
		mixedIn := f.Position != nil && f.Position.MixedIn
		switch {
		case e.kind.WorkspaceOwned && mixedIn && f.Name == schema.WorkspaceColumn:
			e.workspace = f
		case e.kind.Revised && mixedIn && f.Name == schema.RevisionColumn:
			e.revision = f
		case mixedIn && (f.Name == schema.CreatedColumn || f.Name == schema.UpdatedColumn):
			e.timestamps = append(e.timestamps, f)
		}
	}
	e.kind.Singleton = e.workspace != nil && slices.ContainsFunc(n.Indexes, func(idx *gen.Index) bool {
		return idx.Unique && slices.Equal(idx.Columns, []string{e.workspace.StorageKey()}) && !c.partial(n, idx)
	})
	return e
}

// marked reports whether n carries the marker.
func marked(n *gen.Type, marker entschema.Annotation) bool {
	_, ok := n.Annotations[marker.Name()]
	return ok
}

// partial reports whether the index has a WHERE clause.
func (c *checker) partial(n *gen.Type, idx *gen.Index) bool {
	where, ok := decode[entsql.IndexAnnotation](c, n.Name, idx.Annotations)
	return ok && where.Where != ""
}

// fieldAnnotations decodes the annotations of the field f of n.
func (c *checker) fieldAnnotations(n *gen.Type, f *gen.Field) *fieldAnnotations {
	a := &fieldAnnotations{}
	where := n.Name + "." + f.Name
	if v, ok := decode[schema.State](c, where, f.Annotations); ok {
		a.state = &v
	}
	if v, ok := decode[schema.ScopedRef](c, where, f.Annotations); ok {
		a.ref = &v
	}
	if v, ok := decode[schema.UniqueOverAll](c, where, f.Annotations); ok {
		a.unique = &v
	}
	if v, ok := decode[schema.NotWorkspaceOwned](c, where, f.Annotations); ok {
		a.notOwned = &v
	}
	_, a.givenUp = f.Annotations[schema.GivenUpOnDiscard{}.Name()]
	_, a.bookkeeping = f.Annotations[schema.Bookkeeping{}.Name()]
	_, a.sensitive = f.Annotations[schema.Sensitive{}.Name()]
	_, a.derived = f.Annotations[schema.Derived{}.Name()]
	return a
}

// decode reads the annotation of type T from anns, as ent decoded it from the schema; a value it
// cannot read is a problem of where.
func decode[T entschema.Annotation](c *checker, where string, anns gen.Annotations) (T, bool) {
	var v T
	raw, ok := anns[v.Name()]
	if !ok {
		return v, false
	}
	b, err := json.Marshal(raw)
	if err == nil {
		err = json.Unmarshal(b, &v)
	}
	if err != nil {
		c.add(Problem{Entity: where, Rule: RuleAnnotation, Message: fmt.Sprintf("annotation %s cannot be read: %v", v.Name(), err)})
		return v, false
	}
	return v, true
}

// relate resolves what n's annotations name in the rest of the graph: its audit, its references and
// the versions of a Versioned definition.
func (c *checker) relate(n *gen.Type) {
	e := c.s.entities[n]
	if a, ok := decode[schema.Audited](c, n.Name, n.Annotations); ok {
		e.audit = c.audit(n, a)
	}
	for _, f := range n.Fields {
		if t := c.refTarget(n, f); t != nil {
			e.refs = append(e.refs, ScopedRef{Field: f, Target: t})
		}
	}
	if !e.kind.Versioned {
		return
	}
	for _, edge := range n.Edges {
		switch edge.Name {
		case schema.LiveVersionEdge:
			e.version = edge.Type
		case schema.VersionsEdge:
			if edge.Ref != nil {
				e.versionOwner = edge.Ref.Field()
			}
		}
	}
	if e.version == nil {
		c.add(Problem{Entity: n.Name, Rule: RuleVersionedLiveEdge,
			Message: "a Versioned definition needs the live_version edge of VersionedMixin: take the mixin rather than its marker"})
	}
	if e.versionOwner == nil {
		c.add(Problem{Entity: n.Name, Rule: RuleVersionedOwner,
			Message: "a Versioned definition needs the versions edge of VersionedMixin, and its version the owner edge of VersionMixin"})
	}
}

// audit resolves the Audited annotation a of n, a problem for each name it gets wrong.
func (c *checker) audit(n *gen.Type, a schema.Audited) *Audit {
	out := &Audit{Action: a.Action, NamesOnly: a.NamesOnly, NameIsURL: a.NameIsURL}
	if a.Action == "" {
		c.add(Problem{Entity: n.Name, Rule: RuleAuditAction, Message: "schema.Audited needs an Action"})
	}
	for _, name := range a.Always {
		if fieldNamed(n, name) == nil {
			c.add(Problem{Entity: n.Name, Rule: RuleAuditAlways, Message: fmt.Sprintf("schema.Audited.Always names an unknown field %q", name)})
			continue
		}
		out.Always = append(out.Always, name)
	}
	if a.NameField != "" {
		out.NameField = fieldNamed(n, a.NameField)
		if out.NameField == nil {
			c.add(Problem{Entity: n.Name, Rule: RuleAuditNameField, Message: fmt.Sprintf("schema.Audited names an unknown field %q", a.NameField)})
		}
	}
	return out
}

// refTarget is the entity the schema.ScopedRef of the plain field f names, or nil.
func (c *checker) refTarget(n *gen.Type, f *gen.Field) *gen.Type {
	ref := c.s.fields[f].ref
	if ref == nil || ref.Entity == "" {
		return nil
	}
	for _, t := range c.s.graph.Nodes {
		if t.Name == ref.Entity {
			if !c.s.entities[t].kind.WorkspaceOwned {
				c.add(Problem{Entity: n.Name, Field: f.Name, Rule: RuleRefUnknownEntity,
					Message: fmt.Sprintf("schema.ScopedRef names %q, which is not Workspace-owned: the scoped client checks only a Workspace-owned entity", ref.Entity)})
				return nil
			}
			return t
		}
	}
	c.add(Problem{Entity: n.Name, Field: f.Name, Rule: RuleRefUnknownEntity,
		Message: fmt.Sprintf("schema.ScopedRef names an unknown entity %q", ref.Entity)})
	return nil
}

func fieldNamed(n *gen.Type, name string) *gen.Field {
	for _, f := range n.Fields {
		if f.Name == name {
			return f
		}
	}
	return nil
}
