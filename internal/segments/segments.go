// Package segments compiles a Segment's rule query (the react-querybuilder JSON the UI
// emits) into a Contact predicate: a group {combinator: and|or, not, rules[]} whose
// entries are nested groups or leaves {field, operator, value}.
//
// A leaf is built from ent's generated predicates wherever one exists: the core
// attributes (contact.EmailEQ …), Tags (contact.HasTagsWith) and Events
// (contact.HasEventsWith). The one dynamic piece is a Custom field's key inside the
// custom_fields JSON column, which no generated predicate covers; it is a bound
// argument, never part of the query text. A leaf may only name a core attribute, a
// Tag, an Event action, or a Custom field of the Workspace's catalogue (ADR 0006).
package segments

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"

	"github.com/mokevnin/sphericon/ent"
	"github.com/mokevnin/sphericon/ent/contact"
	"github.com/mokevnin/sphericon/ent/event"
	"github.com/mokevnin/sphericon/ent/predicate"
	"github.com/mokevnin/sphericon/ent/tag"
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
)

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

// ContactPredicate parses a stored definition and compiles it for the Workspace of s.
func ContactPredicate(ctx context.Context, s *ent.Scoped, def string) (predicate.Contact, error) {
	g, err := Parse(def)
	if err != nil {
		return nil, err
	}
	return Compile(ctx, s, g)
}

// Compile turns a definition group into a Contact predicate. An empty rule set
// matches everyone (an empty negated group matches no one) — deliberate, so an
// empty rule is never a silent "nobody".
func Compile(ctx context.Context, s *ent.Scoped, g Group) (predicate.Contact, error) {
	return (&compiler{ctx: ctx, s: s, now: time.Now()}).group(g)
}

// compiler holds what one compilation needs: the Workspace's Custom field keys are
// loaded the first time a rule names one.
type compiler struct {
	ctx  context.Context
	s    *ent.Scoped
	now  time.Time
	keys map[string]struct{}
}

func (c *compiler) group(g Group) (predicate.Contact, error) {
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
	if strings.EqualFold(g.Combinator, "or") {
		p = contact.Or(preds...)
	}
	if g.Not {
		p = contact.Not(p)
	}
	return p, nil
}

// entry decodes a rules[] entry: a nested group when it carries a combinator (or has
// no field), a leaf rule otherwise.
func (c *compiler) entry(raw json.RawMessage) (predicate.Contact, error) {
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
		return c.group(g)
	}
	var r Rule
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("malformed rule: %w", err)
	}
	return c.rule(r)
}

func (c *compiler) rule(r Rule) (predicate.Contact, error) {
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
	}
	f, ok := textFields[r.Field]
	if !ok {
		return nil, fmt.Errorf("unknown field %q", r.Field)
	}
	switch r.Operator {
	case "=":
		return f.eq(r.Value), nil
	case "!=":
		return f.neq(r.Value), nil
	case "contains":
		return f.contains(r.Value), nil
	case "beginsWith":
		return f.beginsWith(r.Value), nil
	case "null":
		return f.null, nil
	case "notNull":
		return f.notNull, nil
	default:
		return nil, fmt.Errorf("unsupported operator %q", r.Operator)
	}
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
// catalogue (ADR 0006), and it reaches the query only as a bound argument: no
// generated predicate addresses a key of the custom_fields JSON column.
func (c *compiler) custom(key, op, value string) (predicate.Contact, error) {
	if key == "" {
		return nil, fmt.Errorf("unknown field %q", customFieldPrefix)
	}
	if err := c.requireCustomField(key); err != nil {
		return nil, err
	}

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
	return predicate.Contact(func(s *sql.Selector) {
		s.Where(sql.P(func(b *sql.Builder) { write(b, s.C(contact.FieldCustomFields)) }))
	}), nil
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
