package entgen

import (
	"fmt"
	"slices"
	"text/template"

	"entgo.io/ent/entc/gen"

	"github.com/getsphericon/sphericon/ent/schema"
)

// Funcs are the helpers the templates of ent/template generate from: they read the classification
// of the one graph it was made from and change nothing.
func (s *Schema) Funcs() template.FuncMap {
	return template.FuncMap{
		// entities lists every entity with its table and kind: the manifest of kinds.
		"entities": s.Entities,
		// scopedNodes lists the Workspace-owned entities.
		"scopedNodes": func() []*gen.Type { return s.nodes(func(k schema.Kind) bool { return k.WorkspaceOwned }) },
		"scopedIs":    func(t *gen.Type) bool { return s.kind(t).WorkspaceOwned },
		// kindOf is the kind of n.
		"kindOf": s.kind,
		// discardable reports whether n is a Workspace-owned entity with DiscardMixin (ADR 0049,
		// ADR 0100): its scoped wrapper gets Discard and Purge instead of Delete.
		"discardable": func(n *gen.Type) bool { k := s.kind(n); return k.WorkspaceOwned && k.Discardable },
		// archivable reports whether n is a Workspace-owned entity with ArchiveMixin (ADR 0100): its
		// scoped wrapper gets Archive, Unarchive and Purge instead of Delete.
		"archivable": func(n *gen.Type) bool { k := s.kind(n); return k.WorkspaceOwned && k.Archivable },
		// versioned reports whether n is a Versioned definition (ADR 0065): its scoped wrapper gets
		// the pointer operations (<N>Pointers) that internal/drafts runs the lifecycle on.
		"versioned": func(n *gen.Type) bool { return s.kind(n).Versioned },
		// versionedNodes lists the Versioned definitions.
		"versionedNodes": func() []*gen.Type { return s.nodes(func(k schema.Kind) bool { return k.Versioned }) },
		// versionOf returns the version entity of a Versioned definition.
		"versionOf": func(n *gen.Type) *gen.Type { return s.entities[n].version },
		// versionOwnerField returns the version's column that names its owner (automation_id).
		"versionOwnerField": func(n *gen.Type) *gen.Field { return s.entities[n].versionOwner },
		// isWorkspaceField reports whether f is the Workspace column of a Workspace-owned entity,
		// which the scoped client sets itself and never exposes a setter for.
		"isWorkspaceField":  s.isWorkspaceField,
		"replantFields":     s.replantFields,
		"scopedEdges":       s.scopedEdges,
		"scopedEdgeMutable": scopedEdgeMutable,
		// auditOf returns the Audited annotation of n (ADR 0022), or nil when the entity has not
		// opted into the Audit log.
		"auditOf": func(n *gen.Type) *Audit { return s.entities[n].audit },
		// auditVerbs lists the verbs of the actions an audited entity's entries record (Verbs), none
		// for an entity that has not opted into the Audit log.
		"auditVerbs": func(n *gen.Type) []string {
			if s.entities[n].audit == nil {
				return nil
			}
			return Verbs(s.kind(n))
		},
		// verb names a write of n by one of its Verbs, and fails the generation on a verb n has not.
		"verb": func(n *gen.Type, verb string) (string, error) {
			if !slices.Contains(Verbs(s.kind(n)), verb) {
				return "", fmt.Errorf("%s has no verb %q", n.Name, verb)
			}
			return verb, nil
		},
		"auditFields": s.auditFields,
		// auditIsKey reports whether an audited field is a reference to another row (an edge
		// field of int64), which the Audit log records by its public form (ADR 0142).
		"auditIsKey":     func(f *gen.Field) bool { return f.IsEdgeField() && f.Type.String() == "int64" },
		"auditSensitive": func(f *gen.Field) bool { return s.fields[f].sensitive },
		"stateForm":      s.stateForm,
		// discardClears lists the bools of n a Discard gives up (schema.GivenUpOnDiscard), which the
		// generated Discard sets to false.
		"discardClears": func(n *gen.Type) []*gen.Field {
			return s.fieldsOf(n, func(a *fieldAnnotations) bool { return a.givenUp })
		},
		"bookkeeping": s.bookkeeping,
		// scopedFieldRefs lists the annotated plain fields of n that hold a reference.
		"scopedFieldRefs": func(n *gen.Type) []ScopedRef { return s.entities[n].refs },
		"scopedRefTarget": s.scopedRefTarget,
	}
}

// Verbs are the verbs of the writes of an entity of the kind, which the action of an Audit entry
// names (`<entity>.<verb>`, ADR 0022): create, update and delete, then discard for a discardable
// entity and archive and unarchive for an archivable one (ADR 0100). The scoped wrappers, the
// Versioned definitions' pointers and the registry of the scoped client name their verbs from here.
func Verbs(k schema.Kind) []string {
	verbs := []string{"create", "update", "delete"}
	if k.Discardable {
		verbs = append(verbs, "discard")
	}
	if k.Archivable {
		verbs = append(verbs, "archive", "unarchive")
	}
	return verbs
}

// kind is the kind of n.
func (s *Schema) kind(n *gen.Type) schema.Kind {
	if e, ok := s.entities[n]; ok {
		return e.kind
	}
	return schema.Kind{}
}

// nodes lists the entities whose kind matches, in the graph's order.
func (s *Schema) nodes(match func(schema.Kind) bool) []*gen.Type {
	var out []*gen.Type
	for _, n := range s.graph.Nodes {
		if match(s.entities[n].kind) {
			out = append(out, n)
		}
	}
	return out
}

// fieldsOf lists the fields of n whose annotations match.
func (s *Schema) fieldsOf(n *gen.Type, match func(*fieldAnnotations) bool) []*gen.Field {
	var out []*gen.Field
	for _, f := range n.Fields {
		if match(s.fields[f]) {
			out = append(out, f)
		}
	}
	return out
}

func (s *Schema) isWorkspaceField(f *gen.Field) bool {
	for _, e := range s.entities {
		if e.workspace == f {
			return true
		}
	}
	return false
}

// replantFields lists the fields of n a re-created row copies: all but its Workspace column and
// TimeMixin's moments.
func (s *Schema) replantFields(n *gen.Type) []*gen.Field {
	e := s.entities[n]
	return slices.DeleteFunc(slices.Clone(n.Fields), func(f *gen.Field) bool {
		return f == e.workspace || slices.Contains(e.timestamps, f)
	})
}

// scopedEdges lists the edges of n that point at another Workspace-owned entity and are set on n's
// builders: not a many-to-many edge through a Workspace-owned edge table, whose rows are written as
// rows of their own.
func (s *Schema) scopedEdges(n *gen.Type) []*gen.Edge {
	var out []*gen.Edge
	for _, e := range n.EdgesWithID() {
		if s.kind(e.Type).WorkspaceOwned && (e.Through == nil || !s.kind(e.Through).WorkspaceOwned) {
			out = append(out, e)
		}
	}
	return out
}

// scopedEdgeMutable reports whether the edge can be changed on update.
func scopedEdgeMutable(e *gen.Edge) bool {
	if e.Immutable {
		return false
	}
	f := e.Field()
	return f == nil || !f.Immutable
}

// auditFields lists the fields of an audited entity that appear in a diff: not the Workspace
// column, TimeMixin's moments, the revision or a schema.Derived field.
func (s *Schema) auditFields(n *gen.Type) []*gen.Field {
	e := s.entities[n]
	return slices.DeleteFunc(slices.Clone(n.Fields), func(f *gen.Field) bool {
		return f == e.workspace || f == e.revision || slices.Contains(e.timestamps, f) || s.fields[f].derived
	})
}

// stateForm returns the form schema.State declares for a machine or latch field (ADR 0109), and ""
// for an attribute or an unannotated field.
func (s *Schema) stateForm(f *gen.Field) string {
	st := s.fields[f].state
	if st == nil || st.Form != schema.FormMachine && st.Form != schema.FormLatch {
		return ""
	}
	return string(st.Form)
}

// bookkeeping lists the columns of n that schema.Bookkeeping marks (ADR 0119).
func (s *Schema) bookkeeping(n *gen.Type) []string {
	kept := s.fieldsOf(n, func(a *fieldAnnotations) bool { return a.bookkeeping })
	if len(kept) == 0 {
		return nil
	}
	out := make([]string, 0, len(kept))
	for _, f := range kept {
		out = append(out, f.StorageKey())
	}
	return out
}

// scopedRefTarget returns the Workspace-owned entity a field references (an edge field or an
// annotated plain field), or nil.
func (s *Schema) scopedRefTarget(f *gen.Field) *gen.Type {
	if f.IsEdgeField() {
		if e, err := f.Edge(); err == nil && s.kind(e.Type).WorkspaceOwned {
			return e.Type
		}
		return nil
	}
	for _, e := range s.entities {
		for _, r := range e.refs {
			if r.Field == f {
				return r.Target
			}
		}
	}
	return nil
}
