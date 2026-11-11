package schema_test

import (
	"fmt"
	"strings"
	"testing"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
	"entgo.io/ent/schema/field"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/schema"
)

// TestEveryStateFieldDeclaresItsForm loads the schema as entc does and fails on a state-shaped field
// without schema.State, on an unknown form, and on an attribute without its reason (ADR 0109).
func TestEveryStateFieldDeclaresItsForm(t *testing.T) {
	graph, err := entc.LoadGraph(".", &gen.Config{})
	require.NoError(t, err)

	problems, declared := formProblems(graph)

	require.Empty(t, problems, "annotate each field with schema.Machine(), schema.Latch() or schema.Attribute(reason)")
	require.Positive(t, declared)
}

// formProblems lists what is wrong with the forms the graph declares, and counts the declared ones.
func formProblems(graph *gen.Graph) (problems []string, declared int) {
	for _, n := range graph.Nodes {
		for _, f := range n.Fields {
			raw, annotated := f.Annotations[schema.State{}.Name()].(map[string]any)
			if !annotated {
				if stateShaped(f) {
					problems = append(problems, fmt.Sprintf("%s.%s (%s) declares no form", n.Name, f.Name, f.Type))
				}
				continue
			}
			declared++
			if problem := formProblem(raw); problem != "" {
				problems = append(problems, fmt.Sprintf("%s.%s %s", n.Name, f.Name, problem))
			}
		}
	}
	return problems, declared
}

// formProblem says what is wrong with one declared form, or "" when nothing is.
func formProblem(raw map[string]any) string {
	form, _ := raw["Form"].(string)
	reason, _ := raw["Reason"].(string)
	switch schema.Form(form) {
	case schema.FormMachine, schema.FormLatch:
		return ""
	case schema.FormAttribute:
		if strings.TrimSpace(reason) == "" {
			return "is an attribute with no reason"
		}
		return ""
	default:
		return fmt.Sprintf("has the unknown form %q", form)
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
