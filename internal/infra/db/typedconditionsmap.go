package db

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// This file is the typed-conditions migration's own frozen copy of both Condition shapes (ADR
// 0181): the stored shape before it, as the Condition module's decode read it then (ADR 0029,
// ADR 0054), and the typed tree the module reads since. It refuses what that decode refused from
// the tree alone, and never looks at a Workspace's Custom field catalogue: a leaf's kind and
// operator say which typed test it is. Neither shape may follow a later change of the module.

// conditionPlace is what the tree of one place admits: a Rule's tests the triggering Event's
// properties, a Segment's (and an armed Broadcast's copy of one) does not.
type conditionPlace struct{ properties bool }

var (
	segmentConditions = conditionPlace{}
	ruleConditions    = conditionPlace{properties: true}
)

// The stored shape before ADR 0181: a group's entries are its `rules`, a `rule` leaf names what it
// tests in `field` (a prefix carries a Custom field's or a triggering Event's key), with one
// operator enum, and a number, a moment and the last days are leaves of their own.
const (
	oldCustomFieldPrefix = "custom:"
	oldTriggerPrefix     = "trigger:"
	oldTagField          = "tag"
	oldTelegramField     = "telegram"
	oldSMSField          = "sms"
	oldBirthdayField     = "birthday"

	oldKindRule       = "rule"
	oldKindNumber     = "number"
	oldKindDate       = "date"
	oldKindDateWithin = "dateWithin"
	kindGroup         = "group"
	kindEvent         = "event"
)

// oldAttributes is the Contact's text attributes a `rule` leaf could test.
var oldAttributes = map[string]bool{
	"subject_id": true, "email": true, "phone": true, "first_name": true, "last_name": true,
	"time_zone": true, "country": true, "locale": true, "city": true,
}

// oldTextOperators is the operators of a `rule` leaf that compare text; null and notNull test
// presence.
var oldTextOperators = map[string]bool{"eq": true, "neq": true, "contains": true, "beginsWith": true}

// oldPresence is the presence test each presence operator of a `rule` leaf names.
var oldPresence = map[string]string{"null": "notSet", "notNull": "set"}

// oldComparisons, oldSides and oldEventLeast are the operators of a number, a date and an event
// leaf, with the least times an event operator takes.
var (
	oldComparisons = map[string]bool{"gt": true, "gte": true, "lt": true, "lte": true}
	oldSides       = map[string]bool{"before": true, "after": true}
	oldEventLeast  = map[string]int{"atLeast": 1, "atMost": 0}
)

// The least days a window takes and the least times an event leaf counts, as the bounds were.
const (
	oldWithinDaysMin = 1
	oldEventTimesMin = 0
)

// oldGroup is a stored group before ADR 0181.
type oldGroup struct {
	Kind       string            `json:"kind"`
	Combinator string            `json:"combinator"`
	Not        bool              `json:"not"`
	Rules      []json.RawMessage `json:"rules"`
}

// The stored leaves before ADR 0181, each read as the decode read it: a `rule` leaf's value is
// text ("" when it has none), a `number` leaf's a number and a `date` leaf's moment a time (the
// zero time when it has none). A number and a moment are kept as written.
type (
	oldRule struct {
		Field    string `json:"field"`
		Operator string `json:"operator"`
		Value    string `json:"value"`
	}
	oldNumber struct {
		Field    string          `json:"field"`
		Operator string          `json:"operator"`
		Value    json.RawMessage `json:"value"`
	}
	oldDate struct {
		Field    string          `json:"field"`
		Operator string          `json:"operator"`
		At       json.RawMessage `json:"at"`
	}
	oldDateWithin struct {
		Field      string `json:"field"`
		WithinDays int    `json:"withinDays"`
	}
	oldEvent struct {
		Action     string `json:"action"`
		Operator   string `json:"operator"`
		Times      *int   `json:"times"`
		WithinDays *int   `json:"withinDays"`
	}
)

// The typed tree (ADR 0181), as it is stored since.
type (
	typedGroup struct {
		Kind       string `json:"kind"`
		Combinator string `json:"combinator"`
		Not        bool   `json:"not,omitempty"`
		Entries    []any  `json:"entries"`
	}
	typedAttribute struct {
		Kind      string `json:"kind"`
		Attribute string `json:"attribute"`
		Test      any    `json:"test"`
	}
	typedBirthday struct {
		Kind string `json:"kind"`
		Test any    `json:"test"`
	}
	typedTag struct {
		Kind     string `json:"kind"`
		Name     string `json:"name"`
		Operator string `json:"operator"`
	}
	// typedReach is the Telegram and the SMS leaf.
	typedReach struct {
		Kind     string `json:"kind"`
		Operator string `json:"operator"`
	}
	typedCustomField struct {
		Kind string `json:"kind"`
		Key  string `json:"key"`
		Test any    `json:"test"`
	}
	typedSource struct {
		Kind string `json:"kind"`
	}
	typedProperty struct {
		Kind string      `json:"kind"`
		Of   typedSource `json:"of"`
		Key  string      `json:"key"`
		Test any         `json:"test"`
	}
	typedEvent struct {
		Kind       string `json:"kind"`
		Action     string `json:"action"`
		Operator   string `json:"operator"`
		Times      int    `json:"times"`
		WithinDays *int   `json:"withinDays,omitempty"`
	}
	typedText struct {
		Kind     string `json:"kind"`
		Operator string `json:"operator"`
		Value    string `json:"value"`
	}
	typedPresence struct {
		Kind     string `json:"kind"`
		Operator string `json:"operator"`
	}
	typedNumber struct {
		Kind     string          `json:"kind"`
		Operator string          `json:"operator"`
		Value    json.RawMessage `json:"value"`
	}
	typedMoment struct {
		Kind     string          `json:"kind"`
		Operator string          `json:"operator"`
		At       json.RawMessage `json:"at"`
	}
	typedWithinLast struct {
		Kind string `json:"kind"`
		Days int    `json:"days"`
	}
)

// typedCondition is the stored Condition doc of place p as the typed tree, or nil when it already
// is one (a group whose entries are `entries`, not `rules`), which a run interrupted after its
// rewrite leaves. It refuses a tree it cannot map, naming the entry ("entry 0: entry 2: …").
func typedCondition(doc []byte, p conditionPlace) ([]byte, error) {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(doc, &keys); err != nil {
		return nil, fmt.Errorf("malformed condition: %w", err)
	}
	_, old := keys["rules"]
	if _, typed := keys["entries"]; typed && !old {
		return nil, nil
	}
	var g oldGroup
	if err := json.Unmarshal(doc, &g); err != nil {
		return nil, fmt.Errorf("malformed group: %w", err)
	}
	typed, err := mapGroup(g, p)
	if err != nil {
		return nil, err
	}
	return json.Marshal(typed)
}

// mapGroup maps a group and its entries.
func mapGroup(g oldGroup, p conditionPlace) (typedGroup, error) {
	if g.Kind != kindGroup {
		return typedGroup{}, fmt.Errorf("unknown group kind %q", g.Kind)
	}
	if g.Combinator != "and" && g.Combinator != "or" {
		return typedGroup{}, fmt.Errorf("unknown combinator %q", g.Combinator)
	}
	out := typedGroup{Kind: kindGroup, Combinator: g.Combinator, Not: g.Not, Entries: make([]any, 0, len(g.Rules))}
	for i, raw := range g.Rules {
		entry, err := mapEntry(raw, p)
		if err != nil {
			return typedGroup{}, fmt.Errorf("entry %d: %w", i, err)
		}
		out.Entries = append(out.Entries, entry)
	}
	return out, nil
}

// mapEntry maps a rules[] entry by its `kind`: a nested group or a leaf.
func mapEntry(raw json.RawMessage, p conditionPlace) (any, error) {
	var probe struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, fmt.Errorf("malformed entry: %w", err)
	}
	switch probe.Kind {
	case kindGroup:
		var g oldGroup
		if err := json.Unmarshal(raw, &g); err != nil {
			return nil, fmt.Errorf("malformed group: %w", err)
		}
		return mapGroup(g, p)
	case oldKindRule:
		var l oldRule
		if err := json.Unmarshal(raw, &l); err != nil {
			return nil, fmt.Errorf("malformed rule: %w", err)
		}
		return mapRule(l, p)
	case oldKindNumber:
		var l oldNumber
		if err := json.Unmarshal(raw, &l); err != nil {
			return nil, fmt.Errorf("malformed number leaf: %w", err)
		}
		return mapNumber(l, p)
	case oldKindDate:
		var l oldDate
		if err := json.Unmarshal(raw, &l); err != nil {
			return nil, fmt.Errorf("malformed date leaf: %w", err)
		}
		return mapDate(l)
	case oldKindDateWithin:
		var l oldDateWithin
		if err := json.Unmarshal(raw, &l); err != nil {
			return nil, fmt.Errorf("malformed date leaf: %w", err)
		}
		return mapDateWithin(l)
	case kindEvent:
		var l oldEvent
		if err := json.Unmarshal(raw, &l); err != nil {
			return nil, fmt.Errorf("malformed event leaf: %w", err)
		}
		return mapEvent(l)
	default:
		return nil, fmt.Errorf("unknown entry kind %q", probe.Kind)
	}
}

// mapRule maps a `rule` leaf: the leaf of what its field names, with its operator moved to the
// typed test of its value.
func mapRule(l oldRule, p conditionPlace) (any, error) {
	switch l.Field {
	case oldTagField:
		if err := hasOrNot(l.Operator, "tag"); err != nil {
			return nil, err
		}
		if strings.TrimSpace(l.Value) == "" {
			return nil, errors.New("tag name is required")
		}
		return typedTag{Kind: "tag", Name: l.Value, Operator: l.Operator}, nil
	case oldTelegramField, oldSMSField:
		if err := hasOrNot(l.Operator, l.Field); err != nil {
			return nil, err
		}
		return typedReach{Kind: l.Field, Operator: l.Operator}, nil
	}
	test, ok := valueTest(l.Operator, l.Value)
	if !ok {
		return nil, fmt.Errorf("unsupported operator %q for field %q", l.Operator, l.Field)
	}
	switch {
	case strings.HasPrefix(l.Field, oldCustomFieldPrefix):
		key, err := keyOf(l.Field, oldCustomFieldPrefix)
		if err != nil {
			return nil, err
		}
		return typedCustomField{Kind: "customField", Key: key, Test: test}, nil
	case strings.HasPrefix(l.Field, oldTriggerPrefix):
		return propertyLeaf(l.Field, test, p)
	case oldAttributes[l.Field]:
		return typedAttribute{Kind: "attribute", Attribute: l.Field, Test: test}, nil
	default:
		return nil, fmt.Errorf("unknown field %q", l.Field)
	}
}

// hasOrNot refuses an operator of a Tag, Telegram or SMS leaf other than has and doesNotHave.
func hasOrNot(op, field string) error {
	if op != "has" && op != "doesNotHave" {
		return fmt.Errorf("unsupported operator %q for %s field", op, field)
	}
	return nil
}

// valueTest is the typed test a `rule` leaf's operator names on a value: a presence test for null
// and notNull, a text test for a text operator; false for any other.
func valueTest(op, value string) (any, bool) {
	if presence, ok := oldPresence[op]; ok {
		return typedPresence{Kind: "presence", Operator: presence}, true
	}
	if !oldTextOperators[op] {
		return nil, false
	}
	return typedText{Kind: "text", Operator: op, Value: value}, true
}

// keyOf is the key a prefixed field names, refused when there is none.
func keyOf(field, prefix string) (string, error) {
	key := strings.TrimPrefix(field, prefix)
	if key == "" {
		return "", fmt.Errorf("unknown field %q", prefix)
	}
	return key, nil
}

// propertyLeaf is the property test of the triggering Event a `trigger:<key>` field names, which
// only a Rule's tree admits.
func propertyLeaf(field string, test any, p conditionPlace) (any, error) {
	if !p.properties {
		return nil, fmt.Errorf("field %q: a property of the triggering Event is tested only in a Rule", field)
	}
	key, err := keyOf(field, oldTriggerPrefix)
	if err != nil {
		return nil, err
	}
	return typedProperty{Kind: "property", Of: typedSource{Kind: "trigger"}, Key: key, Test: test}, nil
}

// mapNumber maps a `number` leaf: the number test of a Custom field or of a property of the
// triggering Event.
func mapNumber(l oldNumber, p conditionPlace) (any, error) {
	if !oldComparisons[l.Operator] {
		return nil, fmt.Errorf("unsupported operator %q for number leaf", l.Operator)
	}
	var value *float64
	if len(l.Value) > 0 {
		if err := json.Unmarshal(l.Value, &value); err != nil {
			return nil, fmt.Errorf("malformed number leaf: %w", err)
		}
	}
	if value == nil {
		return nil, errors.New("number leaf value is required")
	}
	test := typedNumber{Kind: "number", Operator: l.Operator, Value: l.Value}
	switch {
	case strings.HasPrefix(l.Field, oldCustomFieldPrefix):
		key, err := keyOf(l.Field, oldCustomFieldPrefix)
		if err != nil {
			return nil, err
		}
		return typedCustomField{Kind: "customField", Key: key, Test: test}, nil
	case strings.HasPrefix(l.Field, oldTriggerPrefix):
		return propertyLeaf(l.Field, test, p)
	default:
		return nil, fmt.Errorf("field %q: a number leaf takes a custom field or a trigger property", l.Field)
	}
}

// zeroMoment is the moment a date leaf without one was read as.
var zeroMoment = json.RawMessage(`"0001-01-01T00:00:00Z"`)

// mapDate maps a `date` leaf: the moment test of the birthday or a Custom field.
func mapDate(l oldDate) (any, error) {
	if !oldSides[l.Operator] {
		return nil, fmt.Errorf("unsupported operator %q for date leaf", l.Operator)
	}
	at := zeroMoment
	var moment time.Time
	if len(l.At) > 0 && string(l.At) != "null" {
		if err := json.Unmarshal(l.At, &moment); err != nil {
			return nil, fmt.Errorf("malformed date leaf: %w", err)
		}
		at = l.At
	}
	return momentLeaf(l.Field, typedMoment{Kind: "moment", Operator: l.Operator, At: at})
}

// mapDateWithin maps a `dateWithin` leaf: the test of the last days of the birthday or a Custom
// field.
func mapDateWithin(l oldDateWithin) (any, error) {
	days := l.WithinDays
	if days < oldWithinDaysMin {
		return nil, fmt.Errorf("withinDays must be at least %d, got %d", oldWithinDaysMin, days)
	}
	return momentLeaf(l.Field, typedWithinLast{Kind: "withinLast", Days: days})
}

// momentLeaf is the leaf of a moment test on field: the birthday, or a Custom field. A triggering
// Event's property takes no moment test.
func momentLeaf(field string, test any) (any, error) {
	if field == oldBirthdayField {
		return typedBirthday{Kind: "birthday", Test: test}, nil
	}
	if !strings.HasPrefix(field, oldCustomFieldPrefix) {
		return nil, fmt.Errorf("field %q: a date leaf takes a custom field", field)
	}
	key, err := keyOf(field, oldCustomFieldPrefix)
	if err != nil {
		return nil, err
	}
	return typedCustomField{Kind: "customField", Key: key, Test: test}, nil
}

// mapEvent keeps an `event` leaf, the count of the Contact's Events of one action, as it is,
// refusing one the decode refused.
func mapEvent(l oldEvent) (any, error) {
	if strings.TrimSpace(l.Action) == "" {
		return nil, errors.New("event action is required")
	}
	least, ok := oldEventLeast[l.Operator]
	if !ok {
		return nil, fmt.Errorf("unsupported operator %q for event leaf", l.Operator)
	}
	if l.Times == nil {
		return nil, errors.New("event leaf times is required")
	}
	if *l.Times < max(least, oldEventTimesMin) {
		return nil, fmt.Errorf("%s takes times of %d or more, got %d", l.Operator, max(least, oldEventTimesMin), *l.Times)
	}
	if l.WithinDays != nil && *l.WithinDays < oldWithinDaysMin {
		return nil, fmt.Errorf("withinDays must be at least %d, got %d", oldWithinDaysMin, *l.WithinDays)
	}
	return typedEvent{Kind: kindEvent, Action: l.Action, Operator: l.Operator, Times: *l.Times, WithinDays: l.WithinDays}, nil
}
