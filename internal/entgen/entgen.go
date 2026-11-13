// Package entgen holds the rules of ent generation (docs/agents/codegen.md). It classifies every
// entity of the ent schema once, by the kinds its mixins declare with their markers
// (ent/schema/kinds.go) and by the structure of its schema; it decodes each annotation into its
// ent/schema type once; and it checks the schema against every rule, returning each problem rather
// than the first. ent/entc.go runs it through Hook before anything is written, and the templates of
// ent/template read the classification through Funcs, which have no side effects.
//
// It never imports the generated ent package: generation must not depend on its previous output
// compiling.
package entgen

import (
	"errors"
	"fmt"

	"entgo.io/ent/entc/gen"
	"entgo.io/ent/schema/field"
)

// Config is the part of the generation's configuration that shapes the graph: every entity's id is
// an int64. ent/entc.go adds where and what to generate; a test loads a schema with it as is.
func Config() *gen.Config {
	return &gen.Config{IDType: &field.TypeInfo{Type: field.TypeInt64}}
}

// Hook is the generation's hook: it classifies and checks the graph about to be generated, fails the
// generation with every problem it found, and otherwise parses the templates of templateDir with
// the classification's Funcs and adds them to the generation.
func Hook(templateDir string) gen.Hook {
	return func(next gen.Generator) gen.Generator {
		return gen.GenerateFunc(func(g *gen.Graph) error {
			s, problems := Classify(g)
			if len(problems) > 0 {
				errs := make([]error, len(problems))
				for i, p := range problems {
					errs[i] = errors.New(p.String())
				}
				return fmt.Errorf("checking ent/schema against the rules of generation: %w", errors.Join(errs...))
			}
			t, err := gen.NewTemplate("scoped").Funcs(s.Funcs()).ParseDir(templateDir)
			if err != nil {
				return fmt.Errorf("parsing %s: %w", templateDir, err)
			}
			g.Templates = append(g.Templates, t)
			return next.Generate(g)
		})
	}
}
