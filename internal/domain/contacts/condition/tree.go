package condition

import (
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/internal/kernel/bounds"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/fieldname"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// The stored shape of a leaf (ADR 0029, ADR 0054): a string `field` whose prefix says what it
// tests, one operator enum for every kind of value, and the `rule`, `number`, `date` and
// `dateWithin` kinds. Only the decode below reads it; everything else works on the typed tree.

const (
	// customFieldPrefix marks a leaf as a Custom field condition: "custom:<key>".
	customFieldPrefix = "custom:"
	// tagField marks a leaf as a Tag condition: field "tag" with operator "has" or
	// "doesNotHave" and value = the Tag name.
	tagField = "tag"

	// telegramField marks a leaf as the Telegram condition (ADR 0075): operator "has" or
	// "doesNotHave", no value.
	telegramField = "telegram"

	// smsField marks a leaf as the "can be texted" condition: operator "has" or "doesNotHave", no
	// value.
	smsField = "sms"
	// triggerFieldPrefix marks a leaf as a test on the triggering Event's own properties:
	// "trigger:<key>". Valid only in a Rule (RulePredicate); a Segment refuses it.
	triggerFieldPrefix = "trigger:"

	// birthdayField is the one core attribute a date leaf tests: the Contact's date of birth.
	birthdayField = "birthday"
)

const (
	kindRule       = "rule"
	kindEvent      = "event"
	kindNumber     = "number"
	kindDate       = "date"
	kindDateWithin = "dateWithin"
)

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

// group is a decoded group of the tree: its entries hold, all of them or any, and the result is
// negated when not is set.
type group struct {
	combinator conditiontree.Combinator
	not        bool
	entries    []node
}

// node is an entry of a group: a nested group or one of the leaves below.
type node interface{ isNode() }

// attributeLeaf tests a core attribute of the Contact: a text attribute (textFields) by a text or
// presence test, the birthday by a moment test.
type attributeLeaf struct {
	name string
	test test
}

// tagLeaf tests whether the Contact has the Tag called name.
type tagLeaf struct {
	name string
	test presenceTest
}

// telegramLeaf tests whether the Contact can be reached on Telegram.
type telegramLeaf struct{ test presenceTest }

// smsLeaf tests whether the Contact can be texted.
type smsLeaf struct{ test presenceTest }

// customFieldLeaf tests the Custom field key by a text, presence, number or moment test. Whether
// the test fits the field's type is data, checked when the leaf compiles (fits).
type customFieldLeaf struct {
	key  string
	test test
}

// eventCountLeaf counts the Contact's Events of action: at least or at most times of them, over
// all time or within the last withinDays days (nil for all time).
type eventCountLeaf struct {
	action     string
	op         eventOperator
	times      int
	withinDays *int
}

// subject is whose properties a property test reads.
type subject string

// subjectTrigger is the Event that fired a Rule.
const subjectTrigger subject = "trigger"

// propertyLeaf tests the property key of its subject by a text, presence or number test.
type propertyLeaf struct {
	subject subject
	key     string
	test    test
}

func (group) isNode()           {}
func (attributeLeaf) isNode()   {}
func (tagLeaf) isNode()         {}
func (telegramLeaf) isNode()    {}
func (smsLeaf) isNode()         {}
func (customFieldLeaf) isNode() {}
func (eventCountLeaf) isNode()  {}
func (propertyLeaf) isNode()    {}

// test is what a leaf asks of the value it reads, typed by that value: text, presence, number or
// moment.
type test interface{ isTest() }

// textTest compares a text value with value: eq, neq, contains or beginsWith (textOperators). A number
// or boolean Custom field's eq and neq are text tests too, compared with the stored value's text.
type textTest struct {
	op    operator
	value string
}

// presenceTest asks whether a value is set (or a Tag, a Telegram Binding, an eligible phone had),
// or not.
type presenceTest struct{ set bool }

// numberTest compares a number with value: gt, gte, lt or lte (comparisons).
type numberTest struct {
	op    operator
	value float64
}

// momentTest places a moment: before at, or at or after it (sides), or within the last withinDays
// days up to now (opWithin).
type momentTest struct {
	op         operator
	at         time.Time
	withinDays int
}

func (textTest) isTest()     {}
func (presenceTest) isTest() {}
func (numberTest) isTest()   {}
func (momentTest) isTest()   {}

// leaves yields every leaf of the group, at any depth and under a negation too.
func (g group) leaves() iter.Seq[node] {
	return func(yield func(node) bool) {
		for _, n := range g.entries {
			if sub, ok := n.(group); ok {
				for l := range sub.leaves() {
					if !yield(l) {
						return
					}
				}
				continue
			}
			if !yield(n) {
				return
			}
		}
	}
}

// decode reads a stored Condition into the typed tree, refusing what it cannot read with the
// path of the entry ("rule 0: rule 2: …") and what it cannot read there. It reads the tree alone:
// whether a named Custom field exists, and whether its test fits the field's type, are the
// compiler's checks.
func decode(g conditiontree.Group) (group, error) {
	if g.Kind != conditiontree.KindGroup {
		return group{}, fmt.Errorf("unknown group kind %q", g.Kind)
	}
	if g.Combinator != conditiontree.And && g.Combinator != conditiontree.Or {
		return group{}, fmt.Errorf("unknown combinator %q", g.Combinator)
	}
	d := group{combinator: g.Combinator, not: g.Not, entries: make([]node, 0, len(g.Rules))}
	for i, raw := range g.Rules {
		n, err := decodeEntry(raw, func(sub conditiontree.Group) (node, error) { return decode(sub) })
		if err != nil {
			return group{}, fmt.Errorf("rule %d: %w", i, err)
		}
		d.entries = append(d.entries, n)
	}
	return d, nil
}

// decodeSkipping is the stored Condition as the readers of what it names see it: an entry decode
// refuses tests nothing and is left out, so one bad entry does not hide the others.
func decodeSkipping(g conditiontree.Group) group {
	d := group{combinator: g.Combinator, not: g.Not, entries: make([]node, 0, len(g.Rules))}
	for _, raw := range g.Rules {
		n, err := decodeEntry(raw, func(sub conditiontree.Group) (node, error) { return decodeSkipping(sub), nil })
		if err != nil {
			continue
		}
		d.entries = append(d.entries, n)
	}
	return d
}

// decodeEntry reads a rules[] entry by its `kind`: a nested group, which sub reads, or a leaf.
func decodeEntry(raw json.RawMessage, sub func(conditiontree.Group) (node, error)) (node, error) {
	var probe struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, fmt.Errorf("malformed rule: %w", err)
	}
	switch probe.Kind {
	case conditiontree.KindGroup:
		var g conditiontree.Group
		if err := json.Unmarshal(raw, &g); err != nil {
			return nil, fmt.Errorf("malformed group: %w", err)
		}
		return sub(g)
	case kindRule:
		var l Leaf
		if err := json.Unmarshal(raw, &l); err != nil {
			return nil, fmt.Errorf("malformed rule: %w", err)
		}
		return decodeRule(l)
	case kindEvent:
		var l EventLeaf
		if err := json.Unmarshal(raw, &l); err != nil {
			return nil, fmt.Errorf("malformed event leaf: %w", err)
		}
		return decodeEvent(l)
	case kindNumber:
		var l NumberLeaf
		if err := json.Unmarshal(raw, &l); err != nil {
			return nil, fmt.Errorf("malformed number leaf: %w", err)
		}
		return decodeNumber(l)
	case kindDate:
		var l DateLeaf
		if err := json.Unmarshal(raw, &l); err != nil {
			return nil, fmt.Errorf("malformed date leaf: %w", err)
		}
		return decodeDate(l)
	case kindDateWithin:
		var l DateWithinLeaf
		if err := json.Unmarshal(raw, &l); err != nil {
			return nil, fmt.Errorf("malformed date leaf: %w", err)
		}
		return decodeDateWithin(l)
	default:
		return nil, fmt.Errorf("unknown rule kind %q", probe.Kind)
	}
}

// decodeRule reads a `rule` leaf: what its field names, tested by the operator as a text or a
// presence test.
func decodeRule(r Leaf) (node, error) {
	op := operator(r.Operator)
	switch {
	case r.Field == tagField:
		has, err := hasOrNot(op, "tag")
		if err != nil {
			return nil, err
		}
		name := strings.TrimSpace(r.Value)
		if name == "" {
			return nil, errors.New("tag name is required")
		}
		return tagLeaf{name: name, test: has}, nil
	case r.Field == telegramField:
		has, err := hasOrNot(op, "telegram")
		if err != nil {
			return nil, err
		}
		return telegramLeaf{test: has}, nil
	case r.Field == smsField:
		has, err := hasOrNot(op, "sms")
		if err != nil {
			return nil, err
		}
		return smsLeaf{test: has}, nil
	case strings.HasPrefix(r.Field, customFieldPrefix):
		key, err := keyOf(r.Field, customFieldPrefix)
		if err != nil {
			return nil, err
		}
		t, ok := valueTest(op, r.Value)
		if !ok {
			return nil, fmt.Errorf("unsupported operator %q for json field %q", op, key)
		}
		return customFieldLeaf{key: key, test: t}, nil
	case strings.HasPrefix(r.Field, triggerFieldPrefix):
		t, ok := valueTest(op, r.Value)
		if !ok {
			return nil, fmt.Errorf("unsupported operator %q for trigger field", op)
		}
		key, err := keyOf(r.Field, triggerFieldPrefix)
		if err != nil {
			return nil, err
		}
		return propertyLeaf{subject: subjectTrigger, key: key, test: t}, nil
	}
	if _, ok := textFields[r.Field]; !ok {
		return nil, fmt.Errorf("unknown field %q", r.Field)
	}
	t, ok := valueTest(op, r.Value)
	if !ok {
		return nil, fmt.Errorf("unsupported operator %q", r.Operator)
	}
	return attributeLeaf{name: r.Field, test: t}, nil
}

// hasOrNot is the presence test of a Tag, Telegram or SMS leaf: has or doesNotHave.
func hasOrNot(op operator, field string) (presenceTest, error) {
	switch op {
	case opHas:
		return presenceTest{set: true}, nil
	case opDoesNotHave:
		return presenceTest{set: false}, nil
	default:
		return presenceTest{}, fmt.Errorf("unsupported operator %q for %s field", op, field)
	}
}

// valueTest is the test of a value a `rule` leaf's operator names: a text test, or null and
// notNull as the presence test. false for any other operator.
func valueTest(op operator, value string) (test, bool) {
	switch op {
	case opNull:
		return presenceTest{set: false}, true
	case opNotNull:
		return presenceTest{set: true}, true
	default:
		if _, ok := textOperators[op]; !ok {
			return nil, false
		}
		return textTest{op: op, value: value}, true
	}
}

// keyOf is the key a prefixed field names, refused when there is none.
func keyOf(field, prefix string) (string, error) {
	key := strings.TrimPrefix(field, prefix)
	if key == "" {
		return "", fmt.Errorf("unknown field %q", prefix)
	}
	return key, nil
}

// decodeNumber reads a `number` leaf: a number Custom field's or a triggering Event's property's
// number test.
func decodeNumber(l NumberLeaf) (node, error) {
	op := operator(l.Operator)
	if _, ok := comparisons[op]; !ok {
		return nil, fmt.Errorf("unsupported operator %q for number leaf", l.Operator)
	}
	if l.Value == nil {
		return nil, errors.New("number leaf value is required")
	}
	t := numberTest{op: op, value: *l.Value}
	switch {
	case strings.HasPrefix(l.Field, customFieldPrefix):
		key, err := keyOf(l.Field, customFieldPrefix)
		if err != nil {
			return nil, err
		}
		return customFieldLeaf{key: key, test: t}, nil
	case strings.HasPrefix(l.Field, triggerFieldPrefix):
		key, err := keyOf(l.Field, triggerFieldPrefix)
		if err != nil {
			return nil, err
		}
		return propertyLeaf{subject: subjectTrigger, key: key, test: t}, nil
	default:
		return nil, fmt.Errorf("field %q: a number leaf takes a custom field of type %s or a trigger property", l.Field, customfield.TypeNumber)
	}
}

// decodeDate reads a `date` leaf: the moment test of the birthday or a datetime Custom field
// against a bound.
func decodeDate(l DateLeaf) (node, error) {
	op := operator(l.Operator)
	if _, ok := sides[op]; !ok {
		return nil, fmt.Errorf("unsupported operator %q for date leaf", l.Operator)
	}
	return momentLeaf(l.Field, momentTest{op: op, at: l.At})
}

// decodeDateWithin reads a `dateWithin` leaf: the moment test of the birthday or a datetime Custom
// field over the last days.
func decodeDateWithin(l DateWithinLeaf) (node, error) {
	if err := validation.Min(fieldname.WithinDays, l.WithinDays, bounds.ConditionWithinDaysMin); err != nil {
		return nil, boundError{err}
	}
	return momentLeaf(l.Field, momentTest{op: opWithin, withinDays: l.WithinDays})
}

// momentLeaf is the leaf of a moment test on field: the birthday or a Custom field. An Event's
// property has no type, so a moment test takes no "trigger:" field.
func momentLeaf(field string, t momentTest) (node, error) {
	if field == birthdayField {
		return attributeLeaf{name: birthdayField, test: t}, nil
	}
	if !strings.HasPrefix(field, customFieldPrefix) {
		return nil, fmt.Errorf("field %q: a date leaf takes a custom field of type %s", field, customfield.TypeDatetime)
	}
	key, err := keyOf(field, customFieldPrefix)
	if err != nil {
		return nil, err
	}
	return customFieldLeaf{key: key, test: t}, nil
}

// decodeEvent reads an `event` leaf: a count of the Contact's Events of one action.
func decodeEvent(l EventLeaf) (node, error) {
	action := strings.TrimSpace(l.Action)
	if action == "" {
		return nil, errors.New("event action is required")
	}
	op := eventOperator(l.Operator)
	count, ok := eventCounts[op]
	if !ok {
		return nil, fmt.Errorf("unsupported operator %q for event leaf", l.Operator)
	}
	if l.Times == nil {
		return nil, errors.New("event leaf times is required")
	}
	times := *l.Times
	if err := validation.Min(fieldname.Times, times, bounds.ConditionEventTimesMin); err != nil {
		return nil, boundError{err}
	}
	if times < count.least {
		return nil, fmt.Errorf("%s takes times of %d or more, got %d", l.Operator, count.least, times)
	}
	if l.WithinDays != nil {
		if err := validation.Min(fieldname.WithinDays, *l.WithinDays, bounds.ConditionWithinDaysMin); err != nil {
			return nil, boundError{err}
		}
	}
	return eventCountLeaf{action: action, op: op, times: times, withinDays: l.WithinDays}, nil
}
