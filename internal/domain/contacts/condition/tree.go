package condition

import (
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"slices"
	"strings"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/internal/kernel/bounds"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/fieldname"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// place is what the tree of one place admits (ADR 0181): the kinds of entry, the contract's union
// of the place's entries, so a Segment's tree takes no test of a Rule's triggering Event, and the
// sources its property tests read.
type place struct {
	kinds   []conditiontree.Kind
	sources []conditiontree.Source
}

var (
	segmentPlace = place{kinds: conditiontree.SegmentKinds}
	rulePlace    = place{kinds: conditiontree.RuleKinds, sources: conditiontree.RuleSources}
	branchPlace  = place{kinds: conditiontree.BranchKinds, sources: conditiontree.BranchSources}
	// everyPlace admits the leaves of every place: the readers of what a stored Condition names
	// (decodeSkipping, NamedSenders) read a tree whatever place holds it.
	everyPlace = place{
		kinds:   lo.Union(conditiontree.SegmentKinds, conditiontree.RuleKinds, conditiontree.BranchKinds),
		sources: lo.Union(conditiontree.RuleSources, conditiontree.BranchSources),
	}
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

// attributeLeaf tests a text attribute of the Contact, read by field, by a text or presence test.
type attributeLeaf struct {
	name  conditiontree.Attribute
	field textField
	test  test
}

// birthdayLeaf places the Contact's date of birth by a moment or a withinLast test.
type birthdayLeaf struct{ test test }

// tagLeaf tests whether the Contact has the Tag called name.
type tagLeaf struct {
	name string
	test presenceTest
}

// telegramLeaf tests whether the Contact can be reached on Telegram.
type telegramLeaf struct{ test presenceTest }

// smsLeaf tests whether the Contact can be texted.
type smsLeaf struct{ test presenceTest }

// customFieldLeaf tests the Custom field key by a text, presence, number, moment or withinLast
// test. Whether the test fits the field's type is data, checked when the leaf compiles (fits).
type customFieldLeaf struct {
	key  string
	test test
}

// eventCountLeaf counts the Contact's Events of action: at least or at most times of them (count),
// over all time or within the last withinDays days (nil for all time), of the Events whose
// properties pass filter (nil for every one), compiled to path (ADR 0153, ADR 0180).
type eventCountLeaf struct {
	action     string
	count      eventCount
	times      int
	withinDays *int
	filter     *conditiontree.EventFilter
	path       *Path
}

// propertyLeaf tests the property key of the triggering Event, the one source a Rule's property
// test reads, by a text, presence or number test.
type propertyLeaf struct {
	key  string
	test propertyTest
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

// test is what a leaf asks of the value it reads, typed by that value: text, presence, number,
// moment or the last days.
type test interface{ isTest() }

// textTest compares a text value with value by op, which means on a Contact attribute and on a
// property of the triggering Event what its textOperator says. A number or boolean Custom field's
// eq and neq are text tests too, compared with the stored value's text.
type textTest struct {
	op      conditiontree.TextOperator
	meaning textOperator
	value   string
}

// presenceTest asks whether a value is set (or a Tag, a Telegram Binding, an eligible phone had),
// or not.
type presenceTest struct{ set bool }

// numberTest compares a number with value by op, as its comparison says.
type numberTest struct {
	op      conditiontree.NumberOperator
	meaning comparison
	value   float64
}

// momentTest places a moment on the side op of at.
type momentTest struct {
	op   conditiontree.MomentOperator
	side side
	at   time.Time
}

// withinLastTest asks for a moment in the window that reaches days back from now.
type withinLastTest struct{ days int }

func (textTest) isTest()       {}
func (presenceTest) isTest()   {}
func (numberTest) isTest()     {}
func (momentTest) isTest()     {}
func (withinLastTest) isTest() {}

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
// read with the path of the entry ("entry 0: entry 2: …") and what it cannot read there. What a
// place, a leaf or a test admits is the contract's vocabulary (package conditiontree), and a group
// without its entries is refused, never read as the empty group that matches every Contact. It
// reads the tree alone: whether a named Custom field exists, and whether its test fits the field's
// type, are the compiler's checks.
func decode(g conditiontree.Group, p place) (group, error) {
	if err := checkGroup(g); err != nil {
		return group{}, err
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

// checkGroup refuses a group whose kind or combinator is unknown, and one without its entries,
// which is never read as the empty group that matches every Contact.
func checkGroup(g conditiontree.Group) error {
	if g.Kind != conditiontree.KindGroup {
		return fmt.Errorf("unknown group kind %q", g.Kind)
	}
	if !slices.Contains(conditiontree.Combinators, g.Combinator) {
		return fmt.Errorf("unknown combinator %q", g.Combinator)
	}
	if g.Entries == nil {
		return errors.New("group entries are required")
	}
	return nil
}

// decodeSkipping is the stored Condition as the readers of what it names see it: an entry decode
// refuses tests nothing and is left out, so one bad entry does not hide the others. It reads the
// leaves of every place.
func decodeSkipping(g conditiontree.Group) group {
	d := group{combinator: g.Combinator, not: g.Not, entries: make([]node, 0, len(g.Entries))}
	for _, raw := range g.Entries {
		n, err := decodeEntry(raw, everyPlace, func(sub conditiontree.Group) (node, error) { return decodeSkipping(sub), nil })
		if err != nil {
			continue
		}
		d.entries = append(d.entries, n)
	}
	return d
}

// entry is a stored entries[] item: a nested group or a leaf, as its `kind` says, with the fields
// of every kind of leaf. Operator is a Tag, Telegram or SMS leaf's HasOperator or an event leaf's
// EventCountOperator.
type entry struct {
	Kind       conditiontree.Kind            `json:"kind"`
	Attribute  conditiontree.Attribute       `json:"attribute"`
	Channel    conditiontree.TrackingChannel `json:"channel"`
	Name       string                        `json:"name"`
	Operator   string                        `json:"operator"`
	Key        string                        `json:"key"`
	Of         *source                       `json:"of"`
	Test       json.RawMessage               `json:"test"`
	Action     string                        `json:"action"`
	Times      *int                          `json:"times"`
	WithinDays *int                          `json:"withinDays"`
	Filter     *conditiontree.EventFilter    `json:"filter"`
}

// source is a property test's `of`: whose properties it reads, by its `kind`.
type source struct {
	Kind conditiontree.Source `json:"kind"`
}

// storedTest is a leaf's stored `test`, with the fields of every kind of test. Value is a text
// test's string or a number test's number; Operator is the operator of the test's kind.
type storedTest struct {
	Kind     conditiontree.TestKind `json:"kind"`
	Operator string                 `json:"operator"`
	Value    json.RawMessage        `json:"value"`
	At       *time.Time             `json:"at"`
	Days     *int                   `json:"days"`
}

// decodeEntry reads an entries[] item by its `kind`: a nested group, which sub reads, or a leaf
// the place p admits.
func decodeEntry(raw json.RawMessage, p place, sub func(conditiontree.Group) (node, error)) (node, error) {
	var e entry
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil, fmt.Errorf("malformed entry: %w", err)
	}
	if !slices.Contains(p.kinds, e.Kind) {
		return nil, fmt.Errorf("unknown entry kind %q", e.Kind)
	}
	switch e.Kind {
	case conditiontree.KindGroup:
		var g conditiontree.Group
		if err := json.Unmarshal(raw, &g); err != nil {
			return nil, fmt.Errorf("malformed group: %w", err)
		}
		return sub(g)
	case conditiontree.KindAttribute:
		field, err := textFieldOf(e.Attribute)
		if err != nil {
			return nil, err
		}
		t, err := decodeTest(e.Test, conditiontree.AttributeTests)
		if err != nil {
			return nil, err
		}
		return attributeLeaf{name: e.Attribute, field: field, test: t}, nil
	case conditiontree.KindBirthday:
		t, err := decodeTest(e.Test, conditiontree.BirthdayTests)
		if err != nil {
			return nil, err
		}
		return birthdayLeaf{test: t}, nil
	case conditiontree.KindTag:
		has, err := hasOrNot(e.Operator)
		if err != nil {
			return nil, err
		}
		name := strings.TrimSpace(e.Name)
		if name == "" {
			return nil, errors.New("tag name is required")
		}
		return tagLeaf{name: name, test: has}, nil
	case conditiontree.KindTelegram:
		has, err := hasOrNot(e.Operator)
		if err != nil {
			return nil, err
		}
		return telegramLeaf{test: has}, nil
	case conditiontree.KindSms:
		has, err := hasOrNot(e.Operator)
		if err != nil {
			return nil, err
		}
		return smsLeaf{test: has}, nil
	case conditiontree.KindEngagementTracking:
		return decodeEngagementTracking(e)
	case conditiontree.KindCustomField:
		if e.Key == "" {
			return nil, errors.New("custom field key is required")
		}
		t, err := decodeTest(e.Test, conditiontree.CustomFieldTests)
		if err != nil {
			return nil, err
		}
		return customFieldLeaf{key: e.Key, test: t}, nil
	case conditiontree.KindEvent:
		return decodeEvent(e)
	case conditiontree.KindProperty:
		return decodeProperty(e, p.sources)
	case conditiontree.KindSentBy:
		// A sent-by test is an Event filter's alone (decodeFilterEntry): no place's tree admits it.
	}
	return nil, fmt.Errorf("unknown entry kind %q", e.Kind)
}

// decodeProperty reads a property test of one of the sources its place's property tests read.
func decodeProperty(e entry, sources []conditiontree.Source) (node, error) {
	if e.Of == nil || !slices.Contains(sources, e.Of.Kind) {
		return nil, errors.New("a property test reads the triggering Event's properties")
	}
	key, t, err := decodePropertyTest(e)
	if err != nil {
		return nil, err
	}
	return propertyLeaf{key: key, test: t}, nil
}

// CheckPropertyKey checks the form of the key of an Event property: property keys have no
// catalogue, so a key is checked for form only (ADR 0133), here, for a filter's tests and for an
// Automation's Enrollment key (ADR 0165) alike.
func CheckPropertyKey(key string) error {
	if key == "" {
		return errors.New("property key is required")
	}
	return nil
}

// decodePropertyTest reads the key and the test of a property test, whatever its subject.
func decodePropertyTest(e entry) (string, propertyTest, error) {
	if err := CheckPropertyKey(e.Key); err != nil {
		return "", nil, err
	}
	t, err := decodeTest(e.Test, conditiontree.PropertyTests)
	if err != nil {
		return "", nil, err
	}
	pt, ok := t.(propertyTest)
	if !ok {
		return "", nil, fmt.Errorf("unsupported test %T for a property", t)
	}
	return e.Key, pt, nil
}

// hasOrNot is the presence test of a Tag, Telegram or SMS leaf: has or doesNotHave.
func hasOrNot(op string) (presenceTest, error) {
	switch conditiontree.HasOperator(op) {
	case conditiontree.Has:
		return presenceTest{set: true}, nil
	case conditiontree.DoesNotHave:
		return presenceTest{set: false}, nil
	}
	return presenceTest{}, fmt.Errorf("unknown operator %q", op)
}

// presenceOf is the presence test of a value: set or notSet.
func presenceOf(op string) (presenceTest, error) {
	switch conditiontree.PresenceOperator(op) {
	case conditiontree.PresenceSet:
		return presenceTest{set: true}, nil
	case conditiontree.PresenceNotSet:
		return presenceTest{set: false}, nil
	}
	return presenceTest{}, fmt.Errorf("unknown presence operator %q", op)
}

// decodeTest reads a leaf's test, refusing one whose `kind` is not among the kinds the leaf takes.
func decodeTest(raw json.RawMessage, kinds []conditiontree.TestKind) (test, error) {
	var t storedTest
	if err := json.Unmarshal(raw, &t); err != nil {
		return nil, fmt.Errorf("malformed test: %w", err)
	}
	if !slices.Contains(kinds, t.Kind) {
		return nil, fmt.Errorf("unknown test kind %q", t.Kind)
	}
	switch t.Kind {
	case conditiontree.TestText:
		var value string
		if err := json.Unmarshal(t.Value, &value); err != nil {
			return nil, fmt.Errorf("text test value: %w", err)
		}
		op := conditiontree.TextOperator(t.Operator)
		meaning, err := textOperatorOf(op)
		if err != nil {
			return nil, err
		}
		return textTest{op: op, meaning: meaning, value: value}, nil
	case conditiontree.TestNumber:
		var value float64
		if err := json.Unmarshal(t.Value, &value); err != nil {
			return nil, fmt.Errorf("number test value: %w", err)
		}
		op := conditiontree.NumberOperator(t.Operator)
		meaning, err := comparisonOf(op)
		if err != nil {
			return nil, err
		}
		return numberTest{op: op, meaning: meaning, value: value}, nil
	case conditiontree.TestMoment:
		op := conditiontree.MomentOperator(t.Operator)
		s, err := sideOf(op)
		if err != nil {
			return nil, err
		}
		if t.At == nil {
			return nil, errors.New("moment test at is required")
		}
		return momentTest{op: op, side: s, at: *t.At}, nil
	case conditiontree.TestWithinLast:
		if t.Days == nil {
			return nil, errors.New("withinLast test days is required")
		}
		if err := validation.Min(fieldname.Days, *t.Days, bounds.ConditionWithinDaysMin); err != nil {
			return nil, boundError{err}
		}
		return withinLastTest{days: *t.Days}, nil
	case conditiontree.TestPresence:
		return presenceOf(t.Operator)
	}
	return nil, fmt.Errorf("unknown test kind %q", t.Kind)
}

// decodeEvent reads an `event` leaf: a count of the Contact's Events of one action.
func decodeEvent(l entry) (node, error) {
	action := strings.TrimSpace(l.Action)
	if action == "" {
		return nil, errors.New("event action is required")
	}
	count, err := eventCountOf(conditiontree.EventCountOperator(l.Operator))
	if err != nil {
		return nil, err
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
	leaf := eventCountLeaf{action: action, count: count, times: times, withinDays: l.WithinDays}
	if l.Filter != nil {
		path, err := CompileEventFilter(action, *l.Filter)
		if err != nil {
			return nil, fmt.Errorf("filter: %w", err)
		}
		leaf.filter, leaf.path = l.Filter, &path
	}
	return leaf, nil
}
