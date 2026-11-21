package condition

import (
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/internal/kernel/bounds"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/fieldname"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// The `kind` of each leaf of the stored tree (ADR 0181), and of each test a leaf holds.
const (
	kindAttribute   = "attribute"
	kindBirthday    = "birthday"
	kindTag         = "tag"
	kindTelegram    = "telegram"
	kindSMS         = "sms"
	kindCustomField = "customField"
	kindEvent       = "event"
	kindProperty    = "property"

	testText       = "text"
	testNumber     = "number"
	testMoment     = "moment"
	testWithinLast = "withinLast"
	testPresence   = "presence"
)

// place is what the tree of one place admits: a Rule's tests the triggering Event's properties, a
// Segment's does not (ADR 0181).
type place struct{ properties bool }

var (
	segmentPlace = place{}
	rulePlace    = place{properties: true}
)

// group is a decoded group of the tree: its entries hold, all of them or any, and the result is
// negated when not is set.
type group struct {
	combinator conditiontree.Combinator
	not        bool
	entries    []node
}

// node is an entry of a group: a nested group or one of the leaves below.
type node interface{ isNode() }

// attributeLeaf tests a text attribute of the Contact (textFields) by a text or presence test.
type attributeLeaf struct {
	name conditiontree.Attribute
	test test
}

// birthdayLeaf places the Contact's date of birth by a moment test.
type birthdayLeaf struct{ test momentTest }

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
func (birthdayLeaf) isNode()    {}
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
// days up to now (opWithin, the `withinLast` test).
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

// decode reads a stored Condition of the place p into the typed tree, refusing what it cannot
// read with the path of the entry ("entry 0: entry 2: …") and what it cannot read there. It reads
// the tree alone: whether a named Custom field exists, and whether its test fits the field's type,
// are the compiler's checks.
func decode(g conditiontree.Group, p place) (group, error) {
	if g.Kind != conditiontree.KindGroup {
		return group{}, fmt.Errorf("unknown group kind %q", g.Kind)
	}
	if g.Combinator != conditiontree.And && g.Combinator != conditiontree.Or {
		return group{}, fmt.Errorf("unknown combinator %q", g.Combinator)
	}
	d := group{combinator: g.Combinator, not: g.Not, entries: make([]node, 0, len(g.Entries))}
	for i, raw := range g.Entries {
		n, err := decodeEntry(raw, p, func(sub conditiontree.Group) (node, error) { return decode(sub, p) })
		if err != nil {
			return group{}, fmt.Errorf("entry %d: %w", i, err)
		}
		d.entries = append(d.entries, n)
	}
	return d, nil
}

// decodeSkipping is the stored Condition as the readers of what it names see it: an entry decode
// refuses tests nothing and is left out, so one bad entry does not hide the others. It reads the
// leaves of every place.
func decodeSkipping(g conditiontree.Group) group {
	d := group{combinator: g.Combinator, not: g.Not, entries: make([]node, 0, len(g.Entries))}
	for _, raw := range g.Entries {
		n, err := decodeEntry(raw, rulePlace, func(sub conditiontree.Group) (node, error) { return decodeSkipping(sub), nil })
		if err != nil {
			continue
		}
		d.entries = append(d.entries, n)
	}
	return d
}

// entry is a stored entries[] item: a nested group or a leaf, as its `kind` says, with the fields
// of every kind of leaf.
type entry struct {
	Kind       string          `json:"kind"`
	Attribute  string          `json:"attribute"`
	Name       string          `json:"name"`
	Operator   string          `json:"operator"`
	Key        string          `json:"key"`
	Of         *source         `json:"of"`
	Test       json.RawMessage `json:"test"`
	Action     string          `json:"action"`
	Times      *int            `json:"times"`
	WithinDays *int            `json:"withinDays"`
}

// source is a property test's `of`: whose properties it reads, by its `kind`.
type source struct {
	Kind string `json:"kind"`
}

// storedTest is a leaf's stored `test`, with the fields of every kind of test. Value is a text
// test's string or a number test's number.
type storedTest struct {
	Kind     string          `json:"kind"`
	Operator string          `json:"operator"`
	Value    json.RawMessage `json:"value"`
	At       *time.Time      `json:"at"`
	Days     *int            `json:"days"`
}

// decodeEntry reads an entries[] item by its `kind`: a nested group, which sub reads, or a leaf
// the place p admits.
func decodeEntry(raw json.RawMessage, p place, sub func(conditiontree.Group) (node, error)) (node, error) {
	var e entry
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil, fmt.Errorf("malformed entry: %w", err)
	}
	switch e.Kind {
	case conditiontree.KindGroup:
		var g conditiontree.Group
		if err := json.Unmarshal(raw, &g); err != nil {
			return nil, fmt.Errorf("malformed group: %w", err)
		}
		return sub(g)
	case kindAttribute:
		name := conditiontree.Attribute(e.Attribute)
		if _, ok := textFields[name]; !ok {
			return nil, fmt.Errorf("unknown attribute %q", e.Attribute)
		}
		t, err := decodeTest(e.Test, testText, testPresence)
		if err != nil {
			return nil, err
		}
		return attributeLeaf{name: name, test: t}, nil
	case kindBirthday:
		t, err := decodeTest(e.Test, testMoment, testWithinLast)
		if err != nil {
			return nil, err
		}
		moment, _ := t.(momentTest)
		return birthdayLeaf{test: moment}, nil
	case kindTag:
		has, err := hasOrNot(e.Operator)
		if err != nil {
			return nil, err
		}
		name := strings.TrimSpace(e.Name)
		if name == "" {
			return nil, errors.New("tag name is required")
		}
		return tagLeaf{name: name, test: has}, nil
	case kindTelegram:
		has, err := hasOrNot(e.Operator)
		if err != nil {
			return nil, err
		}
		return telegramLeaf{test: has}, nil
	case kindSMS:
		has, err := hasOrNot(e.Operator)
		if err != nil {
			return nil, err
		}
		return smsLeaf{test: has}, nil
	case kindCustomField:
		if e.Key == "" {
			return nil, errors.New("custom field key is required")
		}
		t, err := decodeTest(e.Test, testText, testNumber, testMoment, testWithinLast, testPresence)
		if err != nil {
			return nil, err
		}
		return customFieldLeaf{key: e.Key, test: t}, nil
	case kindEvent:
		return decodeEvent(e)
	case kindProperty:
		if p.properties {
			return decodeProperty(e)
		}
	}
	return nil, fmt.Errorf("unknown entry kind %q", e.Kind)
}

// decodeProperty reads a property test of the triggering Event.
func decodeProperty(e entry) (node, error) {
	if e.Of == nil || subject(e.Of.Kind) != subjectTrigger {
		return nil, errors.New("a property test reads the triggering Event's properties")
	}
	if e.Key == "" {
		return nil, errors.New("property key is required")
	}
	t, err := decodeTest(e.Test, testText, testNumber, testPresence)
	if err != nil {
		return nil, err
	}
	return propertyLeaf{subject: subjectTrigger, key: e.Key, test: t}, nil
}

// hasOrNot is the presence test of a Tag, Telegram or SMS leaf: has or doesNotHave.
func hasOrNot(op string) (presenceTest, error) {
	switch operator(op) {
	case opHas:
		return presenceTest{set: true}, nil
	case opDoesNotHave:
		return presenceTest{set: false}, nil
	default:
		return presenceTest{}, fmt.Errorf("unknown operator %q", op)
	}
}

// decodeTest reads a leaf's test, refusing one whose `kind` is not among the kinds the leaf takes.
func decodeTest(raw json.RawMessage, kinds ...string) (test, error) {
	var t storedTest
	if err := json.Unmarshal(raw, &t); err != nil {
		return nil, fmt.Errorf("malformed test: %w", err)
	}
	known := false
	for _, k := range kinds {
		known = known || k == t.Kind
	}
	if !known {
		return nil, fmt.Errorf("unknown test kind %q", t.Kind)
	}
	op := operator(t.Operator)
	switch t.Kind {
	case testText:
		var value string
		if err := json.Unmarshal(t.Value, &value); err != nil {
			return nil, fmt.Errorf("text test value: %w", err)
		}
		if _, ok := textOperators[op]; !ok {
			return nil, fmt.Errorf("unknown text operator %q", t.Operator)
		}
		return textTest{op: op, value: value}, nil
	case testNumber:
		var value float64
		if err := json.Unmarshal(t.Value, &value); err != nil {
			return nil, fmt.Errorf("number test value: %w", err)
		}
		if _, ok := comparisons[op]; !ok {
			return nil, fmt.Errorf("unknown number operator %q", t.Operator)
		}
		return numberTest{op: op, value: value}, nil
	case testMoment:
		if _, ok := sides[op]; !ok {
			return nil, fmt.Errorf("unknown moment operator %q", t.Operator)
		}
		if t.At == nil {
			return nil, errors.New("moment test at is required")
		}
		return momentTest{op: op, at: *t.At}, nil
	case testWithinLast:
		if t.Days == nil {
			return nil, errors.New("withinLast test days is required")
		}
		if err := validation.Min(fieldname.Days, *t.Days, bounds.ConditionWithinDaysMin); err != nil {
			return nil, boundError{err}
		}
		return momentTest{op: opWithin, withinDays: *t.Days}, nil
	default:
		switch op {
		case opSet:
			return presenceTest{set: true}, nil
		case opNotSet:
			return presenceTest{set: false}, nil
		default:
			return nil, fmt.Errorf("unknown presence operator %q", t.Operator)
		}
	}
}

// decodeEvent reads an `event` leaf: a count of the Contact's Events of one action.
func decodeEvent(l entry) (node, error) {
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
