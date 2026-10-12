// Package segments is a domain-agnostic engine that compiles a react-querybuilder
// definition (the JSON shape the UI emits) into an ent SQL predicate. It depends
// only on entgo.io/ent's general-purpose SQL builder — not on any generated
// entity — so it can be lifted into a standalone library. The application-side
// glue that binds it to our Contact entity lives in contact.go (the only file
// here that imports the project's ent package); drop that file when extracting.
//
// Definition format (recursive): a group {combinator: and|or, not, rules[]}
// where each rules[] entry is either a nested group or a leaf {field, operator,
// value}. A caller supplies a Schema that whitelists which fields may be used
// and maps them to SQL columns (plain or JSON), which keeps a stored definition
// from referencing arbitrary columns.
package segments

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
)

// eventFieldPrefix marks a leaf as a behavioral condition: field "event:<action>"
// with operator "performed"/"notPerformed" and value = a day window ("" = ever).
const eventFieldPrefix = "event:"

// tagField marks a leaf as a Tag condition: field "tag" with operator "has" or
// "doesNotHave" and value = the Tag name.
const tagField = "tag"

// Group is a react-querybuilder RuleGroupType. Each rules[] entry is decoded
// lazily because it is either a nested Group or a leaf Rule.
type Group struct {
	Combinator string            `json:"combinator"`
	Not        bool              `json:"not,omitempty"`
	Rules      []json.RawMessage `json:"rules"`
}

// Rule is a react-querybuilder leaf rule.
type Rule struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}

// Schema whitelists the fields a definition may reference and maps them to SQL.
// Columns maps a field name to a plain column. JSONColumns maps a field-name
// prefix (e.g. "custom:") to a JSON column; the path is the field after the
// prefix (e.g. "custom:plan" -> column custom_fields, path "plan").
type Schema struct {
	Columns     map[string]string
	JSONColumns map[string]string
	// Events, when set, enables "event:<action>" leaves that compile to a
	// correlated (NOT) EXISTS against an events table. All names are the SQL
	// columns (kept as strings so this engine stays ent-agnostic).
	Events *EventSchema
	// Tags, when set, enables the "tag" leaf (has / doesNotHave a named Tag),
	// compiled to a correlated (NOT) EXISTS over the subject<->tag join table.
	Tags *TagSchema
}

// TagSchema describes how a subject entity relates to its Tags: the join table
// (subject id, tag id), the tags table (id, name, workspace), and the outer
// subject's id/workspace columns. Names are SQL columns, keeping the engine
// ent-agnostic.
type TagSchema struct {
	JoinTable         string
	JoinSubjectCol    string
	JoinTagCol        string
	TagTable          string
	TagIDCol          string
	TagNameCol        string
	TagWorkspaceCol   string
	OuterJoinCol      string
	OuterWorkspaceCol string
}

// EventSchema describes how to correlate the subject entity to its events: the
// events table + its (join, workspace, action, occurred_at) columns, plus the outer
// subject's join/workspace columns. The join is on the stable identity link
// (event.contact_id ↔ contact.id), never the email string (ADR 0002).
type EventSchema struct {
	Table             string
	JoinCol           string
	WorkspaceCol      string
	ActionCol         string
	OccurredCol       string
	OuterJoinCol      string
	OuterWorkspaceCol string
}

// Predicate is the compiled output: a selector mutator, assignment-compatible
// with ent's generated predicate.<Entity> type (also func(*sql.Selector)).
type Predicate = func(*sql.Selector)

// builder defers predicate construction until a selector is available (so plain
// columns can be table-qualified via s.C).
type builder func(*sql.Selector) *sql.Predicate

// Parse decodes a definition string into a Group. A blank definition is a valid
// "match everyone" group.
func Parse(def string) (Group, error) {
	g := Group{Combinator: "and"}
	if strings.TrimSpace(def) == "" {
		return g, nil
	}
	if err := json.Unmarshal([]byte(def), &g); err != nil {
		return g, fmt.Errorf("invalid segment definition: %w", err)
	}
	return g, nil
}

// Validate parses and fully compiles a definition against schema.
func Validate(def string, schema Schema) error {
	g, err := Parse(def)
	if err != nil {
		return err
	}
	_, err = Compile(g, schema)
	return err
}

// Compile turns a definition group into a selector predicate. An empty rule set
// matches everyone (an empty negated group matches no one) — deliberate, so an
// empty rule is never a silent "nobody".
func Compile(g Group, schema Schema) (Predicate, error) {
	b, err := compileGroup(g, schema)
	if err != nil {
		return nil, err
	}
	return func(s *sql.Selector) { s.Where(b(s)) }, nil
}

func compileGroup(g Group, schema Schema) (builder, error) {
	builders := make([]builder, 0, len(g.Rules))
	for i, raw := range g.Rules {
		b, err := compileEntry(raw, schema)
		if err != nil {
			return nil, fmt.Errorf("rule %d: %w", i, err)
		}
		builders = append(builders, b)
	}

	if len(builders) == 0 {
		expr := "TRUE"
		if g.Not {
			expr = "FALSE"
		}
		return func(*sql.Selector) *sql.Predicate { return sql.ExprP(expr) }, nil
	}

	or := strings.EqualFold(g.Combinator, "or")
	return func(s *sql.Selector) *sql.Predicate {
		preds := make([]*sql.Predicate, len(builders))
		for i, b := range builders {
			preds[i] = b(s)
		}
		var p *sql.Predicate
		if or {
			p = sql.Or(preds...)
		} else {
			p = sql.And(preds...)
		}
		if g.Not {
			p = sql.Not(p)
		}
		return p
	}, nil
}

// compileEntry decodes a rules[] entry: a nested group when it carries a
// combinator (or has no field), a leaf rule otherwise.
func compileEntry(raw json.RawMessage, schema Schema) (builder, error) {
	var probe struct {
		Combinator string `json:"combinator"`
		Field      string `json:"field"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, fmt.Errorf("malformed rule: %w", err)
	}
	if probe.Combinator != "" || probe.Field == "" {
		var g Group
		if err := json.Unmarshal(raw, &g); err != nil {
			return nil, fmt.Errorf("malformed group: %w", err)
		}
		return compileGroup(g, schema)
	}
	var r Rule
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("malformed rule: %w", err)
	}
	return compileRule(r, schema)
}

func compileRule(r Rule, schema Schema) (builder, error) {
	if strings.HasPrefix(r.Field, eventFieldPrefix) {
		action := strings.TrimPrefix(r.Field, eventFieldPrefix)
		if schema.Events == nil || action == "" {
			return nil, fmt.Errorf("unknown field %q", r.Field)
		}
		return compileEvent(schema.Events, action, r.Operator, r.Value)
	}
	if r.Field == tagField && schema.Tags != nil {
		return compileTag(schema.Tags, r.Operator, r.Value)
	}
	if col, path, ok := resolveJSON(r.Field, schema); ok {
		return compileJSON(col, path, r.Operator, r.Value)
	}
	col, ok := schema.Columns[r.Field]
	if !ok {
		return nil, fmt.Errorf("unknown field %q", r.Field)
	}
	switch r.Operator {
	case "=":
		return func(s *sql.Selector) *sql.Predicate { return sql.EQ(s.C(col), r.Value) }, nil
	case "!=":
		return func(s *sql.Selector) *sql.Predicate { return sql.NEQ(s.C(col), r.Value) }, nil
	case "contains":
		return func(s *sql.Selector) *sql.Predicate { return sql.ContainsFold(s.C(col), r.Value) }, nil
	case "beginsWith":
		return func(s *sql.Selector) *sql.Predicate { return sql.HasPrefix(s.C(col), r.Value) }, nil
	case "null":
		return func(s *sql.Selector) *sql.Predicate { return sql.IsNull(s.C(col)) }, nil
	case "notNull":
		return func(s *sql.Selector) *sql.Predicate { return sql.NotNull(s.C(col)) }, nil
	default:
		return nil, fmt.Errorf("unsupported operator %q", r.Operator)
	}
}

// compileEvent builds a correlated (NOT) EXISTS against the events table: the
// subject performed `action` (optionally within the last `value` days). The
// subquery joins events↔subject on the stable identity link (contact_id ↔ id) +
// workspace, so anonymous events stitched onto a Contact at Identify become visible
// to segmentation (ADR 0002), and it works for any subject schema supplying those
// outer columns.
func compileEvent(ev *EventSchema, action, op, value string) (builder, error) {
	var negate bool
	switch op {
	case "performed":
		negate = false
	case "notPerformed":
		negate = true
	default:
		return nil, fmt.Errorf("unsupported operator %q for event field", op)
	}

	var since *time.Time
	if v := strings.TrimSpace(value); v != "" {
		days, err := strconv.Atoi(v)
		if err != nil || days < 0 {
			return nil, fmt.Errorf("event window must be a non-negative number of days, got %q", value)
		}
		if days > 0 {
			t := time.Now().AddDate(0, 0, -days)
			since = &t
		}
	}

	return func(s *sql.Selector) *sql.Predicate {
		sub := sql.Select(ev.JoinCol).From(sql.Table(ev.Table))
		preds := []*sql.Predicate{
			sql.ColumnsEQ(sub.C(ev.JoinCol), s.C(ev.OuterJoinCol)),
			sql.ColumnsEQ(sub.C(ev.WorkspaceCol), s.C(ev.OuterWorkspaceCol)),
			sql.EQ(sub.C(ev.ActionCol), action),
		}
		if since != nil {
			// GTE excludes NULL occurred_at. Safe in practice: the events bus always
			// stamps occurred_at (zero ⇒ publish time), so persisted events are never
			// null. "ever" (since == nil) adds no time predicate and matches all.
			preds = append(preds, sql.GTE(sub.C(ev.OccurredCol), *since))
		}
		sub.Where(sql.And(preds...))
		if negate {
			return sql.NotExists(sub)
		}
		return sql.Exists(sub)
	}, nil
}

// compileTag builds a correlated (NOT) EXISTS: the subject has (or lacks) the Tag
// named value, within the subject's own workspace. Evaluated live — membership is
// never materialized.
func compileTag(tg *TagSchema, op, value string) (builder, error) {
	var negate bool
	switch op {
	case "has":
		negate = false
	case "doesNotHave":
		negate = true
	default:
		return nil, fmt.Errorf("unsupported operator %q for tag field", op)
	}
	name := strings.TrimSpace(value)
	if name == "" {
		return nil, errors.New("tag name is required")
	}

	return func(s *sql.Selector) *sql.Predicate {
		join := sql.Table(tg.JoinTable)
		tags := sql.Table(tg.TagTable)
		sub := sql.Select(join.C(tg.JoinSubjectCol)).From(join).
			Join(tags).On(join.C(tg.JoinTagCol), tags.C(tg.TagIDCol))
		sub.Where(sql.And(
			sql.ColumnsEQ(join.C(tg.JoinSubjectCol), s.C(tg.OuterJoinCol)),
			sql.ColumnsEQ(tags.C(tg.TagWorkspaceCol), s.C(tg.OuterWorkspaceCol)),
			sql.EQ(tags.C(tg.TagNameCol), name),
		))
		if negate {
			return sql.NotExists(sub)
		}
		return sql.Exists(sub)
	}, nil
}

// compileJSON builds a predicate over one key of a JSON column. The key comes from
// the stored rule, so it is always a bound argument and never part of the query text.
func compileJSON(col, key, op, value string) (builder, error) {
	var write func(b *sql.Builder, column string)
	switch op {
	case "=", "!=":
		cmp := " = "
		if op == "!=" {
			cmp = " <> "
		}
		write = func(b *sql.Builder, column string) {
			b.WriteString("(").WriteString(column).WriteString(" ->> ").Arg(key).WriteString(")").
				WriteString(cmp).Arg(value)
		}
	case "contains":
		// Membership of a JSON array (jsonb containment of the encoded value).
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("encode json value: %w", err)
		}
		write = func(b *sql.Builder, column string) {
			b.WriteString("(").WriteString(column).WriteString(" -> ").Arg(key).WriteString(") @> ").
				Arg(string(encoded)).WriteString("::jsonb")
		}
	case "notNull":
		write = func(b *sql.Builder, column string) {
			b.WriteString("(").WriteString(column).WriteString(" -> ").Arg(key).WriteString(") IS NOT NULL")
		}
	default:
		return nil, fmt.Errorf("unsupported operator %q for json field", op)
	}
	return func(s *sql.Selector) *sql.Predicate {
		return sql.P(func(b *sql.Builder) { write(b, s.C(col)) })
	}, nil
}

// resolveJSON reports whether field maps to a JSON column via a schema prefix,
// returning the column and the JSON path (the field minus the prefix).
func resolveJSON(field string, schema Schema) (col, path string, ok bool) {
	for prefix, column := range schema.JSONColumns {
		if strings.HasPrefix(field, prefix) {
			p := strings.TrimPrefix(field, prefix)
			if p == "" {
				return "", "", false
			}
			return column, p, true
		}
	}
	return "", "", false
}
