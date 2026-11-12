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

func scanGo(path string, c Contract) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, err
	}
	var found []finding
	flagSet := func(pos token.Pos, values []string, what string) {
		if c.copiesEnum(values) {
			found = append(found, finding{fset.Position(pos).Line, message(what)})
		}
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CompositeLit:
			flagSet(n.Pos(), literalKeys(n.Elts), "a list of literals repeats a TypeSpec enum")
		case *ast.CaseClause:
			flagSet(n.Pos(), stringValues(n.List), "a case list repeats a TypeSpec enum")
		}
		return true
	})
	return format(path, found), nil
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

func stringLit(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	return s, err == nil
}

var (
	tsString = regexp.MustCompile(`'([^'\\\n]*)'|"([^"\\\n]*)"`)
	// tsArray is an array literal of strings, with spreads among them (`[...ACTIONS, 'email.sent']`):
	// its strings are read, the spread lists by their own literal.
	tsArray = regexp.MustCompile(`\[\s*(?:(?:'[^'\\\n]*'|"[^"\\\n]*"|\.\.\.[A-Za-z_$][\w$.]*)\s*,?\s*)+\]`)
	tsUnion = regexp.MustCompile(`(?:'[^'\\\n]*'|"[^"\\\n]*")(?:\s*\|\s*(?:'[^'\\\n]*'|"[^"\\\n]*"))+`)
	tsLimit = regexp.MustCompile(`\b((?:MAX|MIN|DEFAULT)_[A-Z_]+|max|min|maxLength|minLength)\s*(?:=|=\{|:)\s*(\d[\d_]*)\b`)
	// tsOptions is an array literal with no array inside it, and tsOptionValue the `value` of each
	// of its entries: a Select's options, one per value of an enum.
	tsOptions     = regexp.MustCompile(`\[[^\[\]]*\]`)
	tsOptionValue = regexp.MustCompile(`\bvalue:\s*(?:'([^'\\\n]*)'|"([^"\\\n]*)")`)
	// tsShape opens a declaration of fields: an interface that extends nothing, an object type,
	// or a zod object.
	tsShape = regexp.MustCompile(`\binterface\s+\w+\s*\{|\btype\s+\w+\s*=\s*\{|\bz\.object\(\s*\{`)
	// tsQueryLimit is a page size a request's query names: a response's own pageSize, in a test's
	// mock, is the server's value and not read.
	tsQueryLimit = regexp.MustCompile(`\bquery:\s*\{[^{}]*?\b(pageSize)\s*:\s*(\d[\d_]*)\b`)
	// tsDerived is a generated zod schema a field is read from (`trimmed(zButton.shape.label)`).
	tsDerived = regexp.MustCompile(`\bz[A-Z]\w*`)
	tsField   = regexp.MustCompile(`^\s*(?:readonly\s+)?([A-Za-z_$][\w$]*)\??\s*:`)
	// tsConst is a top-level constant of one string: a file's such constants, read together, may
	// name the values of an enum one by one (`const EMAIL_NODE_TYPE = 'email'`, ...).
	tsConst = regexp.MustCompile(`(?m)^(?:export\s+)?const\s+[A-Za-z_$][\w$]*\s*(?::\s*[^=\n]+)?=\s*(?:'([^'\\\n]*)'|"([^"\\\n]*)")\s*$`)
)

func scanTS(path string, c Contract) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	src := string(raw)
	lineOf := func(off int) int { return strings.Count(src[:off], "\n") + 1 }
	var found []finding
	for _, re := range []*regexp.Regexp{tsArray, tsUnion} {
		for _, m := range re.FindAllStringIndex(src, -1) {
			var values []string
			for _, s := range tsString.FindAllStringSubmatch(src[m[0]:m[1]], -1) {
				values = append(values, s[1]+s[2])
			}
			line := lineOf(m[0])
			if c.copiesEnum(values) {
				found = append(found, finding{line, message("a literal list or union repeats a TypeSpec enum")})
			}
		}
	}
	var constants []string
	firstConst := 0
	for _, m := range tsConst.FindAllStringSubmatchIndex(src, -1) {
		if firstConst == 0 {
			firstConst = m[0] + 1
		}
		constants = append(constants, src[max(m[2], m[4]):max(m[3], m[5])])
	}
	if firstConst > 0 && c.copiesEnum(constants) {
		found = append(found, finding{lineOf(firstConst - 1), message("string constants repeat a TypeSpec enum")})
	}
	for _, m := range tsOptions.FindAllStringIndex(src, -1) {
		var values []string
		for _, s := range tsOptionValue.FindAllStringSubmatch(src[m[0]:m[1]], -1) {
			values = append(values, s[1]+s[2])
		}
		line := lineOf(m[0])
		if len(values) >= 2 && c.copiesEnum(values) {
			found = append(found, finding{line, message("the values of a list of options repeat a TypeSpec enum")})
		}
	}
	for _, m := range tsShape.FindAllStringIndex(src, -1) {
		fields, derived := topLevelFields(src[m[1]:])
		line := lineOf(m[0])
		if derived {
			continue
		}
		if name := c.Shapes[shapeKey(fields)]; len(fields) >= 2 && name != "" {
			found = append(found, finding{line, message(fmt.Sprintf(
				"a type or schema declares the fields of %s, which the generated client exports", name))})
		}
	}
	for _, re := range []*regexp.Regexp{tsLimit, tsQueryLimit} {
		found = append(found, limitMatches(src, re, c, lineOf)...)
	}
	return format(path, found), nil
}

// limitMatches are the matches of re (a name, then an integer) whose value is a contract limit.
func limitMatches(src string, re *regexp.Regexp, c Contract, lineOf func(int) int) []finding {
	var found []finding
	for _, m := range re.FindAllStringSubmatchIndex(src, -1) {
		v, err := strconv.ParseInt(strings.ReplaceAll(src[m[4]:m[5]], "_", ""), 10, 64)
		line := lineOf(m[0])
		if err == nil && c.Limits[v] {
			found = append(found, finding{line, message(
				fmt.Sprintf("%s = %d repeats a limit TypeSpec declares", src[m[2]:m[3]], v))})
		}
	}
	return found
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

// topLevelFields are the names of the fields a declaration of fields lists, read from the text
// after its opening brace up to the matching one; nested objects, calls and arrays are skipped (a
// generic's comma splits a member, which then names no second field). derived is whether every
// field is read from a generated zod schema: a form's schema made of the contract's own parts.
func topLevelFields(body string) (fields []string, derived bool) {
	depth, start := 0, 0
	derived = true
	member := func(end int) {
		if m := tsField.FindStringSubmatch(body[start:end]); m != nil {
			fields = append(fields, m[1])
			derived = derived && tsDerived.MatchString(body[start:end])
		}
	}
	for i, r := range body {
		switch r {
		case '{', '(', '[':
			depth++
		case ')', ']':
			depth--
		case '}':
			if depth == 0 {
				member(i)
				return fields, derived && len(fields) > 0
			}
			depth--
		case ',', ';', '\n':
			if depth == 0 {
				member(i)
				start = i + 1
			}
		}
	}
	return fields, false
}
