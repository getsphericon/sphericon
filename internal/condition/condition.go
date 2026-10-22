// Package condition is the Condition: the typed tree of tests on a Contact (the API
// contract's tree, ADR 0029, ADR 0054), its validation and its compiler to a Contact
// predicate. A Segment is a named Condition and a Rule holds one. A group {kind: "group", combinator: and|or, not,
// rules[]} whose entries are nested groups or leaves {kind: "rule", field, operator,
// value}, told apart by `kind`.
//
// A leaf is built from ent's generated predicates wherever one exists: the core
// attributes (contact.EmailEQ …), Tags (contact.HasTagsWith) and Events
// (contact.HasEventsWith). The one dynamic piece is a Custom field's key inside the
// custom_fields JSON column, which no generated predicate covers; it is addressed
// with ent's JSON predicates (sqljson) after the key is checked. A leaf may only name a core attribute, a
// Tag, an Event action, or a Custom field of the Workspace's catalogue (ADR 0006).
package condition

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/tag"
	"github.com/getsphericon/sphericon/internal/events"
)

const (
	// eventFieldPrefix marks a leaf as a behavioral condition: field "event:<action>"
	// with operator "performed"/"notPerformed" and value = a day window ("" = ever).
	eventFieldPrefix = "event:"
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
	kindGroup = "group"
	kindRule  = "rule"
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
	return (&compiler{ctx: ctx, s: s, now: time.Now()}).group(g)
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
	return (&compiler{ctx: ctx, s: s, now: time.Now(), trigger: &props}).group(g)
}

// compiler holds what one compilation needs: the Workspace's Custom field keys are
// loaded the first time a rule names one.
type compiler struct {
	ctx  context.Context
	s    *ent.Scoped
	now  time.Time
	keys map[string]struct{}
	// trigger is the properties of the Event a Rule's "trigger:" leaves read; nil in a Segment,
	// where such a leaf is refused.
	trigger *map[string]any
}

func (c *compiler) group(g Group) (predicate.Contact, error) {
	if g.Kind != kindGroup {
		return nil, fmt.Errorf("unknown group kind %q", g.Kind)
	}
	if g.Combinator != "and" && g.Combinator != "or" {
		return nil, fmt.Errorf("unknown combinator %q", g.Combinator)
	}
	preds := make([]predicate.Contact, 0, len(g.Rules))
	for i, raw := range g.Rules {
		p, err := c.entry(raw)
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
func (c *compiler) entry(raw json.RawMessage) (predicate.Contact, error) {
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
		return c.group(g)
	case kindRule:
		var r Leaf
		if err := json.Unmarshal(raw, &r); err != nil {
			return nil, fmt.Errorf("malformed rule: %w", err)
		}
		return c.rule(r)
	default:
		return nil, fmt.Errorf("unknown rule kind %q", probe.Kind)
	}
}

func (c *compiler) rule(r Leaf) (predicate.Contact, error) {
	switch {
	case strings.HasPrefix(r.Field, eventFieldPrefix):
		action := strings.TrimPrefix(r.Field, eventFieldPrefix)
		if action == "" {
			return nil, fmt.Errorf("unknown field %q", r.Field)
		}
		return c.event(action, r.Operator, r.Value)
	case r.Field == tagField:
		return tagRule(r.Operator, r.Value)
	case strings.HasPrefix(r.Field, customFieldPrefix):
		return c.custom(strings.TrimPrefix(r.Field, customFieldPrefix), r.Operator, r.Value)
	case strings.HasPrefix(r.Field, triggerFieldPrefix):
		return c.triggerRule(strings.TrimPrefix(r.Field, triggerFieldPrefix), r.Operator, r.Value)
	}
	f, ok := textFields[r.Field]
	if !ok {
		return nil, fmt.Errorf("unknown field %q", r.Field)
	}
	op, ok := operators[r.Operator]
	if !ok {
		return nil, fmt.Errorf("unsupported operator %q", r.Operator)
	}
	return op.contact(f, r.Value), nil
}

// triggerRule is a test on one property of the triggering Event. It is decided here, in memory,
// and becomes the constant predicate "everyone" or "no one" for the rest of the tree.
func (c *compiler) triggerRule(key, op, value string) (predicate.Contact, error) {
	if c.trigger == nil {
		return nil, fmt.Errorf("field %q tests the triggering Event and is valid only in a Rule", triggerFieldPrefix+key)
	}
	if key == "" {
		return nil, fmt.Errorf("unknown field %q", triggerFieldPrefix)
	}
	o, ok := operators[op]
	if !ok {
		return nil, fmt.Errorf("unsupported operator %q for trigger field", op)
	}
	if have, present := (*c.trigger)[key]; present && o.event(have, value) {
		return contact.And(), nil
	}
	return predicate.Contact(func(s *sql.Selector) { s.Where(sql.False()) }), nil
}

// event is the subject performed (or never performed) `action`, optionally within the
// last `value` days. Events attach to a Contact by the stable contact_id, so anonymous
// events stitched onto it at Identify are visible here (ADR 0002).
func (c *compiler) event(action, op, value string) (predicate.Contact, error) {
	var negate bool
	switch op {
	case "performed":
	case "notPerformed":
		negate = true
	default:
		return nil, fmt.Errorf("unsupported operator %q for event field", op)
	}

	preds := []predicate.Event{event.Action(action)}
	if events.IsEngagement(action) {
		// A machine open or click is in the log and in no count (ADR 0053).
		preds = append(preds, events.HumanOnly)
	}
	if v := strings.TrimSpace(value); v != "" {
		days, err := strconv.Atoi(v)
		if err != nil || days < 0 {
			return nil, fmt.Errorf("event window must be a non-negative number of days, got %q", value)
		}
		if days > 0 {
			// GTE excludes NULL occurred_at. Safe in practice: the events bus always
			// stamps occurred_at (zero ⇒ publish time), so persisted events are never
			// null. "ever" adds no time predicate and matches all.
			preds = append(preds, event.OccurredAtGTE(c.now.AddDate(0, 0, -days)))
		}
	}

	p := contact.HasEventsWith(preds...)
	if negate {
		p = contact.Not(p)
	}
	return p, nil
}

// tagRule is the subject has (or lacks) the Tag named value. Evaluated live —
// membership is never materialized.
func tagRule(op, value string) (predicate.Contact, error) {
	var negate bool
	switch op {
	case "has":
	case "doesNotHave":
		negate = true
	default:
		return nil, fmt.Errorf("unsupported operator %q for tag field", op)
	}
	name := strings.TrimSpace(value)
	if name == "" {
		return nil, fmt.Errorf("tag name is required")
	}

	p := contact.HasTagsWith(tag.Name(name))
	if negate {
		p = contact.Not(p)
	}
	return p, nil
}

// custom is a condition on one Custom field. The key must be in the Workspace's
// catalogue (ADR 0006), and it is addressed with ent's JSON predicates (sqljson), since no
// generated predicate addresses a key of the custom_fields JSON column.
func (c *compiler) custom(key, op, value string) (predicate.Contact, error) {
	if key == "" {
		return nil, fmt.Errorf("unknown field %q", customFieldPrefix)
	}
	if err := c.requireCustomField(key); err != nil {
		return nil, err
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
	case "eq":
		cond = func(column string) *sql.Predicate { return sqljson.ValueEQ(column, value, path) }
	case "neq":
		cond = func(column string) *sql.Predicate { return sqljson.ValueNEQ(column, value, path) }
	case "contains":
		// Membership of a JSON array (jsonb containment of the encoded value). The value
		// is read as JSON, not unquoted to text, so a string element matches as a string.
		cond = func(column string) *sql.Predicate {
			return sqljson.ValueContains(column, value, path, sqljson.Unquote(false))
		}
	case "notNull":
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

// requireCustomField checks key against the Workspace's Custom field catalogue.
func (c *compiler) requireCustomField(key string) error {
	if c.keys == nil {
		fields, err := c.s.CustomField().Query().All(c.ctx)
		if err != nil {
			return fmt.Errorf("load custom fields: %w", err)
		}
		c.keys = make(map[string]struct{}, len(fields))
		for _, f := range fields {
			c.keys[f.Key] = struct{}{}
		}
	}
	if _, ok := c.keys[key]; !ok {
		return fmt.Errorf("unknown custom field %q", key)
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

// Decode loads a stored condition into an API DTO.
func Decode(def string, into json.Unmarshaler) error {
	if err := into.UnmarshalJSON([]byte(def)); err != nil {
		return fmt.Errorf("decode condition: %w", err)
	}
	return nil
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
