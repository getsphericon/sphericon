// Package contractcopy is rule 6 ("knowledge has one place") for the values a TypeSpec contract
// declares: it reads the generated openapi/*.openapi.json for its enums and numeric limits and
// reports a hand-written Go or TypeScript copy of them, and a fixture scope the contract does not
// declare. Go is read through go/parser, TypeScript through patterns (the frontend has no Go types
// to ask).
//
// Typed Go const blocks are the domain's own vocabulary and are not read.
//
// A value that only looks like a contract value (an OAuth limit, an internal cap) carries
// `// contract-copy: <reason>` on its line or the line above; a bare marker is still a finding.
package contractcopy

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// marker is the annotation that documents a value as not being a contract copy.
const marker = "contract-copy:"

// fix is the one remedy every finding ends with.
const fix = "generate it from TypeSpec"

// Contract is what the OpenAPI documents declare: every enum (a sorted set of values) and every
// numeric limit.
type Contract struct {
	Enums  [][]string
	Limits map[int64]bool
}

func message(what string) string {
	return fmt.Sprintf("rule 6: %s; a value that is not in the contract carries `// %s <reason>`; fix: %s",
		what, marker, fix)
}

// Check loads the contract from specDir and scans root for copies.
func Check(root, specDir string) ([]string, error) {
	c, err := LoadContract(filepath.Join(root, specDir))
	if err != nil {
		return nil, err
	}
	var out []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if skipDir(rel, d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if skipFile(rel) {
			return nil
		}
		var found []string
		switch filepath.Ext(path) {
		case ".go":
			found, err = scanGo(path, c)
		case ".ts", ".tsx":
			found, err = scanTS(path, c)
		case ".yml":
			if strings.HasPrefix(rel, "fixtures/") {
				found, err = scanFixture(path, c)
			}
		}
		if err != nil {
			return err
		}
		out = append(out, found...)
		return nil
	})
	return out, err
}

// LoadContract reads every *.openapi.json under dir.
func LoadContract(dir string) (Contract, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.openapi.json"))
	if err != nil {
		return Contract{}, err
	}
	if len(files) == 0 {
		return Contract{}, fmt.Errorf("no OpenAPI documents in %s", dir)
	}
	c := Contract{Limits: map[int64]bool{}}
	seen := map[string]bool{}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			return c, err
		}
		var doc any
		if err := json.Unmarshal(raw, &doc); err != nil {
			return c, fmt.Errorf("%s: %w", f, err)
		}
		c.collect(doc, seen)
	}
	return c, nil
}

var limitKeys = []string{"maxLength", "minLength", "maximum", "minimum", "maxItems", "minItems", "default"}

func (c *Contract) collect(node any, seen map[string]bool) {
	switch n := node.(type) {
	case map[string]any:
		if raw, ok := n["enum"].([]any); ok {
			var vals []string
			for _, v := range raw {
				if s, ok := v.(string); ok {
					vals = append(vals, s)
				}
			}
			slices.Sort(vals)
			key := strings.Join(vals, "\x00")
			if len(vals) >= 2 && !seen[key] {
				seen[key] = true
				c.Enums = append(c.Enums, vals)
			}
		}
		for _, k := range limitKeys {
			if f, ok := n[k].(float64); ok && f == float64(int64(f)) && f > 1 {
				c.Limits[int64(f)] = true
			}
		}
		for _, v := range n {
			c.collect(v, seen)
		}
	case []any:
		for _, v := range n {
			c.collect(v, seen)
		}
	}
}

// copiesEnum reports whether the literals restate a contract enum: all of its values, or three or
// more that are at least a quarter of a larger one (a pair of values shared by chance is not a copy).
func (c *Contract) copiesEnum(values []string) bool {
	set := map[string]bool{}
	for _, l := range values {
		set[l] = true
	}
	for _, e := range c.Enums {
		hit := 0
		for _, v := range e {
			if set[v] {
				hit++
			}
		}
		if hit == len(e) || hit >= 3 && hit*4 >= len(e) {
			return true
		}
	}
	return false
}

// scopes is every value of an enum that looks like a token scope (`resource:action`), used to
// validate fixture data, which cannot import the generated type.
func (c *Contract) scopes() map[string]bool {
	out := map[string]bool{}
	for _, e := range c.Enums {
		if slices.ContainsFunc(e, func(v string) bool { return strings.Contains(v, ":") }) {
			for _, v := range e {
				out[v] = true
			}
		}
	}
	return out
}

func skipDir(rel, name string) bool {
	switch name {
	case "node_modules", ".git", "testdata", "dist", ".claude", ".bin", "worktrees":
		return true
	}
	switch rel {
	case "openapi", "typespec", "gen", "src/generated", "internal/emaildoc/assets":
		return true
	}
	if strings.HasPrefix(rel, "ent/") && rel != "ent/schema" {
		return true
	}
	return strings.HasSuffix(rel, "/src/generated")
}

func skipFile(rel string) bool {
	if strings.HasSuffix(rel, "_gen.go") || strings.HasSuffix(rel, "_gen_test.go") ||
		strings.HasSuffix(rel, ".gen.ts") || strings.HasSuffix(rel, ".d.ts") {
		return true
	}
	return strings.HasPrefix(rel, "ent/") && !strings.HasPrefix(rel, "ent/schema/")
}
