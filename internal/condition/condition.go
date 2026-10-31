// Package condition is the Condition: the typed tree of tests on a Contact (the API
// contract's tree, ADR 0029, ADR 0054), its validation and its compiler to a Contact
// predicate. A Segment is a named Condition and a Rule holds one. A group {kind: "group", combinator: and|or, not,
// rules[]} whose entries are nested groups, leaves {kind: "rule", field, operator, value}, event
// counts {kind: "event", action, operator, times, withinDays}, number comparisons {kind: "number",
// field, operator, value} and date tests {kind: "date", field, operator, at} and {kind:
// "dateWithin", field, withinDays}, told apart by `kind`.
//
// A leaf is built from ent's generated predicates wherever one exists: the core
// attributes (contact.EmailEQ …) and Tags (contact.HasTagsWith). An event count is a correlated
// COUNT(*) of the Contact's Events, built with ent's SQL builder and the generated Event
// predicates. The one dynamic piece is a Custom field's key inside the
// custom_fields JSON column, which no generated predicate covers; it is addressed
// with ent's JSON predicates (sqljson) after the key is checked, and by a number or date leaf with a
// jsonpath filter, which is false on a value of another type where a cast would fail the query. A
// leaf may only name a core attribute, a Tag, an Event action, or a Custom field of the Workspace's
// catalogue (ADR 0006), and a number or date leaf only a Custom field of its type.
package condition

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/tag"
	"github.com/getsphericon/sphericon/internal/events"
)

const (
	// customFieldPrefix marks a leaf as a Custom field condition: "custom:<key>".
	customFieldPrefix = "custom:"
	// tagField marks a leaf as a Tag condition: field "tag" with operator "has" or
	// "doesNotHave" and value = the Tag name.
	tagField = "tag"
	// triggerFieldPrefix marks a leaf as a test on the triggering Event's own properties:
	// "trigger:<key>". Valid only in a Rule (RulePredicate); a Segment refuses it.
	triggerFieldPrefix = "trigger:"
)

const (
	kindGroup      = "group"
	kindRule       = "rule"
	kindEvent      = "event"
	kindNumber     = "number"
	kindDate       = "date"
	kindDateWithin = "dateWithin"
)

// Group is a group of the rule tree. Each rules[] entry is decoded lazily because it
// is either a nested Group or a Leaf, as its `kind` says.
type Group struct {
	Kind       string            `json:"kind"`
	Combinator string            `json:"combinator"`
	Not        bool              `json:"not,omitempty"`
	Rules      []json.RawMessage `json:"rules"`
}

// Leaf is one test of the tree: a field, an operator and a value.
type Leaf struct {
	Kind     string `json:"kind"`
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}

// EventLeaf is a count of the Contact's Events of one action: at least or at most Times of
// them, over all time or within the last WithinDays days. Times is a pointer so that a leaf
// without it is refused instead of read as zero.
type EventLeaf struct {
	Kind       string `json:"kind"`
	Action     string `json:"action"`
	Operator   string `json:"operator"`
	Times      *int   `json:"times"`
	WithinDays *int   `json:"withinDays,omitempty"`
}

// NumberLeaf compares a number: a Custom field of type number ("custom:<key>") or, in a Rule, a
// property of the triggering Event ("trigger:<key>"). Value is a pointer so that a leaf without it
// is refused instead of read as zero.
type NumberLeaf struct {
	Kind     string   `json:"kind"`
	Field    string   `json:"field"`
	Operator string   `json:"operator"`
	Value    *float64 `json:"value"`
}

// DateLeaf tests a Custom field of type datetime ("custom:<key>"): its moment is before At, or at
// or after it.
type DateLeaf struct {
	Kind     string    `json:"kind"`
	Field    string    `json:"field"`
	Operator string    `json:"operator"`
	At       time.Time `json:"at"`
}

// DateWithinLeaf tests a Custom field of type datetime ("custom:<key>"): its moment is within the
// last WithinDays days up to now.
type DateWithinLeaf struct {
	Kind       string `json:"kind"`
	Field      string `json:"field"`
	WithinDays int    `json:"withinDays"`
}

// textField is the generated predicates of one core text attribute.
type textField struct {
	eq, neq, contains, beginsWith func(string) predicate.Contact
	null, notNull                 predicate.Contact
}

// textFields whitelists the core attributes a rule may target.
var textFields = map[string]textField{
	"subject_id": {contact.SubjectIDEQ, contact.SubjectIDNEQ, contact.SubjectIDContainsFold, contact.SubjectIDHasPrefix, contact.SubjectIDIsNil(), contact.SubjectIDNotNil()},
	"email":      {contact.EmailEQ, contact.EmailNEQ, contact.EmailContainsFold, contact.EmailHasPrefix, contact.EmailIsNil(), contact.EmailNotNil()},
	"phone":      {contact.PhoneEQ, contact.PhoneNEQ, contact.PhoneContainsFold, contact.PhoneHasPrefix, contact.PhoneIsNil(), contact.PhoneNotNil()},
	"first_name": {contact.FirstNameEQ, contact.FirstNameNEQ, contact.FirstNameContainsFold, contact.FirstNameHasPrefix, contact.FirstNameIsNil(), contact.FirstNameNotNil()},
	"last_name":  {contact.LastNameEQ, contact.LastNameNEQ, contact.LastNameContainsFold, contact.LastNameHasPrefix, contact.LastNameIsNil(), contact.LastNameNotNil()},
	"time_zone":  {contact.TimeZoneEQ, contact.TimeZoneNEQ, contact.TimeZoneContainsFold, contact.TimeZoneHasPrefix, contact.TimeZoneIsNil(), contact.TimeZoneNotNil()},
}

// Parse decodes a stored condition into a Group. A blank condition is a valid
// "match everyone" group.
func Parse(def string) (Group, error) {
	if strings.TrimSpace(def) == "" {
		return Group{Kind: kindGroup, Combinator: "and"}, nil
	}
	var g Group
	if err := json.Unmarshal([]byte(def), &g); err != nil {
		return g, fmt.Errorf("invalid condition: %w", err)
	}
	return g, nil
}

// ContactPredicate parses a stored condition and compiles it for the Workspace of s.
func ContactPredicate(ctx context.Context, s *ent.Scoped, def string) (predicate.Contact, error) {
	g, err := Parse(def)
	if err != nil {
		return nil, err
	}
	return Compile(ctx, s, g)
}

// Compile turns a condition group into a Contact predicate. An empty rule set
// matches everyone (an empty negated group matches no one) — deliberate, so an
// empty rule is never a silent "nobody". A "trigger:" leaf is refused: only a Rule has an Event.
func Compile(ctx context.Context, s *ent.Scoped, g Group) (predicate.Contact, error) {
	return (&compiler{s: s, now: time.Now()}).group(ctx, g)
}

// RulePredicate parses a Rule's stored condition and compiles it for the Workspace of s, with
// "trigger:<key>" leaves allowed. They are evaluated in memory against props, the properties of
// the Event that fired the Rule, and folded into the tree as constants before the rest compiles
// to a predicate (the Event's row may not exist yet). A key the Event lacks, or a value of
// another type, makes the leaf false. Any key is valid, so props may be nil to only validate.
func RulePredicate(ctx context.Context, s *ent.Scoped, def string, props map[string]any) (predicate.Contact, error) {
	g, err := Parse(def)
	if err != nil {
		return nil, err
	}
	return (&compiler{s: s, now: time.Now(), trigger: &props}).group(ctx, g)
}

// compiler holds what one compilation needs: the Workspace's Custom field keys are
// loaded the first time a rule names one.
type compiler struct {
	s    *ent.Scoped
	now  time.Time
	keys map[string]customfield.Type
	// trigger is the properties of the Event a Rule's "trigger:" leaves read; nil in a Segment,
	// where such a leaf is refused.
	trigger *map[string]any
}

func (c *compiler) group(ctx context.Context, g Group) (predicate.Contact, error) {
	if g.Kind != kindGroup {
		return nil, fmt.Errorf("unknown group kind %q", g.Kind)
	}
	if g.Combinator != "and" && g.Combinator != "or" {
		return nil, fmt.Errorf("unknown combinator %q", g.Combinator)
	}
	preds := make([]predicate.Contact, 0, len(g.Rules))
	for i, raw := range g.Rules {
		p, err := c.entry(ctx, raw)
		if err != nil {
			return nil, fmt.Errorf("rule %d: %w", i, err)
		}
		preds = append(preds, p)
	}

	if len(preds) == 0 {
		if g.Not {
			return predicate.Contact(func(s *sql.Selector) { s.Where(sql.False()) }), nil
		}
		return contact.And(), nil
	}

	p := contact.And(preds...)
	if g.Combinator == "or" {
		p = contact.Or(preds...)
	}
	if g.Not {
		p = contact.Not(p)
	}
	return p, nil
}

// entry decodes a rules[] entry by its `kind`: a nested group or a leaf rule.
func (c *compiler) entry(ctx context.Context, raw json.RawMessage) (predicate.Contact, error) {
	var probe struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, fmt.Errorf("malformed rule: %w", err)
	}
	switch probe.Kind {
	case kindGroup:
		var g Group
		if err := json.Unmarshal(raw, &g); err != nil {
			return nil, fmt.Errorf("malformed group: %w", err)
		}
		return c.group(ctx, g)
	case kindRule:
		var r Leaf
		if err := json.Unmarshal(raw, &r); err != nil {
			return nil, fmt.Errorf("malformed rule: %w", err)
		}
		return c.rule(ctx, r)
	case kindEvent:
		var l EventLeaf
		if err := json.Unmarshal(raw, &l); err != nil {
			return nil, fmt.Errorf("malformed event leaf: %w", err)
		}
		return c.event(l)
	case kindNumber:
		var l NumberLeaf
		if err := json.Unmarshal(raw, &l); err != nil {
			return nil, fmt.Errorf("malformed number leaf: %w", err)
		}
		return c.number(ctx, l)
	case kindDate:
		var l DateLeaf
		if err := json.Unmarshal(raw, &l); err != nil {
			return nil, fmt.Errorf("malformed date leaf: %w", err)
		}
		return c.date(ctx, l)
	case kindDateWithin:
		var l DateWithinLeaf
		if err := json.Unmarshal(raw, &l); err != nil {
			return nil, fmt.Errorf("malformed date leaf: %w", err)
		}
		return c.dateWithin(ctx, l)
	default:
		return nil, fmt.Errorf("unknown rule kind %q", probe.Kind)
	}
}

func (c *compiler) rule(ctx context.Context, r Leaf) (predicate.Contact, error) {
	op := operator(r.Operator)
	switch {
	case r.Field == tagField:
		return tagRule(op, r.Value)
	case strings.HasPrefix(r.Field, customFieldPrefix):
		return c.custom(ctx, strings.TrimPrefix(r.Field, customFieldPrefix), op, r.Value)
	case strings.HasPrefix(r.Field, triggerFieldPrefix):
		return c.triggerRule(strings.TrimPrefix(r.Field, triggerFieldPrefix), op, r.Value)
	}
	f, ok := textFields[r.Field]
	if !ok {
		return nil, fmt.Errorf("unknown field %q", r.Field)
	}
	t, ok := textTests[op]
	if !ok {
		return nil, fmt.Errorf("unsupported operator %q", r.Operator)
	}
	return t.contact(f, r.Value), nil
}

// everyone and nobody are the predicates every Contact and no Contact satisfies. everyone is an
// explicit TRUE, not an empty contact.And(), which writes nothing and so is lost under a NOT.
var (
	everyone = predicate.Contact(func(s *sql.Selector) { s.Where(sql.Not(sql.False())) })
	nobody   = predicate.Contact(func(s *sql.Selector) { s.Where(sql.False()) })
)

// triggerRule is a text test on one property of the triggering Event.
func (c *compiler) triggerRule(key string, op operator, value string) (predicate.Contact, error) {
	t, ok := textTests[op]
	if !ok {
		return nil, fmt.Errorf("unsupported operator %q for trigger field", op)
	}
	return c.fold(key, func(have any) bool { return t.event(have, value) })
}

// fold decides a test on the property key of the triggering Event here, in memory, and makes it
// the constant predicate "everyone" or "no one" for the rest of the tree. A key the Event lacks
// fails the test.
func (c *compiler) fold(key string, holds func(have any) bool) (predicate.Contact, error) {
	if c.trigger == nil {
		return nil, fmt.Errorf("field %q tests the triggering Event and is valid only in a Rule", triggerFieldPrefix+key)
	}
	if key == "" {
		return nil, fmt.Errorf("unknown field %q", triggerFieldPrefix)
	}
	if have, present := (*c.trigger)[key]; present && holds(have) {
		return everyone, nil
	}
	return nobody, nil
}

// number compares a number Custom field (in SQL) or a property of the triggering Event (in Go, by
// the same operator table) with the leaf's value.
func (c *compiler) number(ctx context.Context, l NumberLeaf) (predicate.Contact, error) {
	cmp, ok := comparisons[operator(l.Operator)]
	if !ok {
		return nil, fmt.Errorf("unsupported operator %q for number leaf", l.Operator)
	}
	if l.Value == nil {
		return nil, errors.New("number leaf value is required")
	}
	want := *l.Value
	switch {
	case strings.HasPrefix(l.Field, customFieldPrefix):
		key := strings.TrimPrefix(l.Field, customFieldPrefix)
		if err := c.requireCustomFieldOf(ctx, key, customfield.TypeNumber); err != nil {
			return nil, err
		}
		return jsonPath(key, asIs, cmp.filter, map[string]any{"v": want})
	case strings.HasPrefix(l.Field, triggerFieldPrefix):
		return c.fold(strings.TrimPrefix(l.Field, triggerFieldPrefix), func(have any) bool {
			n, ok := number(have)
			return ok && cmp.holds(n, want)
		})
	default:
		return nil, fmt.Errorf("field %q: a number leaf takes a custom field of type %s or a trigger property", l.Field, customfield.TypeNumber)
	}
}

// date tests which side of the leaf's moment a datetime Custom field's moment is on.
func (c *compiler) date(ctx context.Context, l DateLeaf) (predicate.Contact, error) {
	filter, ok := sides[operator(l.Operator)]
	if !ok {
		return nil, fmt.Errorf("unsupported operator %q for date leaf", l.Operator)
	}
	key, err := c.datetimeField(ctx, l.Field)
	if err != nil {
		return nil, err
	}
	return jsonPath(key, asMoment, filter, map[string]any{"at": l.At.UTC().Format(momentLayout)})
}

// dateWithin tests that a datetime Custom field's moment is within the last WithinDays days up to
// now, so a moment still to come is not within. A window reaching back past the first year starts
// there.
func (c *compiler) dateWithin(ctx context.Context, l DateWithinLeaf) (predicate.Contact, error) {
	if l.WithinDays < 1 {
		return nil, fmt.Errorf("date leaf withinDays must be 1 or more, got %d", l.WithinDays)
	}
	key, err := c.datetimeField(ctx, l.Field)
	if err != nil {
		return nil, err
	}
	since := c.now.AddDate(0, 0, -l.WithinDays)
	if since.Year() < 1 {
		since = time.Date(1, time.January, 1, 0, 0, 0, 0, time.UTC)
	}
	return jsonPath(key, asMoment, window, map[string]any{
		"since": since.UTC().Format(momentLayout), "now": c.now.UTC().Format(momentLayout),
	})
}

// datetimeField is the key of the datetime Custom field a date leaf tests. An Event's property has
// no type, so a date leaf takes no "trigger:" field.
func (c *compiler) datetimeField(ctx context.Context, field string) (string, error) {
	if !strings.HasPrefix(field, customFieldPrefix) {
		return "", fmt.Errorf("field %q: a date leaf takes a custom field of type %s", field, customfield.TypeDatetime)
	}
	key := strings.TrimPrefix(field, customFieldPrefix)
	if err := c.requireCustomFieldOf(ctx, key, customfield.TypeDatetime); err != nil {
		return "", err
	}
	return key, nil
}

// target writes the JSON value a jsonpath filter reads, from the custom_fields column and key.
type target func(b *sql.Builder, column, key string)

// asIs is the value of key as it is.
func asIs(b *sql.Builder, column, key string) {
	b.WriteString(column).WriteString(" -> ").Arg(key).WriteString("::text")
}

// asMoment is the value of key as text with a trailing Z written +00:00. jsonpath's datetime()
// reads an offset as ±hh[:mm] on the older Postgres majors this runs on (14 among them), so a Z,
// as JavaScript and RFC 3339 write UTC, would make a moment no moment. A value of another type
// becomes its text, which is no date-time; a missing key stays NULL.
func asMoment(b *sql.Builder, column, key string) {
	b.WriteString("to_jsonb(regexp_replace(").WriteString(column).WriteString(" ->> ").Arg(key).
		WriteString("::text, '[Zz]$', '+00:00'))")
}

// jsonPath is the jsonpath filter holding for the value of key in the custom_fields column, read by
// at, with vars bound to its $names. In strict mode a list is not unwrapped, and a value of another
// type, a string that is no date or a missing key makes it false.
func jsonPath(key string, at target, filter string, vars map[string]any) (predicate.Contact, error) {
	operands, err := json.Marshal(vars)
	if err != nil {
		return nil, fmt.Errorf("encode operands: %w", err)
	}
	path := "strict $ ? (" + filter + ")"
	return predicate.Contact(func(s *sql.Selector) {
		column := s.C(contact.FieldCustomFields)
		// raw sql: ent has no jsonpath predicate, and its JSON comparisons (sqljson.ValueGT …) cast
		// the value, which fails the whole query on one Contact's value of another type. A jsonpath
		// filter in silent mode is false there instead; COALESCE makes a missing key or column false,
		// not NULL, so a negated group holds for it as for a Rule's folded leaf. The key, the filter
		// and the operands are bound arguments.
		s.Where(sql.P(func(b *sql.Builder) {
			b.WriteString("COALESCE(jsonb_path_exists(")
			at(b, column, key)
			b.WriteString(", ").Arg(path).WriteString("::jsonpath, ").Arg(string(operands)).
				WriteString("::jsonb, true), false)")
		}))
	}), nil
}

// eventCount is one operator of an event leaf: the least times it takes, and the test of the
// Contact's count, given the correlated subquery that selects it.
type eventCount struct {
	least int
	holds func(count *sql.Selector, times int) *sql.Predicate
}

// eventOperator is how an event leaf bounds the count of the Contact's Events.
type eventOperator string

const (
	atLeast eventOperator = "atLeast"
	atMost  eventOperator = "atMost"
)

// eventCounts is each event operator with the bound it takes.
var eventCounts = map[eventOperator]eventCount{
	// Without a GROUP BY the count is one row even over no Events, so HAVING decides alone.
	atLeast: {least: 1, holds: func(count *sql.Selector, times int) *sql.Predicate {
		return sql.Exists(count.Having(sql.GTE(sql.Count("*"), times)))
	}},
	atMost: {least: 0, holds: func(count *sql.Selector, times int) *sql.Predicate {
		return sql.NotExists(count.Having(sql.GT(sql.Count("*"), times)))
	}},
}

// event is the subject performed `action` at least (or at most) Times times, over all time or
// within the last WithinDays days. Events attach to a Contact by the stable contact_id, so
// anonymous events stitched onto it at Identify are counted here (ADR 0002), and the count is
// correlated on it, so an anonymous Event (a NULL contact_id) is in no Contact's count.
func (c *compiler) event(l EventLeaf) (predicate.Contact, error) {
	action := strings.TrimSpace(l.Action)
	if action == "" {
		return nil, errors.New("event action is required")
	}
	op, ok := eventCounts[eventOperator(l.Operator)]
	if !ok {
		return nil, fmt.Errorf("unsupported operator %q for event leaf", l.Operator)
	}
	if l.Times == nil {
		return nil, errors.New("event leaf times is required")
	}
	times := *l.Times
	if times < op.least {
		return nil, fmt.Errorf("%s takes times of %d or more, got %d", l.Operator, op.least, times)
	}

	preds := []predicate.Event{event.Action(action)}
	if events.IsEngagement(action) {
		// A machine open or click is in the log and in no count (ADR 0053).
		preds = append(preds, events.HumanOnly)
	}
	if l.WithinDays != nil {
		days := *l.WithinDays
		if days < 1 {
			return nil, fmt.Errorf("event leaf withinDays must be 1 or more, got %d", days)
		}
		// A window reaching back past the first year counts all time: no Event is older, and a
		// cutoff that far back would overflow a stored timestamp. A window longer than Event
		// retention is capped by it (ADR 0019). GTE excludes a NULL occurred_at, which no
		// persisted Event has: the events bus always stamps it.
		if since := c.now.AddDate(0, 0, -days); since.Year() >= 1 {
			preds = append(preds, event.OccurredAtGTE(since))
		}
	}

	return predicate.Contact(func(s *sql.Selector) {
		t := sql.Table(event.Table)
		count := sql.Dialect(s.Dialect()).Select(sql.Count("*")).From(t)
		count.Where(sql.ColumnsEQ(count.C(event.FieldContactID), s.C(contact.FieldID)))
		for _, p := range preds {
			p(count)
		}
		s.Where(op.holds(count, times))
	}), nil
}

// tagRule is the subject has (or lacks) the Tag named value. Evaluated live —
// membership is never materialized.
func tagRule(op operator, value string) (predicate.Contact, error) {
	var negate bool
	switch op {
	case opHas:
	case opDoesNotHave:
		negate = true
	default:
		return nil, fmt.Errorf("unsupported operator %q for tag field", op)
	}
	name := strings.TrimSpace(value)
	if name == "" {
		return nil, errors.New("tag name is required")
	}

	p := contact.HasTagsWith(tag.Name(name))
	if negate {
		p = contact.Not(p)
	}
	return p, nil
}

// custom is a condition on one Custom field. The key must be in the Workspace's
// catalogue (ADR 0006), the operator must fit its type (RuleOperators), and it is addressed with
// ent's JSON predicates (sqljson), since no generated predicate addresses a key of the
// custom_fields JSON column.
func (c *compiler) custom(ctx context.Context, key string, op operator, value string) (predicate.Contact, error) {
	t, err := c.customFieldType(ctx, key)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(customFieldOperators[t], op) {
		return nil, fmt.Errorf("unsupported operator %q for json field %q of type %s", op, key, t)
	}

	// ent inlines a JSON path key into the statement as a quoted literal instead of binding
	// it: quotes are doubled so any key stays one literal (standard_conforming_strings
	// makes a backslash plain), and a key ent would read as an array index is refused.
	if isIndexKey(key) {
		return nil, fmt.Errorf("unsupported custom field key %q", key)
	}
	path := sqljson.Path(strings.ReplaceAll(key, "'", "''"))
	var cond func(column string) *sql.Predicate
	switch op {
	case opEQ:
		cond = func(column string) *sql.Predicate { return sqljson.ValueEQ(column, value, path) }
	case opNEQ:
		cond = func(column string) *sql.Predicate { return sqljson.ValueNEQ(column, value, path) }
	case opContains:
		// Membership of a JSON array (jsonb containment of the encoded value). The value
		// is read as JSON, not unquoted to text, so a string element matches as a string.
		cond = func(column string) *sql.Predicate {
			return sqljson.ValueContains(column, value, path, sqljson.Unquote(false))
		}
	case opNotNull:
		cond = func(column string) *sql.Predicate { return sqljson.HasKey(column, path) }
	default:
		return nil, fmt.Errorf("unsupported operator %q for json field", op)
	}
	return predicate.Contact(func(s *sql.Selector) {
		s.Where(cond(s.C(contact.FieldCustomFields)))
	}), nil
}

// isIndexKey reports whether ent would read key as an array index ("[0]") rather than
// an object key.
func isIndexKey(key string) bool {
	return len(key) > 2 && key[0] == '[' && key[len(key)-1] == ']'
}

// ErrStorage marks a failure to read what compiling needs from the database, as opposed to a
// condition that does not compile.
var ErrStorage = errors.New("condition: storage")

// customFieldType checks key against the Workspace's Custom field catalogue and returns its type.
func (c *compiler) customFieldType(ctx context.Context, key string) (customfield.Type, error) {
	if key == "" {
		return "", fmt.Errorf("unknown field %q", customFieldPrefix)
	}
	if c.keys == nil {
		fields, err := c.s.CustomField().Query().All(ctx)
		if err != nil {
			return "", fmt.Errorf("%w: load custom fields: %w", ErrStorage, err)
		}
		c.keys = make(map[string]customfield.Type, len(fields))
		for _, f := range fields {
			c.keys[f.Key] = f.Type
		}
	}
	t, ok := c.keys[key]
	if !ok {
		return "", fmt.Errorf("unknown custom field %q", key)
	}
	return t, nil
}

// requireCustomFieldOf checks that key is a Custom field of the Workspace of type want.
func (c *compiler) requireCustomFieldOf(ctx context.Context, key string, want customfield.Type) error {
	t, err := c.customFieldType(ctx, key)
	if err != nil {
		return err
	}
	if t != want {
		return fmt.Errorf("custom field %q is of type %s, not %s", key, t, want)
	}
	return nil
}

// Encode stores a condition given as an API DTO. The API's typed tree is the stored
// shape: a condition is saved as its JSON, and ContactPredicate reads it back.
func Encode(group json.Marshaler) (string, error) {
	b, err := group.MarshalJSON()
	if err != nil {
		return "", fmt.Errorf("encode condition: %w", err)
	}
	return string(b), nil
}

// EncodeOptional is Encode for a condition the request may leave out: nil when it is absent.
func EncodeOptional[G any, P interface {
	*G
	json.Marshaler
}](opt interface{ Get() (G, bool) }) (*string, error) {
	group, ok := opt.Get()
	if !ok {
		return nil, nil
	}
	def, err := Encode(P(&group))
	if err != nil {
		return nil, err
	}
	return &def, nil
}

// Decode loads a stored condition into an API DTO.
func Decode(def string, into json.Unmarshaler) error {
	if err := into.UnmarshalJSON([]byte(def)); err != nil {
		return fmt.Errorf("decode condition: %w", err)
	}
	return nil
}

// Size counts the nodes of the group: itself and every nested group and rule, at any depth. A
// malformed entry counts as one node.
func (g Group) Size() int {
	n := 1
	for _, raw := range g.Rules {
		var sub Group
		if json.Unmarshal(raw, &sub) == nil && sub.Kind == kindGroup {
			n += sub.Size()
			continue
		}
		n++
	}
	return n
}

// NamesTag reports whether any rule of the group, at any depth, targets the Tag called name.
// A malformed condition names nothing.
func (g Group) NamesTag(name string) bool {
	for _, raw := range g.Rules {
		var probe struct {
			Kind string `json:"kind"`
		}
		if json.Unmarshal(raw, &probe) != nil {
			continue
		}
		switch probe.Kind {
		case kindGroup:
			var sub Group
			if json.Unmarshal(raw, &sub) == nil && sub.NamesTag(name) {
				return true
			}
		case kindRule:
			var r Leaf
			if json.Unmarshal(raw, &r) == nil && r.Field == tagField && strings.TrimSpace(r.Value) == name {
				return true
			}
		}
	}
	return false
}
