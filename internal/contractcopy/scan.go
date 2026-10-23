package contractcopy

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// limitName matches the name of a constant that reads as a bound.
var limitName = regexp.MustCompile(`(?i)^(max|min|default)|(max|min|limit)(imum)?$`)

type finding struct {
	line int
	text string
}

func format(path string, fs []finding) []string {
	sort.SliceStable(fs, func(i, j int) bool { return fs[i].line < fs[j].line })
	out := make([]string, 0, len(fs))
	for _, f := range fs {
		out = append(out, fmt.Sprintf("%s:%d: %s", path, f.line, f.text))
	}
	return out
}

// documented reports whether a `contract-copy: <reason>` comment with a reason sits on the line or
// the line above.
func documented(lines map[int]bool, line int) bool {
	return lines[line] || lines[line-1]
}

var markerLine = regexp.MustCompile(`contract-copy:\s*\S`)

func scanGo(path string, c Contract) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	notes := markerLines(fset, file)
	var found []finding
	flagSet := func(pos token.Pos, values []string, what string) {
		line := fset.Position(pos).Line
		if c.copiesEnum(values) && !documented(notes, line) {
			found = append(found, finding{line, message(what)})
		}
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CompositeLit:
			flagSet(n.Pos(), literalKeys(n.Elts), "a list of literals repeats a TypeSpec enum")
		case *ast.CaseClause:
			flagSet(n.Pos(), stringValues(n.List), "a case list repeats a TypeSpec enum")
		case *ast.ValueSpec:
			found = append(found, limitFindings(fset, n, c, notes)...)
		}
		return true
	})
	return format(path, found), nil
}

// markerLines are the lines a `contract-copy: <reason>` comment covers: its own and, for a
// comment group, the line after its last line.
func markerLines(fset *token.FileSet, file *ast.File) map[int]bool {
	notes := map[int]bool{}
	for _, cg := range file.Comments {
		for _, cm := range cg.List {
			if markerLine.MatchString(cm.Text) {
				notes[fset.Position(cm.End()).Line] = true
				notes[fset.Position(cm.Pos()).Line] = true
			}
		}
	}
	return notes
}

// literalKeys are the string literals a composite literal lists: its elements, or the keys of a
// map's entries.
func literalKeys(elts []ast.Expr) []string {
	var values []string
	for _, e := range elts {
		if kv, ok := e.(*ast.KeyValueExpr); ok {
			e = kv.Key
		}
		if s, ok := stringLit(e); ok {
			values = append(values, s)
		}
	}
	return values
}

// stringValues are the string literals among exprs.
func stringValues(exprs []ast.Expr) []string {
	var values []string
	for _, e := range exprs {
		if s, ok := stringLit(e); ok {
			values = append(values, s)
		}
	}
	return values
}

// limitFindings are the constants of a declaration named like a limit whose integer value
// repeats one the contract declares.
func limitFindings(fset *token.FileSet, vs *ast.ValueSpec, c Contract, notes map[int]bool) []finding {
	var found []finding
	for i, name := range vs.Names {
		if i >= len(vs.Values) || !limitName.MatchString(name.Name) {
			continue
		}
		lit, ok := vs.Values[i].(*ast.BasicLit)
		if !ok || lit.Kind != token.INT {
			continue
		}
		v, err := strconv.ParseInt(strings.ReplaceAll(lit.Value, "_", ""), 0, 64)
		line := fset.Position(vs.Pos()).Line
		if err == nil && c.Limits[v] && !documented(notes, line) {
			found = append(found, finding{line, message(
				fmt.Sprintf("%s = %d repeats a limit TypeSpec declares", name.Name, v))})
		}
	}
	return found
}

func stringLit(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	return s, err == nil
}

var (
	tsString   = regexp.MustCompile(`'([^'\\\n]*)'|"([^"\\\n]*)"`)
	tsArray    = regexp.MustCompile(`\[\s*(?:(?:'[^'\\\n]*'|"[^"\\\n]*")\s*,?\s*)+\]`)
	tsUnion    = regexp.MustCompile(`(?:'[^'\\\n]*'|"[^"\\\n]*")(?:\s*\|\s*(?:'[^'\\\n]*'|"[^"\\\n]*"))+`)
	tsLimit    = regexp.MustCompile(`\b((?:MAX|MIN|DEFAULT)_[A-Z_]+|max|min|maxLength|minLength)\s*(?:=|=\{|:)\s*(\d[\d_]*)\b`)
	tsNoteLine = regexp.MustCompile(`contract-copy:\s*\S`)
)

func scanTS(path string, c Contract) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	src := string(raw)
	notes := map[int]bool{}
	for i, l := range strings.Split(src, "\n") {
		if tsNoteLine.MatchString(l) {
			notes[i+1] = true
		}
	}
	lineOf := func(off int) int { return strings.Count(src[:off], "\n") + 1 }
	var found []finding
	for _, re := range []*regexp.Regexp{tsArray, tsUnion} {
		for _, m := range re.FindAllStringIndex(src, -1) {
			var values []string
			for _, s := range tsString.FindAllStringSubmatch(src[m[0]:m[1]], -1) {
				values = append(values, s[1]+s[2])
			}
			line := lineOf(m[0])
			if c.copiesEnum(values) && !documented(notes, line) {
				found = append(found, finding{line, message("a literal list or union repeats a TypeSpec enum")})
			}
		}
	}
	for _, m := range tsLimit.FindAllStringSubmatchIndex(src, -1) {
		v, err := strconv.ParseInt(strings.ReplaceAll(src[m[4]:m[5]], "_", ""), 10, 64)
		line := lineOf(m[0])
		if err == nil && c.Limits[v] && !documented(notes, line) {
			found = append(found, finding{line, message(
				fmt.Sprintf("%s = %d repeats a limit TypeSpec declares", src[m[2]:m[3]], v))})
		}
	}
	return format(path, found), nil
}

var fixtureScope = regexp.MustCompile(`[a-z_]+:[a-z_]+`)

// scanFixture checks that every scope a fixture row names is one the contract declares: fixtures
// are data and cannot import the generated enum, so the check runs the other way.
func scanFixture(path string, c Contract) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	declared := c.scopes()
	var found []finding
	for i, l := range strings.Split(string(raw), "\n") {
		if !strings.Contains(l, "scopes") {
			continue
		}
		var list []string
		if _, rest, ok := strings.Cut(l, "scopes:"); ok {
			rest = strings.Trim(strings.TrimSpace(rest), `'"`)
			if json.Unmarshal([]byte(rest), &list) != nil {
				list = fixtureScope.FindAllString(rest, -1)
			}
		}
		for _, s := range list {
			if !declared[s] {
				found = append(found, finding{i + 1, message(
					fmt.Sprintf("fixture scope %q is not declared by the contract", s))})
			}
		}
	}
	return format(path, found), nil
}
