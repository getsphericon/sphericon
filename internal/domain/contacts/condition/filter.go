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
	"github.com/getsphericon/sphericon/internal/kernel/publicid"
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

// filterNode is an entry of a filter's group: a nested group, a property test or a sent-by test.
type filterNode interface{ isFilterNode() }

// filterLeaf tests the property key of the object the filter is about.
type filterLeaf struct {
	key  string
	test propertyTest
}

// sentByLeaf tests that the message a message's System event is about was sent by the Broadcast
// or the Automation id (ADR 0153, ADR 0180): the `broadcastId` or `automationId` the Event carries.
// publicID is the id as the filter names it, which a refusal quotes.
type sentByLeaf struct {
	sender   conditiontree.Sender
	id       int64
	publicID string
}

func (filterGroup) isFilterNode() {}
func (filterLeaf) isFilterNode()  {}
func (sentByLeaf) isFilterNode()  {}

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

// decodeFilterEntry reads an entries[] item of a filter by its `kind`: a nested group, a property
// test, which names no subject, or a sent-by test, the kinds the filter admits.
func decodeFilterEntry(raw json.RawMessage) (filterNode, error) {
	var e entry
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil, fmt.Errorf("malformed entry: %w", err)
	}
	if !slices.Contains(conditiontree.EventFilterKinds, e.Kind) {
		return nil, fmt.Errorf("unknown entry kind %q", e.Kind)
	}
	switch e.Kind {
	case conditiontree.KindGroup:
		var g conditiontree.Group
		if err := json.Unmarshal(raw, &g); err != nil {
			return nil, fmt.Errorf("malformed group: %w", err)
		}
		return decodeFilter(g)
	case conditiontree.KindSentBy:
		return decodeSentBy(raw)
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

// storedSentBy is a stored sent-by test: whether a Broadcast or an Automation sent the message, and
// its id in the public form the contract carries (ADR 0142).
type storedSentBy struct {
	Sender conditiontree.Sender `json:"sender"`
	ID     string               `json:"id"`
}

// decodeSentBy reads a sent-by test, refusing a sender outside the contract and an id that is no
// public id of this Instance.
func decodeSentBy(raw json.RawMessage) (filterNode, error) {
	var t storedSentBy
	if err := json.Unmarshal(raw, &t); err != nil {
		return nil, fmt.Errorf("malformed sent-by test: %w", err)
	}
	if !slices.Contains(conditiontree.Senders, t.Sender) {
		return nil, fmt.Errorf("unknown sender %q", t.Sender)
	}
	id, err := publicid.Decode(t.ID)
	if err != nil {
		return nil, fmt.Errorf("sent-by id %q is no id", t.ID)
	}
	return sentByLeaf{sender: t.Sender, id: id, publicID: t.ID}, nil
}

// property is the property of a message's System event that holds the key of the sender.
func (l sentByLeaf) property() string {
	switch l.sender {
	case conditiontree.SenderBroadcast:
		return systemevent.BroadcastIDProperty
	case conditiontree.SenderAutomation:
		return systemevent.AutomationIDProperty
	}
	return ""
}

// checkSenders refuses a sent-by test, at any depth, in the filter of an action that is not a
// message's System event: no other Event carries what sent a message (ADR 0180).
func checkSenders(action string, g filterGroup) error {
	if slices.Contains(systemevent.MessageActions, action) {
		return nil
	}
	for i, n := range g.entries {
		switch n := n.(type) {
		case filterGroup:
			if err := checkSenders(action, n); err != nil {
				return fmt.Errorf("entry %d: %w", i, err)
			}
		case sentByLeaf:
			return fmt.Errorf("entry %d: %s is no message's Event, so no %s sent it", i, action, n.sender)
		}
	}
	return nil
}

// Path is a property filter compiled to one SQL/JSON path (ADR 0180), independent of the object it
// runs on: an Event's properties, an Order's line, the properties of the Event a Rule handles.
type Path struct {
	// Text is the path in filter form, strict $ ? (…). It holds the filter's structure and no key
	// or value of it.
	Text string
	// Vars is the path's variables, a JSON object: the keys and values the filter tests.
	Vars json.RawMessage
	// contains are the filter's contains tests, which the statement decides on the object and
	// hands the path as variables of their own (pathBuilder.contains).
	contains []containsTest
}

// containsTest is a contains test of the property key with the operand, whose answer on the object
// is the path's variable name.
type containsTest struct {
	name    string
	key     string
	operand string
}

// CompileEventFilter reads the filter of the Events of action, an Event matcher's or an event
// test's, refusing a tree it cannot read (an entry other than a group, a property test that names
// no subject or a sent-by test, a test a property does not take, a sent-by test of an action that
// is no message's System event), and compiles it to its path. An empty group holds for every
// object, an empty negated group for none.
func CompileEventFilter(action string, f conditiontree.EventFilter) (Path, error) {
	tree, err := decodeFilter(f.Group)
	if err != nil {
		return Path{}, err
	}
	if err := checkSenders(action, tree); err != nil {
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
		b.WriteString(", '{}'::jsonb), ").Arg(p.Text).WriteString("::jsonpath, ")
		p.vars(b, target)
		b.WriteString(", true), false)")
	})
}

// vars writes the path's variables: the bound object of the keys and values, and the answer of each
// contains test on target, a boolean. A contains test holds where the object's property key is a
// text that holds the operand, whatever their case: both bound arguments, compared by strpos, so no
// character of either is read as a pattern. A property that is no text, or is missing, answers
// false, and the path's own guard of its type fails the test there too.
func (p Path) vars(b *sql.Builder, target func(b *sql.Builder)) {
	if len(p.contains) == 0 {
		b.Arg(string(p.Vars)).WriteString("::jsonb")
		return
	}
	b.WriteString("(").Arg(string(p.Vars)).WriteString("::jsonb || jsonb_build_object(")
	for i, c := range p.contains {
		if i > 0 {
			b.WriteString(", ")
		}
		b.Arg(c.name).WriteString("::text, COALESCE(strpos(lower(")
		target(b)
		b.WriteString(" ->> ").Arg(c.key).WriteString("::text), lower(").Arg(c.operand).WriteString("::text)) > 0, false)")
	}
	b.WriteString("))")
}

// Document is the target of a path that is a JSON object given with the query, bound as one of
// its arguments: the properties of the Event a Rule handles, which may have no row yet.
func Document(doc json.RawMessage) func(b *sql.Builder) {
	return func(b *sql.Builder) {
		b.Arg(string(doc)).WriteString("::jsonb")
	}
}

// pathBuilder writes the expressions of a path and binds the keys and values they test as its
// variables, $v0, $v1, …, and the answers of its contains tests as $c0, $c1, …, so that none is
// spliced into it.
type pathBuilder struct {
	vars  map[string]any
	tests []containsTest
}

func newPathBuilder() *pathBuilder { return &pathBuilder{vars: map[string]any{}} }

// bind makes v a variable of the path and returns its name.
func (b *pathBuilder) bind(v any) string {
	name := "v" + strconv.Itoa(len(b.vars))
	b.vars[name] = v
	return "$" + name
}

// contains makes the contains test of the property key with the operand a variable of the path,
// which the statement answers (Path.vars), and returns its name.
func (b *pathBuilder) contains(key, operand string) string {
	name := "c" + strconv.Itoa(len(b.tests))
	b.tests = append(b.tests, containsTest{name: name, key: key, operand: operand})
	return "$" + name
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
	case sentByLeaf:
		return b.property(n.property(), keyTest{id: n.id})
	}
	return "!" + always
}

// keyTest holds for a JSON number that is the key id: the `broadcastId` or `automationId` a
// message's System event carries. It is bound as an integer, so a key past 2^53 compares exactly.
type keyTest struct{ id int64 }

func (keyTest) isTest() {}

func (t keyTest) path(b *pathBuilder, _ string) string {
	return `@.type() == "number" && @ == ` + b.bind(t.id)
}

// property is the predicate that the object @ holds the property key and that its value passes t.
// The key is matched among the object's keys, so it is a variable too; a key the object lacks
// matches none and the test fails, never unknown. Each test guards the value's type before it
// compares, so a value of another type fails it rather than leave it unknown, which a negation
// would keep. An object @ that is no object (JSON null, a list) holds no property: the guard
// before keyvalue, which fails on it, keeps the test false there rather than unknown.
func (b *pathBuilder) property(key string, t propertyTest) string {
	return `@.type() == "object" && exists(@.keyvalue() ? (@.key == ` + b.bind(key) + ").value ? (" + t.path(b, key) + "))"
}

// path is the path of the filter predicate expr, with the variables bound so far. In strict mode
// a list is not unwrapped, so a list that holds a matching value is no match.
func (b *pathBuilder) path(expr string) (Path, error) {
	vars, err := json.Marshal(b.vars)
	if err != nil {
		return Path{}, fmt.Errorf("encode path variables: %w", err)
	}
	return Path{Text: "strict $ ? (" + expr + ")", Vars: vars, contains: b.tests}, nil
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

// NamedSender is a Broadcast or an Automation a sent-by test names (ADR 0153): which sender, its key
// and its id as the test names it, and the path of the test in its tree ("entry 0: filter: entry
// 1: "), which a refusal of an unknown one quotes.
type NamedSender struct {
	Sender   conditiontree.Sender
	ID       int64
	PublicID string
	Path     string
}

// NamedSenders is every Broadcast and Automation the sent-by tests of the Condition's event leaves
// name, at any depth, in the order they appear. A Condition that does not decode names none: its
// save is refused for that first.
func NamedSenders(g conditiontree.Group) []NamedSender {
	tree, err := decode(g, rulePlace)
	if err != nil {
		return nil
	}
	return namedSendersOf(tree, "")
}

// FilterSenders is every Broadcast and Automation the sent-by tests of an Event matcher's filter
// name, as NamedSenders.
func FilterSenders(f conditiontree.EventFilter) []NamedSender {
	tree, err := decodeFilter(f.Group)
	if err != nil {
		return nil
	}
	return filterSendersOf(tree, "")
}

func namedSendersOf(g group, path string) []NamedSender {
	var named []NamedSender
	for i, n := range g.entries {
		at := fmt.Sprintf("%sentry %d: ", path, i)
		switch n := n.(type) {
		case group:
			named = append(named, namedSendersOf(n, at)...)
		case eventCountLeaf:
			if n.filter == nil {
				continue
			}
			tree, err := decodeFilter(n.filter.Group)
			if err == nil {
				named = append(named, filterSendersOf(tree, at+"filter: ")...)
			}
		}
	}
	return named
}

func filterSendersOf(g filterGroup, path string) []NamedSender {
	var named []NamedSender
	for i, n := range g.entries {
		at := fmt.Sprintf("%sentry %d: ", path, i)
		switch n := n.(type) {
		case filterGroup:
			named = append(named, filterSendersOf(n, at)...)
		case sentByLeaf:
			named = append(named, NamedSender{Sender: n.sender, ID: n.id, PublicID: n.publicID, Path: at})
		}
	}
	return named
}

// Holds is the selector predicate that the path holds on the JSON object in column of the table the
// selector reads: the properties of a stored Event.
func (p Path) Holds(column string) func(*sql.Selector) {
	return func(sel *sql.Selector) {
		sel.Where(p.On(func(b *sql.Builder) { b.Ident(sel.C(column)) }))
	}
}
