package condition

import (
	"context"
	"encoding/json"
	"fmt"

	"entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
)

// A Branch's paths each hold a Condition (ADR 0124), of the Branch path's place: a Rule's leaves,
// whose property tests read the Event the Enrollment began with or the one an earlier Wait for
// event arrived on (ADR 0136). The Branch reads all of them for
// its Contact in one statement, so every path is judged on one consistent read of the Contact.

// CheckBranch checks a Branch path's Condition as a Rule's is checked: it reads as the place's
// tree, and every leaf names a Contact attribute, a Tag, an Event action or a Custom field of the
// Workspace of s, by a test its type takes. A property test may name any key.
func (m *Module) CheckBranch(ctx context.Context, s *ent.Scoped, g conditiontree.Branch) error {
	tree, err := decode(g.Group, branchPlace)
	if err != nil {
		return err
	}
	_, err = (&compiler{s: s, now: m.now(), trigger: json.RawMessage(`{}`)}).group(ctx, tree)
	return err
}

// OutcomesInput is what Outcomes reads: the Contact, the Conditions in order, the properties of
// the Event the Enrollment began with (nil for none), which each test of the triggering Event
// reads, the properties of the Events its Wait for event Steps arrived on by each Step's name,
// which each test of the awaited Event reads (a name absent from it has no copy, ADR 0136), and the
// messages the Enrollment was sent by the send Step that sent each, which each test of its own
// message reads (a Step absent from it sent none).
type OutcomesInput struct {
	ContactID  int64
	Conditions []conditiontree.Branch
	Properties map[string]any
	Awaited    map[string]map[string]any
	Messages   map[automationstep.ID]SentMessage
}

// Outcomes reads what each Condition of in is for its Contact, on its live data, in the order
// given, by Kleene's logic (ADR 0123): it holds, it fails, or it is unknown. The Event's properties
// are a JSON object bound to the statement, which each property test reads (ADR 0180). Unknown is the unknown of engagement alone: a Condition is
// unknown when SQL reads it NULL and would read it true or false were every engagement count read
// as it stands; a NULL of a test that reads no value (a field the Contact has none of) is no match,
// false, as a Segment and a Rule read it. Every Condition is read in one statement, one snapshot of
// the Contact. A Condition that does not compile fails the read.
func (m *Module) Outcomes(ctx context.Context, s *ent.Scoped, in OutcomesInput) ([]conditiontree.Truth, error) {
	contactID, conds, props := in.ContactID, in.Conditions, in.Properties
	if len(conds) == 0 {
		return []conditiontree.Truth{}, nil
	}
	trigger, err := propertiesDocument(props)
	if err != nil {
		return nil, fmt.Errorf("encode the Enrollment's Event properties: %w", err)
	}
	now := m.now()
	read := &compiler{s: s, now: now, trigger: trigger, awaited: in.Awaited, messages: in.Messages}
	counted := &compiler{s: s, now: now, trigger: trigger, awaited: in.Awaited, messages: in.Messages, counted: true}
	type judged struct{ held, asCounted predicate.Contact }
	preds := make([]judged, len(conds))
	for i, g := range conds {
		tree, err := decode(g.Group, branchPlace)
		if err != nil {
			return nil, fmt.Errorf("path %d: %w", i, err)
		}
		if preds[i].held, err = read.group(ctx, tree); err != nil {
			return nil, fmt.Errorf("path %d: %w", i, err)
		}
		if CanBeUnknown(g.Group) {
			if preds[i].asCounted, err = counted.group(ctx, tree); err != nil {
				return nil, fmt.Errorf("path %d: %w", i, err)
			}
		}
	}
	var rows []struct {
		Truths string `sql:"truths"`
	}
	err = s.Contact().Query().
		Where(contact.ID(contactID)).
		Modify(func(sel *sql.Selector) {
			// raw sql: ent has no CASE and no select expression of a predicate. Each Condition
			// is one letter of the column: t, f or u.
			sel.Select().AppendSelectExprAs(sql.ExprFunc(func(b *sql.Builder) {
				b.WriteString("CONCAT(")
				for i, p := range preds {
					if i > 0 {
						b.WriteString(", ")
					}
					truth(b, sel, p.held, p.asCounted)
				}
				b.WriteString(")")
			}), "truths")
		}).
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("%w: read the Branch's Conditions: %w", ErrStorage, err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("%w: contact %d is gone", ErrStorage, contactID)
	}
	return truthsOf(rows[0].Truths, len(conds))
}

// truth writes the letter of p's truth for the row of sel: t when it holds, f when it fails, u when
// it is NULL and asCounted, p with every engagement count read as it stands, is not; f for any
// other NULL, and for any NULL of a p with no engagement count (asCounted nil).
func truth(b *sql.Builder, sel *sql.Selector, p, asCounted predicate.Contact) {
	held := predicateOf(sel, p)
	b.WriteString("CASE WHEN ").Join(held).WriteString(" THEN 't' WHEN NOT ").Join(held).WriteString(" THEN 'f'")
	if asCounted != nil {
		b.WriteString(" WHEN ").Join(predicateOf(sel, asCounted)).WriteString(" IS NOT NULL THEN 'u'")
	}
	b.WriteString(" ELSE 'f' END")
}

// predicateOf is p as an expression on the row of sel, wrapped in parentheses.
func predicateOf(sel *sql.Selector, p predicate.Contact) *sql.Predicate {
	inner := sel.Clone().SetP(nil)
	p(inner)
	held := inner.P()
	// raw sql: ent has no expression of a predicate's value; it is the predicate in parentheses.
	return sql.P(func(b *sql.Builder) {
		b.Wrap(func(b *sql.Builder) { b.Join(held) })
	})
}

// truthsOf reads the letters Outcomes selected, one per Condition.
func truthsOf(letters string, n int) ([]conditiontree.Truth, error) {
	if len(letters) != n {
		return nil, fmt.Errorf("%w: read %d truths for %d Conditions", ErrStorage, len(letters), n)
	}
	out := make([]conditiontree.Truth, n)
	for i := range letters {
		switch letters[i] {
		case 't':
			out[i] = conditiontree.TruthTrue
		case 'f':
			out[i] = conditiontree.TruthFalse
		default:
			out[i] = conditiontree.TruthUnknown
		}
	}
	return out, nil
}
