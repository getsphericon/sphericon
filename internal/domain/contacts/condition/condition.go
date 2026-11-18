// Package condition is the Condition: the typed tree of tests on a Contact (the API
// contract's tree, ADR 0029, ADR 0054), its validation and its compiler to a Contact
// predicate. A Segment is a named Condition and a Rule holds one. A stored Condition is read once
// into the typed tree (decode, tree.go): groups with a combinator, a negation and their entries,
// whose leaves test a Contact attribute, a Tag, Telegram and SMS reachability, a Custom field, a
// count of the Contact's Events or a property of the triggering Event, each by a test typed by
// the value it reads (text, presence, number or moment). Everything else here works on that tree.
//
// A leaf is built from ent's generated predicates wherever one exists: the core
// attributes (contact.EmailEQ …) and Tags (contact.HasTagsWith). An event count is a correlated
// COUNT(*) of the Contact's Events, built with ent's SQL builder and the generated Event
// predicates. The one dynamic piece is a Custom field's key inside the
// custom_fields JSON column, which no generated predicate covers; it is addressed
// with ent's JSON predicates (sqljson) after the key is checked, and by a number or moment test
// with a jsonpath filter, which is false on a value of another type where a cast would fail the
// query. A leaf may only name a core attribute, a Tag, an Event action, or a Custom field of the
// Workspace's catalogue (ADR 0006), and a number or moment test only a Custom field of its type.
package condition

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/tagname"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/tag"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/reportday"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

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
	"country":    {contact.CountryEQ, contact.CountryNEQ, contact.CountryContainsFold, contact.CountryHasPrefix, contact.CountryIsNil(), contact.CountryNotNil()},
	"locale":     {contact.LocaleEQ, contact.LocaleNEQ, contact.LocaleContainsFold, contact.LocaleHasPrefix, contact.LocaleIsNil(), contact.LocaleNotNil()},
	"city":       {contact.CityEQ, contact.CityNEQ, contact.CityContainsFold, contact.CityHasPrefix, contact.CityIsNil(), contact.CityNotNil()},
}

// birthdaySides is the predicate of each side of a date leaf on the birthday.
var birthdaySides = map[operator]func(time.Time) predicate.Contact{
	opBefore: contact.BirthdayLT,
	opAfter:  contact.BirthdayGTE,
}

// boundError is a leaf's refusal of a bound of the contract (package bounds), whose text is the
// refusal's message: a Segment or a Rule reports the compiler's text as the detail of its own
// refusal, which a person reads.
type boundError struct{ refusal error }

func (e boundError) Error() string { return validation.Message(e.refusal) }
func (e boundError) Unwrap() error { return e.refusal }

// Module compiles Conditions to Contact predicates. now is the Instance clock a relative date (in
// the last N days) counts back from.
type Module struct {
	now func() time.Time
}

// New builds the module over the Instance clock.
func New(now func() time.Time) *Module { return &Module{now: now} }

// Compile turns a condition group into a Contact predicate. An empty rule set
// matches everyone (an empty negated group matches no one) — deliberate, so an
// empty rule is never a silent "nobody". A test of the triggering Event is refused: only a Rule
// has an Event. A relative date (in the last N days) counts back from the Instance clock's time.
func (m *Module) Compile(ctx context.Context, s *ent.Scoped, g conditiontree.Group) (predicate.Contact, error) {
	tree, err := decode(g)
	if err != nil {
		return nil, err
	}
	return (&compiler{s: s, now: m.now()}).group(ctx, tree)
}

// RulePredicate compiles a Rule's Condition for the Workspace of s, with tests of the triggering
// Event's properties allowed. They are evaluated in memory against props, the properties of
// the Event that fired the Rule, and folded into the tree as constants before the rest compiles
// to a predicate (the Event's row may not exist yet). A key the Event lacks, or a value of
// another type, makes the leaf false. Any key is valid, so props may be nil to only validate.
func (m *Module) RulePredicate(ctx context.Context, s *ent.Scoped, g conditiontree.Group, props map[string]any) (predicate.Contact, error) {
	tree, err := decode(g)
	if err != nil {
		return nil, err
	}
	return (&compiler{s: s, now: m.now(), trigger: &props}).group(ctx, tree)
}

// compiler holds what one compilation needs: the Workspace's Custom field keys are
// loaded the first time a rule names one.
type compiler struct {
	s    *ent.Scoped
	now  time.Time
	keys map[string]customfield.Type
	// loc is the Workspace time zone, loaded by the first moment test that needs it.
	loc *time.Location
	// trigger is the properties of the Event a Rule's tests of the triggering Event read; nil in a
	// Segment, where such a leaf is refused.
	trigger *map[string]any
}

func (c *compiler) group(ctx context.Context, g group) (predicate.Contact, error) {
	preds := make([]predicate.Contact, 0, len(g.entries))
	for i, n := range g.entries {
		p, err := c.node(ctx, n)
		if err != nil {
			return nil, fmt.Errorf("rule %d: %w", i, err)
		}
		preds = append(preds, p)
	}

	if len(preds) == 0 {
		if g.not {
			return predicate.Contact(func(s *sql.Selector) { s.Where(sql.False()) }), nil
		}
		return contact.And(), nil
	}

	p := contact.And(preds...)
	if g.combinator == conditiontree.Or {
		p = contact.Or(preds...)
	}
	if g.not {
		p = contact.Not(p)
	}
	return p, nil
}

// node compiles one entry of a group: a nested group or a leaf.
func (c *compiler) node(ctx context.Context, n node) (predicate.Contact, error) {
	switch l := n.(type) {
	case group:
		return c.group(ctx, l)
	case attributeLeaf:
		return c.attribute(l)
	case tagLeaf:
		return tagRule(l), nil
	case telegramLeaf:
		// The subject has a Binding on a Telegram Integration of the Workspace. A Visitor's
		// Binding is no Contact's (ADR 0171). Evaluated live; a Binding exists only for a Telegram
		// Integration and goes with it, so the existence of one is the test.
		return reachable(contact.HasBindings(), l.test), nil
	case smsLeaf:
		return reachable(textable(), l.test), nil
	case customFieldLeaf:
		return c.custom(ctx, l)
	case eventCountLeaf:
		return c.event(l), nil
	case propertyLeaf:
		return c.property(l)
	default:
		return nil, fmt.Errorf("unknown leaf %T", n)
	}
}

// attribute tests a core attribute: a text attribute by a text or presence test, the birthday by
// a moment test.
func (c *compiler) attribute(l attributeLeaf) (predicate.Contact, error) {
	if l.name == birthdayField {
		t, ok := l.test.(momentTest)
		if !ok {
			return nil, fmt.Errorf("unsupported test %T for field %q", l.test, l.name)
		}
		return c.birthday(t), nil
	}
	f, ok := textFields[l.name]
	if !ok {
		return nil, fmt.Errorf("unknown field %q", l.name)
	}
	switch t := l.test.(type) {
	case textTest:
		return textTests[t.op].contact(f, t.value), nil
	case presenceTest:
		return t.contact(f), nil
	default:
		return nil, fmt.Errorf("unsupported test %T for field %q", l.test, l.name)
	}
}

// birthday places the Contact's date of birth: before or after a moment, or within the last days
// up to now.
func (c *compiler) birthday(t momentTest) predicate.Contact {
	if t.op == opWithin {
		since := c.since(t.withinDays)
		return contact.And(contact.BirthdayGTE(since.UTC()), contact.BirthdayLTE(c.now.UTC()))
	}
	return birthdaySides[t.op](t.at.UTC())
}

// since is the start of a window of the last days up to now. A window reaching back past the first
// year starts there.
func (c *compiler) since(days int) time.Time {
	since := c.now.AddDate(0, 0, -days)
	if since.Year() < 1 {
		since = time.Date(1, time.January, 1, 0, 0, 0, 0, time.UTC)
	}
	return since
}

// everyone and nobody are the predicates every Contact and no Contact satisfies. everyone is an
// explicit TRUE, not an empty contact.And(), which writes nothing and so is lost under a NOT.
var (
	everyone = predicate.Contact(func(s *sql.Selector) { s.Where(sql.Not(sql.False())) })
	nobody   = predicate.Contact(func(s *sql.Selector) { s.Where(sql.False()) })
)

// property is a test of one property of the triggering Event, decided here, in memory, and made
// the constant predicate "everyone" or "no one" for the rest of the tree. A key the Event lacks
// fails the test.
func (c *compiler) property(l propertyLeaf) (predicate.Contact, error) {
	if c.trigger == nil {
		return nil, fmt.Errorf("property %q of the triggering Event is tested only in a Rule", l.key)
	}
	var holds func(have any) bool
	switch t := l.test.(type) {
	case textTest:
		holds = func(have any) bool { return textTests[t.op].event(have, t.value) }
	case presenceTest:
		holds = t.event
	case numberTest:
		holds = func(have any) bool {
			n, ok := number(have)
			return ok && comparisons[t.op].holds(n, t.value)
		}
	default:
		return nil, fmt.Errorf("unsupported test %T for property %q", l.test, l.key)
	}
	if have, present := (*c.trigger)[l.key]; present && holds(have) {
		return everyone, nil
	}
	return nobody, nil
}

// location is the Workspace time zone, in which a datetime Custom field's value without an offset
// is read (ADR 0087); it is loaded the first time a moment test needs it.
func (c *compiler) location(ctx context.Context) (*time.Location, error) {
	if c.loc != nil {
		return c.loc, nil
	}
	ws, err := c.s.Workspace(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: load workspace: %w", ErrStorage, err)
	}
	loc, err := reportday.Location(ws.Timezone)
	if err != nil {
		return nil, fmt.Errorf("workspace time zone: %w", err)
	}
	c.loc = loc
	return loc, nil
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

// event is the subject performed `action` at least (or at most) times times, over all time or
// within the last withinDays days. Events attach to a Contact by the stable contact_id, so
// anonymous events stitched onto it at Identify are counted here (ADR 0002), and the count is
// correlated on it, so an anonymous Event (a NULL contact_id) is in no Contact's count.
func (c *compiler) event(l eventCountLeaf) predicate.Contact {
	preds := []predicate.Event{event.Action(l.action)}
	if events.IsEngagement(l.action) {
		// A machine open or click is in the log and in no count (ADR 0053).
		preds = append(preds, events.HumanOnly())
	}
	if l.withinDays != nil {
		// A window reaching back past the first year counts all time: no Event is older, and a
		// cutoff that far back would overflow a stored timestamp. A window longer than Event
		// retention is capped by it (ADR 0019).
		if since := c.now.AddDate(0, 0, -*l.withinDays); since.Year() >= 1 {
			preds = append(preds, event.OccurredAtGTE(since))
		}
	}

	holds := eventCounts[l.op].holds
	return predicate.Contact(func(s *sql.Selector) {
		t := sql.Table(event.Table)
		count := sql.Dialect(s.Dialect()).Select(sql.Count("*")).From(t)
		count.Where(sql.ColumnsEQ(count.C(event.FieldContactID), s.C(contact.FieldID)))
		for _, p := range preds {
			p(count)
		}
		s.Where(holds(count, l.times))
	})
}

// tagRule is the subject has (or lacks) the Tag. Evaluated live — membership is never
// materialized.
func tagRule(l tagLeaf) predicate.Contact {
	return reachable(contact.HasTagsWith(tag.NameKey(tagname.Key(l.name))), l.test)
}

// reachable is p when the presence test asks for it to be had, its negation otherwise.
func reachable(p predicate.Contact, t presenceTest) predicate.Contact {
	if t.set {
		return p
	}
	return contact.Not(p)
}

// textable is the subject can be texted: its phone is a Destination Send-eligibility on sms lets a
// marketing text reach, that is, one with an SMS Confirmation and no SMS opt-out or Suppression
// (ADR 0077). A phone alone is "phone is set"; on Telegram the Binding is itself the opt-in, on
// sms the Confirmation is, so the counterpart of the Telegram condition is the eligible phone.
// Evaluated live through the audience predicate, the rule's single definition. Every SMS opt-out
// is of everything, so the Sending source named here changes nothing.
func textable() predicate.Contact {
	return eligibility.Audience(eligibility.PhoneDestination(), eligibility.SourceBroadcasts)
}

// custom is a test of one Custom field. The key must be in the Workspace's catalogue (ADR 0006)
// and the test must fit its type (fits). A text or presence test is addressed with ent's JSON
// predicates (sqljson), since no generated predicate addresses a key of the custom_fields JSON
// column; a number or moment test with a jsonpath filter.
func (c *compiler) custom(ctx context.Context, l customFieldLeaf) (predicate.Contact, error) {
	t, err := c.customFieldType(ctx, l.key)
	if err != nil {
		return nil, err
	}
	if err := l.fits(t); err != nil {
		return nil, err
	}
	switch test := l.test.(type) {
	case numberTest:
		return jsonPath(l.key, asIs, comparisons[test.op].filter, map[string]any{"v": test.value})
	case momentTest:
		return c.customMoment(ctx, l.key, test)
	default:
		return customJSON(l.key, l.test)
	}
}

// fits refuses the leaf's test on a Custom field of type t: a text or presence test whose operator
// t does not take (the one table, customFieldOperators, which the API lists as each Custom field's
// ruleOperators), a number test on a field that is not a number, a moment test on one that is not
// a datetime.
func (l customFieldLeaf) fits(t customfield.Type) error {
	var want customfield.Type
	switch test := l.test.(type) {
	case numberTest:
		want = customfield.TypeNumber
	case momentTest:
		want = customfield.TypeDatetime
	case textTest:
		return fitsOperator(test.op, l.key, t)
	case presenceTest:
		return fitsOperator(test.operator(), l.key, t)
	default:
		return fmt.Errorf("unsupported test %T for custom field %q", l.test, l.key)
	}
	if t != want {
		return fmt.Errorf("custom field %q is of type %s, not %s", l.key, t, want)
	}
	return nil
}

// fitsOperator refuses op on a Custom field of type t unless t takes it.
func fitsOperator(op operator, key string, t customfield.Type) error {
	if !slices.Contains(customFieldOperators[t], op) {
		return fmt.Errorf("unsupported operator %q for json field %q of type %s", op, key, t)
	}
	return nil
}

// customJSON is a text or presence test of a Custom field, addressed with ent's JSON predicates.
func customJSON(key string, t test) (predicate.Contact, error) {
	// ent inlines a JSON path key into the statement as a quoted literal instead of binding
	// it: quotes are doubled so any key stays one literal (standard_conforming_strings
	// makes a backslash plain), and a key ent would read as an array index is refused.
	if isIndexKey(key) {
		return nil, fmt.Errorf("unsupported custom field key %q", key)
	}
	path := sqljson.Path(strings.ReplaceAll(key, "'", "''"))
	var cond func(column string) *sql.Predicate
	switch t := t.(type) {
	case presenceTest:
		cond = func(column string) *sql.Predicate { return sqljson.HasKey(column, path) }
	case textTest:
		switch t.op {
		case opEQ:
			cond = func(column string) *sql.Predicate { return sqljson.ValueEQ(column, t.value, path) }
		case opNEQ:
			cond = func(column string) *sql.Predicate { return sqljson.ValueNEQ(column, t.value, path) }
		case opContains:
			// Membership of a JSON array (jsonb containment of the encoded value). The value
			// is read as JSON, not unquoted to text, so a string element matches as a string.
			cond = func(column string) *sql.Predicate {
				return sqljson.ValueContains(column, t.value, path, sqljson.Unquote(false))
			}
		default:
			// beginsWith fits no Custom field type (fits), so it never reaches here.
		}
	}
	if cond == nil {
		return nil, fmt.Errorf("unsupported test %T for json field %q", t, key)
	}
	return predicate.Contact(func(s *sql.Selector) {
		s.Where(cond(s.C(contact.FieldCustomFields)))
	}), nil
}

// customMoment places a datetime Custom field's moment: which side of a bound it is on, or within
// the last days up to now, so a moment still to come is not within.
func (c *compiler) customMoment(ctx context.Context, key string, t momentTest) (predicate.Contact, error) {
	loc, err := c.location(ctx)
	if err != nil {
		return nil, err
	}
	if t.op == opWithin {
		since := c.since(t.withinDays)
		return jsonPath(key, asMoment, window, map[string]any{
			"since": since.UTC().Format(momentLayout), "now": c.now.UTC().Format(momentLayout),
			"sinceLocal": since.In(loc).Format(localLayout), "nowLocal": c.now.In(loc).Format(localLayout),
		})
	}
	return jsonPath(key, asMoment, sides[t.op], map[string]any{
		"at": t.at.UTC().Format(momentLayout), "atLocal": t.at.In(loc).Format(localLayout),
	})
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

// FromDTO is a Condition given as an API DTO: the API's typed tree is the stored shape (ADR 0169).
func FromDTO(group json.Marshaler) (conditiontree.Group, error) {
	var g conditiontree.Group
	b, err := group.MarshalJSON()
	if err == nil {
		err = json.Unmarshal(b, &g)
	}
	if err != nil {
		return g, fmt.Errorf("encode condition: %w", err)
	}
	return g, nil
}

// FromOptionalDTO is FromDTO for a condition the request may leave out: nil when it is absent.
func FromOptionalDTO[G any, P interface {
	*G
	json.Marshaler
}](opt interface{ Get() (G, bool) }) (*conditiontree.Group, error) {
	group, ok := opt.Get()
	if !ok {
		return nil, nil
	}
	g, err := FromDTO(P(&group))
	if err != nil {
		return nil, err
	}
	return &g, nil
}

// ToDTO loads a Condition into an API DTO.
func ToDTO(g conditiontree.Group, into json.Unmarshaler) error {
	b, err := json.Marshal(g)
	if err == nil {
		err = into.UnmarshalJSON(b)
	}
	if err != nil {
		return fmt.Errorf("decode condition: %w", err)
	}
	return nil
}

// FitsCustomFieldType reports whether every leaf of the group, at any depth, that tests the Custom
// field key would still compile were the field of type t (customFieldLeaf.fits). A malformed entry
// tests nothing.
func FitsCustomFieldType(g conditiontree.Group, key string, t customfield.Type) bool {
	for n := range readable(g).leaves() {
		if l, ok := n.(customFieldLeaf); ok && l.key == key && l.fits(t) != nil {
			return false
		}
	}
	return true
}

// NamesTag reports whether any leaf of the group, at any depth, tests the Tag called name.
// A malformed entry names nothing.
func NamesTag(g conditiontree.Group, name string) bool {
	for n := range readable(g).leaves() {
		if l, ok := n.(tagLeaf); ok && tagname.Same(l.name, name) {
			return true
		}
	}
	return false
}

// CanBeUnknown reports whether the group can be unknown for a Contact (ADR 0181): it can when a
// leaf, at any depth and under a negation too, counts an open or a click, whose tracking a Contact
// may have refused. A malformed entry counts nothing.
func CanBeUnknown(g conditiontree.Group) bool {
	for n := range readable(g).leaves() {
		if l, ok := n.(eventCountLeaf); ok && events.IsEngagement(l.action) {
			return true
		}
	}
	return false
}
