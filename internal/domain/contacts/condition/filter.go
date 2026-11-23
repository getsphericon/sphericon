package condition

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
)

// An Event matcher's filter (ADR 0133) is a Condition of property tests only: the tests a Rule
// has for its triggering Event's properties, under and, or and not, naming no subject since they
// test the one Event the filter is about. It is two-valued: a property the Event lacks, or holds
// with another type, fails its test, so a not over it matches. It compiles once to one SQL/JSON
// path that Postgres runs on the object where it is (ADR 0180), and a Rule's property tests of its
// triggering Event compile through the same pieces (pathBuilder), so the two cannot disagree.

// filterGroup is a decoded group of a filter: its entries hold, all of them or any, and the result
// is negated when not is set. Its entries are nested groups and property tests, and nothing else:
// a test of the Contact or of its history has no value here.
type filterGroup struct {
	combinator conditiontree.Combinator
	not        bool
	entries    []filterNode
}

// filterNode is an entry of a filter's group: a nested group or a property test.
type filterNode interface{ isFilterNode() }

// filterLeaf tests the property key of the object the filter is about.
type filterLeaf struct {
	key  string
	test propertyTest
}

func (filterGroup) isFilterNode() {}
func (filterLeaf) isFilterNode()  {}

// decodeFilter reads a stored filter into its typed tree, refusing what it cannot read with the
// path of the entry ("entry 0: entry 2: …"), as decode does for the other places.
func decodeFilter(g conditiontree.Group) (filterGroup, error) {
	if err := checkGroup(g); err != nil {
		return filterGroup{}, err
	}
	d := filterGroup{combinator: g.Combinator, not: g.Not, entries: make([]filterNode, 0, len(g.Entries))}
	for i, raw := range g.Entries {
		n, err := decodeFilterEntry(raw)
		if err != nil {
			return filterGroup{}, fmt.Errorf("entry %d: %w", i, err)
		}
		d.entries = append(d.entries, n)
	}
	return d, nil
}

// decodeFilterEntry reads an entries[] item of a filter by its `kind`: a nested group or a
// property test, which names no subject, the two kinds the filter admits.
func decodeFilterEntry(raw json.RawMessage) (filterNode, error) {
	var e entry
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil, fmt.Errorf("malformed entry: %w", err)
	}
	if !slices.Contains(conditiontree.EventFilterKinds, e.Kind) {
		return nil, fmt.Errorf("unknown entry kind %q", e.Kind)
	}
	if e.Kind == conditiontree.KindGroup {
		var g conditiontree.Group
		if err := json.Unmarshal(raw, &g); err != nil {
			return nil, fmt.Errorf("malformed group: %w", err)
		}
		return decodeFilter(g)
	}
	if e.Of != nil {
		return nil, errors.New("a filter's property test names no subject")
	}
	key, t, err := decodePropertyTest(e)
	if err != nil {
		return nil, err
	}
	return filterLeaf{key: key, test: t}, nil
}

// Path is a property filter compiled to one SQL/JSON path (ADR 0180), independent of the object it
// runs on: an Event's properties, an Order's line, the properties of the Event a Rule handles.
type Path struct {
	// Text is the path in filter form, strict $ ? (…). It holds the filter's structure and no key
	// or value of it, but the operand of a contains test: like_regex takes only a literal, which
	// is written as a JSON string and matched literally (flag q).
	Text string
	// Vars is the path's variables, a JSON object: the keys and values the filter tests.
	Vars json.RawMessage
}

// CompileEventFilter reads an Event matcher's filter, refusing a tree it cannot read (an entry
// other than a group or a property test that names no subject, a test a property does not take),
// and compiles it to its path. An empty group holds for every object, an empty negated group for
// none.
func CompileEventFilter(f conditiontree.EventFilter) (Path, error) {
	tree, err := decodeFilter(f.Group)
	if err != nil {
		return Path{}, err
	}
	b := newPathBuilder()
	return b.path(b.group(tree))
}

// On is the SQL predicate that the path holds on target, which writes the JSON object the filter
// is about (a column, or Document). A missing object (SQL NULL) is read as the empty one, and one
// that is no object holds no property: every test fails there, so a negation over it holds.
func (p Path) On(target func(b *sql.Builder)) *sql.Predicate {
	// raw sql: ent has no SQL/JSON path predicate. The path and its variables are bound arguments,
	// run in silent mode; a NULL target is the empty object, and the outer COALESCE keeps the
	// predicate decided whatever the path yields.
	return sql.P(func(b *sql.Builder) {
		b.WriteString("COALESCE(jsonb_path_exists(COALESCE(")
		target(b)
		b.WriteString(", '{}'::jsonb), ").Arg(p.Text).WriteString("::jsonpath, ").Arg(string(p.Vars)).
			WriteString("::jsonb, true), false)")
	})
}

// Document is the target of a path that is a JSON object given with the query, bound as one of
// its arguments: the properties of the Event a Rule handles, which may have no row yet.
func Document(doc json.RawMessage) func(b *sql.Builder) {
	return func(b *sql.Builder) {
		b.Arg(string(doc)).WriteString("::jsonb")
	}
}

// pathBuilder writes the expressions of a path and binds the keys and values they test as its
// variables, $v0, $v1, …, so that none is spliced into it.
type pathBuilder struct {
	vars map[string]any
	err  error
}

func newPathBuilder() *pathBuilder { return &pathBuilder{vars: map[string]any{}} }

// bind makes v a variable of the path and returns its name.
func (b *pathBuilder) bind(v any) string {
	name := "v" + strconv.Itoa(len(b.vars))
	b.vars[name] = v
	return "$" + name
}

// literal writes s as a string literal of the path, in JSON's escapes, which the path's own
// string literals read alike. Only an operator that takes no variable writes one.
func (b *pathBuilder) literal(s string) string {
	quoted, err := json.Marshal(s)
	if err != nil {
		b.err = errors.Join(b.err, fmt.Errorf("encode operand: %w", err))
	}
	return string(quoted)
}

// always is a predicate every object holds, which an empty group is: the path has no true literal.
const always = "exists(@)"

// group is the predicate of a filter's group, on the object @ the filter is about.
func (b *pathBuilder) group(g filterGroup) string {
	expr := always
	if len(g.entries) > 0 {
		parts := make([]string, 0, len(g.entries))
		for _, n := range g.entries {
			parts = append(parts, "("+b.node(n)+")")
		}
		join := " && "
		if g.combinator == conditiontree.Or {
			join = " || "
		}
		expr = strings.Join(parts, join)
	}
	if g.not {
		return "!(" + expr + ")"
	}
	return expr
}

// node is the predicate of an entry of a filter's group.
func (b *pathBuilder) node(n filterNode) string {
	switch n := n.(type) {
	case filterGroup:
		return b.group(n)
	case filterLeaf:
		return b.property(n.key, n.test)
	}
	return "!" + always
}

// property is the predicate that the object @ holds the property key and that its value passes t.
// The key is matched among the object's keys, so it is a variable too; a key the object lacks
// matches none and the test fails, never unknown. Each test guards the value's type before it
// compares, so a value of another type fails it rather than leave it unknown, which a negation
// would keep. An object @ that is no object (JSON null, a list) holds no property: the guard
// before keyvalue, which fails on it, keeps the test false there rather than unknown.
func (b *pathBuilder) property(key string, t propertyTest) string {
	return `@.type() == "object" && exists(@.keyvalue() ? (@.key == ` + b.bind(key) + ").value ? (" + t.path(b) + "))"
}

// path is the path of the filter predicate expr, with the variables bound so far. In strict mode
// a list is not unwrapped, so a list that holds a matching value is no match.
func (b *pathBuilder) path(expr string) (Path, error) {
	if b.err != nil {
		return Path{}, b.err
	}
	vars, err := json.Marshal(b.vars)
	if err != nil {
		return Path{}, fmt.Errorf("encode path variables: %w", err)
	}
	return Path{Text: "strict $ ? (" + expr + ")", Vars: vars}, nil
}

// Definitions is what an Event matcher's filter names in the Workspace's catalogue (ADR 0133): the
// Tags and the Custom fields whose names its tests compare the Event's own property with.
type Definitions struct {
	Tags         []string
	CustomFields []string
}

// NamedDefinitions is what the filter of a matcher on action names (Definitions): the value of each
// eq and neq test, at any depth, of the property of action that holds a definition's name, the
// `tag` of contact.tagged and contact.untagged and the `key` of contact.field_changed. A contains or
// beginsWith test names a pattern, not a definition. A filter that does not read names nothing.
func NamedDefinitions(action string, f conditiontree.EventFilter) Definitions {
	var d Definitions
	switch action {
	case systemevent.ContactTagged, systemevent.ContactUntagged:
		d.Tags = namesOf(f, "tag")
	case systemevent.ContactFieldChanged:
		d.CustomFields = namesOf(f, "key")
	}
	return d
}

// namesOf is the values the filter's eq and neq tests of the property key compare it with, each
// once, in the order they first appear.
func namesOf(f conditiontree.EventFilter, key string) []string {
	tree, err := decodeFilter(f.Group)
	if err != nil {
		return nil
	}
	var names []string
	var walk func(g filterGroup)
	walk = func(g filterGroup) {
		for _, n := range g.entries {
			switch n := n.(type) {
			case filterGroup:
				walk(n)
			case filterLeaf:
				t, ok := n.test.(textTest)
				if ok && n.key == key && (t.op == conditiontree.TextEq || t.op == conditiontree.TextNeq) && !slices.Contains(names, t.value) {
					names = append(names, t.value)
				}
			}
		}
	}
	walk(tree)
	return names
}

// Holds is the selector predicate that the path holds on the JSON object in column of the table the
// selector reads: the properties of a stored Event.
func (p Path) Holds(column string) func(*sql.Selector) {
	return func(sel *sql.Selector) {
		sel.Where(p.On(func(b *sql.Builder) { b.Ident(sel.C(column)) }))
	}
}
