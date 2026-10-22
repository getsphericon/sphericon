//go:build ignore

package main

import (
	"log"
	"slices"
	"strings"
	"text/template"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
	"entgo.io/ent/schema/field"
)

func main() {
	err := entc.Generate("./schema", &gen.Config{
		Target:  ".",
		Package: "github.com/getsphericon/sphericon/ent",
		IDType:  &field.TypeInfo{Type: field.TypeInt64},
		Features: []gen.Feature{
			gen.FeatureSnapshot,
			gen.FeatureUpsert,
			// Modifier exposes .Modify() on query builders, used by the analytics
			// dashboard for date_trunc day-bucketed engagement aggregates.
			gen.FeatureModifier,
			// Lock exposes .ForUpdate() on query builders: accounts locks the Workspace's
			// owner rows so the Owner invariant holds under concurrent role changes.
			gen.FeatureLock,
			// ExecQuery exposes ExecContext on the client: internal/workspacefeed takes
			// its advisory lock and the next feed position in raw SQL.
			gen.FeatureExecQuery,
			// VersionedMigration generates migrate.NamedDiff, which cmd/db uses to write
			// the next migration straight from the ent schema.
			gen.FeatureVersionedMigration,
		},
		// The scoped client (ADR 0017): ent/template/scoped*.tmpl generate ent/scoped*.go.
		Templates: []*gen.Template{
			gen.MustParse(gen.NewTemplate("scoped").Funcs(scopedFuncs()).ParseDir("template")),
		},
	})
	if err != nil {
		log.Fatal("running ent codegen:", err)
	}
}

// scopedRef is a plain field that references another Workspace-owned entity.
type scopedRef struct {
	Field  *gen.Field
	Target *gen.Type
}

// scopedFuncs are the helpers ent/template/scoped.tmpl needs to find the
// workspace-mixin entities and the references between them.
func scopedFuncs() template.FuncMap {
	return template.FuncMap{
		// scopedNodes lists the entities that carry the workspace mixin.
		"scopedNodes": func(g *gen.Graph) []*gen.Type {
			checkScopedRefs(g)
			var out []*gen.Type
			for _, n := range g.Nodes {
				if isScoped(n) {
					out = append(out, n)
				}
			}
			return out
		},
		"scopedIs": isScoped,
		// discardable reports whether n embeds the DiscardMixin (ADR 0049): its scoped
		// wrapper gets Discard, Undiscard and Purge instead of Delete.
		"discardable": func(n *gen.Type) bool { return isScoped(n) && isDiscardable(n) },
		// scopedEdges lists the edges of n that point at another scoped entity.
		"scopedEdges": func(n *gen.Type) []*gen.Edge {
			var out []*gen.Edge
			for _, e := range n.EdgesWithID() {
				if isScoped(e.Type) {
					out = append(out, e)
				}
			}
			return out
		},
		// scopedEdgeMutable reports whether the edge can be changed on update.
		"scopedEdgeMutable": func(e *gen.Edge) bool {
			if e.Immutable {
				return false
			}
			f := e.Field()
			return f == nil || !f.Immutable
		},
		// auditOf returns the Audited annotation of n (ADR 0022), or nil when the
		// entity has not opted into the Audit log.
		"auditOf": auditOf,
		// auditFields lists the fields of an audited entity that appear in a diff.
		"auditFields": func(n *gen.Type) []*gen.Field {
			var out []*gen.Field
			for _, f := range n.Fields {
				switch f.Name {
				case "workspace_id", "created_at", "updated_at":
					continue
				}
				out = append(out, f)
			}
			return out
		},
		"auditSensitive": func(f *gen.Field) bool {
			_, ok := f.Annotations["Sensitive"]
			return ok
		},
		// scopedFieldRefs lists the annotated plain fields of n that hold a reference.
		"scopedFieldRefs": func(g *gen.Graph, n *gen.Type) []scopedRef {
			var out []scopedRef
			for _, f := range n.Fields {
				if t := annotatedTarget(g, f); t != nil {
					out = append(out, scopedRef{Field: f, Target: t})
				}
			}
			return out
		},
		// scopedRefTarget returns the scoped entity a field references (an edge
		// field or an annotated plain field), or nil.
		"scopedRefTarget": func(g *gen.Graph, f *gen.Field) *gen.Type {
			if f.IsEdgeField() {
				if e, err := f.Edge(); err == nil && isScoped(e.Type) {
					return e.Type
				}
				return nil
			}
			return annotatedTarget(g, f)
		},
	}
}

// checkScopedRefs fails the generation when a reference column has no foreign key and no written
// reason (rule 8, ADR 0033). Every Int64 id column of any entity must be an edge field (a key) or carry
// schema.ScopedRef: Entity (a scoped client check) or Unchecked with a Reason (a tombstone or a snapshot
// that outlives its target). Unchecked without a Reason fails on any field, string snapshots included.
func checkScopedRefs(g *gen.Graph) {
	for _, n := range g.Nodes {
		for _, f := range n.Fields {
			ann, annotated := f.Annotations["ScopedRef"].(map[string]any)
			if annotated && ann["Unchecked"] == true {
				if reason, _ := ann["Reason"].(string); strings.TrimSpace(reason) == "" {
					log.Fatalf("%s.%s is an unchecked reference with no foreign key: schema.ScopedRef needs a Reason (tombstone or snapshot)", n.Name, f.Name)
				}
				continue
			}
			if f.Name == "workspace_id" && f.Position != nil && f.Position.MixedIn {
				continue
			}
			if f.IsEdgeField() || f.Type.Type != field.TypeInt64 || !strings.HasSuffix(f.Name, "_id") {
				continue
			}
			if annotatedTarget(g, f) == nil {
				log.Fatalf("%s.%s looks like a reference with no foreign key: add an edge, or annotate it with schema.ScopedRef{Entity: ...} (scoped check) or {Unchecked: true, Reason: ...} (tombstone or snapshot)", n.Name, f.Name)
			}
		}
	}
}

// audit is the decoded schema.Audited annotation of an entity.
type audit struct {
	Action    string
	NameField *gen.Field
	NamesOnly bool
	NameIsURL bool
	// Always lists field names an update's diff carries even when unchanged.
	Always []string
}

func auditOf(n *gen.Type) *audit {
	ann, ok := n.Annotations["Audited"].(map[string]any)
	if !ok {
		return nil
	}
	a := &audit{}
	a.Action, _ = ann["Action"].(string)
	a.NamesOnly, _ = ann["NamesOnly"].(bool)
	a.NameIsURL, _ = ann["NameIsURL"].(bool)
	always, _ := ann["Always"].([]any)
	for _, v := range always {
		name, _ := v.(string)
		if !slices.ContainsFunc(n.Fields, func(f *gen.Field) bool { return f.Name == name }) {
			log.Fatalf("%s: schema.Audited.Always names an unknown field %q", n.Name, name)
		}
		a.Always = append(a.Always, name)
	}
	if a.Action == "" {
		log.Fatalf("%s: schema.Audited needs an Action", n.Name)
	}
	if name, _ := ann["NameField"].(string); name != "" {
		for _, f := range n.Fields {
			if f.Name == name {
				a.NameField = f
			}
		}
		if a.NameField == nil {
			log.Fatalf("%s: schema.Audited names an unknown field %q", n.Name, name)
		}
	}
	return a
}

func isScoped(t *gen.Type) bool {
	for _, f := range t.Fields {
		if f.Name == "workspace_id" && f.Position != nil && f.Position.MixedIn {
			return true
		}
	}
	return false
}

func isDiscardable(t *gen.Type) bool {
	for _, f := range t.Fields {
		if f.Name == "discarded_at" && f.Position != nil && f.Position.MixedIn {
			return true
		}
	}
	return false
}

func annotatedTarget(g *gen.Graph, f *gen.Field) *gen.Type {
	ann, ok := f.Annotations["ScopedRef"].(map[string]any)
	if !ok {
		return nil
	}
	name, _ := ann["Entity"].(string)
	if name == "" {
		return nil
	}
	for _, n := range g.Nodes {
		if n.Name == name {
			return n
		}
	}
	log.Fatalf("ScopedRef on %s names an unknown entity %q", f.Name, name)
	return nil
}
