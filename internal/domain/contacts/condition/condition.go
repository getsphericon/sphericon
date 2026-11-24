// Package condition is the Condition: the typed tree of tests on a Contact (the API
// contract's tree, ADR 0029, ADR 0054, ADR 0181), its validation and its compiler to a Contact
// predicate. A Segment is a named Condition and a Rule holds one; each place's tree admits its own
// leaves. A stored Condition is read once into the typed tree (decode, tree.go): groups with a
// combinator, a negation and their entries, whose leaves test a Contact attribute, the birthday, a
// Tag, Telegram and SMS reachability, an Engagement tracking consent, a Custom field, a count of the
// Contact's Events or, in a Rule, a property of the triggering Event, each by a test typed by the
// value it reads (text, presence, number, moment or the last days). An Event matcher's filter is a tree of property tests
// alone, compiled to one SQL/JSON path (filter.go, ADR 0180), and a Rule's property tests run as the
// same path on the triggering Event's properties: no property test is evaluated in Go. What each
// place, leaf and test admits, and every operator, is the contract's vocabulary generated into
// package conditiontree (the filter's kinds are written there until the contract carries a
// filter); what an operator means is a switch over its values here. Everything else here works on
// that tree.
//
// A leaf is built from ent's generated predicates wherever one exists: the core
// attributes (contact.EmailEQ …) and Tags (contact.HasTagsWith). An event count is a correlated
// COUNT(*) of the Contact's Events, built with ent's SQL builder and the generated Event
// predicates. A count of opens or clicks is the one leaf that can be unknown (ADR 0123): NULL for
// a Contact whose Destination on the Channel of the count is not tracked now, so Postgres carries it
// through AND, OR, NOT and WHERE by Kleene's rules and a Segment or a Rule reads it as no match;
// every other leaf is meant two-valued. The one dynamic piece is a Custom field's key inside the
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
	"github.com/getsphericon/sphericon/ent/binding"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/tag"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/reportday"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// textField is the generated predicates of one core text attribute.
type textField struct {
	eq, neq, contains, beginsWith func(string) predicate.Contact
	null, notNull                 predicate.Contact
}

// textFieldOf is the generated predicates of the core text attribute a, or a refusal of an
// attribute outside the contract.
func textFieldOf(a conditiontree.Attribute) (textField, error) {
	switch a {
	case conditiontree.AttributeSubjectId:
		return textField{contact.SubjectIDEQ, contact.SubjectIDNEQ, contact.SubjectIDContainsFold, contact.SubjectIDHasPrefix, contact.SubjectIDIsNil(), contact.SubjectIDNotNil()}, nil
	case conditiontree.AttributeEmail:
		return textField{contact.EmailEQ, contact.EmailNEQ, contact.EmailContainsFold, contact.EmailHasPrefix, contact.EmailIsNil(), contact.EmailNotNil()}, nil
	case conditiontree.AttributePhone:
		return textField{contact.PhoneEQ, contact.PhoneNEQ, contact.PhoneContainsFold, contact.PhoneHasPrefix, contact.PhoneIsNil(), contact.PhoneNotNil()}, nil
	case conditiontree.AttributeFirstName:
		return textField{contact.FirstNameEQ, contact.FirstNameNEQ, contact.FirstNameContainsFold, contact.FirstNameHasPrefix, contact.FirstNameIsNil(), contact.FirstNameNotNil()}, nil
	case conditiontree.AttributeLastName:
		return textField{contact.LastNameEQ, contact.LastNameNEQ, contact.LastNameContainsFold, contact.LastNameHasPrefix, contact.LastNameIsNil(), contact.LastNameNotNil()}, nil
	case conditiontree.AttributeTimeZone:
		return textField{contact.TimeZoneEQ, contact.TimeZoneNEQ, contact.TimeZoneContainsFold, contact.TimeZoneHasPrefix, contact.TimeZoneIsNil(), contact.TimeZoneNotNil()}, nil
	case conditiontree.AttributeCountry:
		return textField{contact.CountryEQ, contact.CountryNEQ, contact.CountryContainsFold, contact.CountryHasPrefix, contact.CountryIsNil(), contact.CountryNotNil()}, nil
	case conditiontree.AttributeLocale:
		return textField{contact.LocaleEQ, contact.LocaleNEQ, contact.LocaleContainsFold, contact.LocaleHasPrefix, contact.LocaleIsNil(), contact.LocaleNotNil()}, nil
	case conditiontree.AttributeCity:
		return textField{contact.CityEQ, contact.CityNEQ, contact.CityContainsFold, contact.CityHasPrefix, contact.CityIsNil(), contact.CityNotNil()}, nil
	}
	return textField{}, fmt.Errorf("unknown attribute %q", a)
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

// Compile turns a Segment's Condition into a Contact predicate, unknown (SQL NULL) for a Contact
// an engagement count is unknown for where the rest of the tree does not decide (ADR 0123). An
// empty group matches everyone (an empty negated group matches no one) — deliberate, so an empty
// Condition is never a silent "nobody". A relative date (in the last N days) counts back from the
// Instance clock's time.
func (m *Module) Compile(ctx context.Context, s *ent.Scoped, g conditiontree.Segment) (predicate.Contact, error) {
	tree, err := decode(g.Group, segmentPlace)
	if err != nil {
		return nil, err
	}
	return (&compiler{s: s, now: m.now()}).group(ctx, tree)
}

// RulePredicate compiles a Rule's Condition for the Workspace of s, with tests of the triggering
// Event's properties allowed. props, the properties of the Event that fired the Rule (whose row
// may not exist yet), are a JSON object bound to the query, and each such test is its SQL/JSON path
// on them (ADR 0180), so Postgres decides it with the rest of the tree. A key the Event lacks, or a
// value of another type, makes the leaf false. Any key is valid, so props may be nil to only
// validate.
func (m *Module) RulePredicate(ctx context.Context, s *ent.Scoped, g conditiontree.Rule, props map[string]any) (predicate.Contact, error) {
	tree, err := decode(g.Group, rulePlace)
	if err != nil {
		return nil, err
	}
	if props == nil {
		props = map[string]any{}
	}
	trigger, err := json.Marshal(props)
	if err != nil {
		return nil, fmt.Errorf("encode the triggering Event's properties: %w", err)
	}
	return (&compiler{s: s, now: m.now(), trigger: trigger}).group(ctx, tree)
}

// LeftOut narrows a Contact query to the Contacts a Segment's Condition leaves out because an
// engagement count is unknown for them (ADR 0123): the Condition is unknown for them (Compile is
// NULL), and it would be known were every engagement count read as it stands. A Contact for whom
// only a test of another kind reads no value (a field it does not have) is not among them. A
// Condition that counts no engagement (CanBeUnknown) leaves nobody out.
func (m *Module) LeftOut(ctx context.Context, s *ent.Scoped, g conditiontree.Segment) (predicate.Contact, error) {
	tree, err := decode(g.Group, segmentPlace)
	if err != nil {
		return nil, err
	}
	if !CanBeUnknown(g.Group) {
		return predicate.Contact(func(s *sql.Selector) { s.Where(sql.False()) }), nil
	}
	now := m.now()
	unknown, err := (&compiler{s: s, now: now}).group(ctx, tree)
	if err != nil {
		return nil, err
	}
	asCounted, err := (&compiler{s: s, now: now, counted: true}).group(ctx, tree)
	if err != nil {
		return nil, err
	}
	return predicate.Contact(func(s *sql.Selector) {
		s.Where(sql.And(isNull(s, unknown), sql.Not(isNull(s, asCounted))))
	}), nil
}

// isNull is the predicate p is unknown (SQL NULL) for the row of s.
func isNull(s *sql.Selector, p predicate.Contact) *sql.Predicate {
	inner := s.Clone().SetP(nil)
	p(inner)
	held := inner.P()
	// raw sql: ent tests a column for NULL (sql.IsNull), not the value of a predicate.
	return sql.P(func(b *sql.Builder) {
		b.Wrap(func(b *sql.Builder) { b.Join(held) })
		b.WriteString(" IS NULL")
	})
}

// compiler holds what one compilation needs: the Workspace's Custom field keys are
// loaded the first time a leaf names one.
type compiler struct {
	s    *ent.Scoped
	now  time.Time
	keys map[string]customfield.Type
	// ws is the Workspace row, loaded by the first leaf that needs one of its settings: its time
	// zone, or its Engagement tracking default.
	ws *ent.Workspace
	// counted reads every engagement count as it stands, two-valued, as if every Destination
	// were tracked: what LeftOut compares the Condition with.
	counted bool
	// trigger is the properties of the Event a Rule's tests of the triggering Event read, a JSON
	// object; nil in a Segment, whose tree holds no such test.
	trigger json.RawMessage
}

func (c *compiler) group(ctx context.Context, g group) (predicate.Contact, error) {
	preds := make([]predicate.Contact, 0, len(g.entries))
	for i, n := range g.entries {
		p, err := c.node(ctx, n)
		if err != nil {
			return nil, fmt.Errorf("entry %d: %w", i, err)
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
	case birthdayLeaf:
		return c.birthday(l)
	case tagLeaf:
		return tagRule(l), nil
	case telegramLeaf:
		// The subject has a Binding on a Telegram Integration of the Workspace. A Visitor's
		// Binding is no Contact's (ADR 0171). Evaluated live; a Binding exists only for a Telegram
		// Integration and goes with it, so the existence of one is the test.
		return present(contact.HasBindings(), l.test), nil
	case smsLeaf:
		return present(textable(), l.test), nil
	case engagementTrackingLeaf:
		return l.contact(), nil
	case customFieldLeaf:
		return c.custom(ctx, l)
	case eventCountLeaf:
		return c.event(ctx, l)
	case propertyLeaf:
		return c.property(l)
	default:
		return nil, fmt.Errorf("unknown leaf %T", n)
	}
}

// attribute tests a text attribute of the Contact by a text or presence test, the ones decode
// reads for it.
func (c *compiler) attribute(l attributeLeaf) (predicate.Contact, error) {
	switch t := l.test.(type) {
	case textTest:
		return t.meaning.contact(l.field, t.value), nil
	case presenceTest:
		return t.contact(l.field), nil
	default:
		return nil, fmt.Errorf("unsupported test %T for attribute %q", l.test, l.name)
	}
}

// birthday places the Contact's date of birth: before or after a moment, or within the last days
// up to now, the tests decode reads for it.
func (c *compiler) birthday(l birthdayLeaf) (predicate.Contact, error) {
	switch t := l.test.(type) {
	case momentTest:
		return t.side.birthday(t.at.UTC()), nil
	case withinLastTest:
		since := c.since(t.days)
		return contact.And(contact.BirthdayGTE(since.UTC()), contact.BirthdayLTE(c.now.UTC())), nil
	default:
		return nil, fmt.Errorf("unsupported test %T for the birthday", l.test)
	}
}

// since is the start of a window of the last days up to now. A window reaching back past the first
// year starts there: no moment is older, and a cutoff before it would overflow a stored timestamp.
func (c *compiler) since(days int) time.Time {
	since := c.now.AddDate(0, 0, -days)
	if since.Year() < 1 {
		since = time.Date(1, time.January, 1, 0, 0, 0, 0, time.UTC)
	}
	return since
}

// property is a test of one property of the triggering Event: its SQL/JSON path, the one a filter
// compiles to (ADR 0180), on the Event's properties bound to the query.
func (c *compiler) property(l propertyLeaf) (predicate.Contact, error) {
	b := newPathBuilder()
	path, err := b.path(b.property(l.key, l.test))
	if err != nil {
		return nil, err
	}
	on := Document(c.trigger)
	return predicate.Contact(func(s *sql.Selector) { s.Where(path.On(on)) }), nil
}

// location is the Workspace time zone, in which a datetime Custom field's value without an offset
// is read (ADR 0087); it is loaded the first time a moment test needs it.
func (c *compiler) location(ctx context.Context) (*time.Location, error) {
	ws, err := c.workspace(ctx)
	if err != nil {
		return nil, err
	}
	loc, err := reportday.Location(ws.Timezone)
	if err != nil {
		return nil, fmt.Errorf("workspace time zone: %w", err)
	}
	return loc, nil
}

// workspace is the Workspace row, loaded the first time a leaf needs one of its settings.
func (c *compiler) workspace(ctx context.Context) (*ent.Workspace, error) {
	if c.ws != nil {
		return c.ws, nil
	}
	ws, err := c.s.Workspace(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: load workspace: %w", ErrStorage, err)
	}
	c.ws = ws
	return ws, nil
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
		// not NULL, so a negated group holds for it as for a Rule's property test. The key, the filter
		// and the operands are bound arguments.
		s.Where(sql.P(func(b *sql.Builder) {
			b.WriteString("COALESCE(jsonb_path_exists(")
			at(b, column, key)
			b.WriteString(", ").Arg(path).WriteString("::jsonpath, ").Arg(string(operands)).
				WriteString("::jsonb, true), false)")
		}))
	}), nil
}

// eventCount is what one operator of an event leaf means: the least times it takes, and the test of
// the Contact's count, given the correlated subquery that selects it.
type eventCount struct {
	least int
	holds func(count *sql.Selector, times int) *sql.Predicate
}

// eventCountOf is what op means, or a refusal of an operator outside the contract. Without a GROUP
// BY the count is one row even over no Events, so HAVING decides alone.
func eventCountOf(op conditiontree.EventCountOperator) (eventCount, error) {
	switch op {
	case conditiontree.AtLeast:
		return eventCount{least: 1, holds: func(count *sql.Selector, times int) *sql.Predicate {
			return sql.Exists(count.Having(sql.GTE(sql.Count("*"), times)))
		}}, nil
	case conditiontree.AtMost:
		return eventCount{least: 0, holds: func(count *sql.Selector, times int) *sql.Predicate {
			return sql.NotExists(count.Having(sql.GT(sql.Count("*"), times)))
		}}, nil
	}
	return eventCount{}, fmt.Errorf("unsupported operator %q for event leaf", op)
}

// event is the subject performed `action` at least (or at most) times times, over all time or
// within the last withinDays days. Events attach to a Contact by the stable contact_id, so
// anonymous events stitched onto it at Identify are counted here (ADR 0002), and the count is
// correlated on it, so an anonymous Event (a NULL contact_id) is in no Contact's count. A count of
// opens or clicks is unknown, in either direction, for a Contact whose Destination on the Channel
// of the count is not tracked now (ADR 0123): its Engagement tracking consent, else the
// Workspace's default; a Contact with no Destination there keeps the count.
func (c *compiler) event(ctx context.Context, l eventCountLeaf) (predicate.Contact, error) {
	preds := []predicate.Event{event.Action(l.action)}
	channel, engagement := events.EngagementChannel(l.action)
	if engagement {
		// A machine open or click is in the log and in no count (ADR 0053).
		preds = append(preds, events.HumanOnly())
	}
	if l.withinDays != nil {
		// A window reaching back past the first year starts there (since), which counts all time:
		// no Event is older. A window longer than Event retention is capped by it (ADR 0019).
		preds = append(preds, event.OccurredAtGTE(c.since(*l.withinDays)))
	}

	counted := func(s *sql.Selector) *sql.Predicate {
		t := sql.Table(event.Table)
		count := sql.Dialect(s.Dialect()).Select(sql.Count("*")).From(t)
		count.Where(sql.ColumnsEQ(count.C(event.FieldContactID), s.C(contact.FieldID)))
		for _, p := range preds {
			p(count)
		}
		return l.count.holds(count, l.times)
	}
	if !engagement || c.counted {
		return predicate.Contact(func(s *sql.Selector) { s.Where(counted(s)) }), nil
	}
	ws, err := c.workspace(ctx)
	if err != nil {
		return nil, err
	}
	dflt := ws.EngagementTrackingDefault
	return predicate.Contact(func(s *sql.Selector) {
		unknown, holds := untracked(s, channel, dflt), counted(s)
		// raw sql: ent has no CASE. The NULL stays inside the leaf, around the count's EXISTS (which
		// is never NULL), so the tree above carries it by Kleene's rules.
		s.Where(sql.P(func(b *sql.Builder) {
			b.WriteString("CASE WHEN ").Join(unknown).WriteString(" THEN NULL ELSE ").Join(holds).WriteString(" END")
		}))
	}), nil
}

// untracked is the Contact of s has a Destination on channel that is not tracked under the
// Workspace's default dflt: its email address, its phone, or any of its own Telegram Bindings (a
// Visitor's is no Contact's, ADR 0171). A Contact with none has nothing untracked, and neither has
// one on a Channel that tracks no engagement.
func untracked(s *sql.Selector, channel channels.Channel, dflt engagementtracking.Decision) *sql.Predicate {
	switch channel {
	case channels.Email:
		return sql.And(sql.NotNull(s.C(contact.FieldEmail)), engagementtracking.Untracked(s, channel, dflt, contact.FieldEmail))
	case channels.SMS:
		return sql.And(sql.NotNull(s.C(contact.FieldPhone)), engagementtracking.Untracked(s, channel, dflt, contact.FieldPhone))
	case channels.Telegram:
		b := sql.Table(binding.Table)
		chats := sql.Select(b.C(binding.FieldID)).From(b)
		chats.Where(sql.And(
			sql.EQ(b.C(binding.FieldChannel), string(binding.ChannelTelegram)),
			sql.ColumnsEQ(b.C(binding.FieldContactID), s.C(contact.FieldID)),
			engagementtracking.Untracked(chats, channel, dflt, binding.FieldDestination),
		))
		return sql.Exists(chats)
	}
	return sql.False()
}

// tagRule is the subject has (or lacks) the Tag. Evaluated live — membership is never
// materialized.
func tagRule(l tagLeaf) predicate.Contact {
	has := contact.HasTagsWith(tag.NameKey(tagname.Key(l.name)))
	return present(has, l.test)
}

// present is p, what a Contact has (a Tag, a Binding, an eligible phone), when the presence test
// asks for it, its negation when it asks for its absence.
func present(p predicate.Contact, t presenceTest) predicate.Contact {
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
		return jsonPath(l.key, asIs, test.meaning.filter(), map[string]any{"v": test.value})
	case momentTest:
		return c.customMoment(ctx, l.key, test)
	case withinLastTest:
		return c.customWindow(ctx, l.key, test)
	default:
		return customJSON(l.key, l.test)
	}
}

// fits refuses the leaf's test on a Custom field of type t unless t takes the test and its operator
// (the one table, customFieldTests, which the API lists as each Custom field's conditionTests).
func (l customFieldLeaf) fits(t customfield.Type) error {
	tests := customFieldTests[t]
	var kind conditiontree.TestKind
	var op string
	var takes bool
	switch test := l.test.(type) {
	case textTest:
		kind, op, takes = conditiontree.TestText, string(test.op), slices.Contains(tests.Text, test.op)
	case numberTest:
		kind, op, takes = conditiontree.TestNumber, string(test.op), slices.Contains(tests.Number, test.op)
	case momentTest:
		kind, op, takes = conditiontree.TestMoment, string(test.op), slices.Contains(tests.Moment, test.op)
	case withinLastTest:
		if !tests.WithinLast {
			return fmt.Errorf("custom field %q of type %s takes no %s test", l.key, t, conditiontree.TestWithinLast)
		}
		return nil
	case presenceTest:
		kind, op, takes = conditiontree.TestPresence, string(test.operator()), slices.Contains(tests.Presence, test.operator())
	default:
		return fmt.Errorf("unsupported test %T for custom field %q", l.test, l.key)
	}
	if !takes {
		return fmt.Errorf("custom field %q of type %s takes no %s test %q", l.key, t, kind, op)
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
		// Only set fits a Custom field type (fits): a key that is not set is no test here.
		if t.set {
			cond = func(column string) *sql.Predicate { return sqljson.HasKey(column, path) }
		}
	case textTest:
		switch t.op {
		case conditiontree.TextEq:
			cond = func(column string) *sql.Predicate { return sqljson.ValueEQ(column, t.value, path) }
		case conditiontree.TextNeq:
			cond = func(column string) *sql.Predicate { return sqljson.ValueNEQ(column, t.value, path) }
		case conditiontree.TextContains:
			// Membership of a JSON array (jsonb containment of the encoded value). The value
			// is read as JSON, not unquoted to text, so a string element matches as a string.
			cond = func(column string) *sql.Predicate {
				return sqljson.ValueContains(column, t.value, path, sqljson.Unquote(false))
			}
		case conditiontree.TextBeginsWith:
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

// customMoment places a datetime Custom field's moment: which side of a bound it is on.
func (c *compiler) customMoment(ctx context.Context, key string, t momentTest) (predicate.Contact, error) {
	loc, err := c.location(ctx)
	if err != nil {
		return nil, err
	}
	return jsonPath(key, asMoment, t.side.filter, map[string]any{
		"at": t.at.UTC().Format(momentLayout), "atLocal": t.at.In(loc).Format(localLayout),
	})
}

// customWindow places a datetime Custom field's moment within the last days up to now, so a moment
// still to come is not within.
func (c *compiler) customWindow(ctx context.Context, key string, t withinLastTest) (predicate.Contact, error) {
	loc, err := c.location(ctx)
	if err != nil {
		return nil, err
	}
	since := c.since(t.days)
	return jsonPath(key, asMoment, window, map[string]any{
		"since": since.UTC().Format(momentLayout), "now": c.now.UTC().Format(momentLayout),
		"sinceLocal": since.In(loc).Format(localLayout), "nowLocal": c.now.In(loc).Format(localLayout),
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

// FromDTO is a Condition of one place given as an API DTO: the API's typed tree is the stored shape
// (ADR 0169).
func FromDTO[P conditiontree.Place](group json.Marshaler) (P, error) {
	var g P
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
func FromOptionalDTO[P conditiontree.Place, G any, M interface {
	*G
	json.Marshaler
}](opt interface{ Get() (G, bool) }) (*P, error) {
	group, ok := opt.Get()
	if !ok {
		return nil, nil
	}
	g, err := FromDTO[P](M(&group))
	if err != nil {
		return nil, err
	}
	return &g, nil
}

// FitsCustomFieldType reports whether every leaf of the group, at any depth, that tests the Custom
// field key would still compile were the field of type t (customFieldLeaf.fits). A malformed entry
// tests nothing.
func FitsCustomFieldType(g conditiontree.Group, key string, t customfield.Type) bool {
	for n := range decodeSkipping(g).leaves() {
		if l, ok := n.(customFieldLeaf); ok && l.key == key && l.fits(t) != nil {
			return false
		}
	}
	return true
}

// NamesTag reports whether any leaf of the group, at any depth, tests the Tag called name.
// A malformed entry names nothing.
func NamesTag(g conditiontree.Group, name string) bool {
	for n := range decodeSkipping(g).leaves() {
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
	for n := range decodeSkipping(g).leaves() {
		if l, ok := n.(eventCountLeaf); ok && events.IsEngagement(l.action) {
			return true
		}
	}
	return false
}
