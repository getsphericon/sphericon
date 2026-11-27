package automations

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationsteprecord"
	"github.com/getsphericon/sphericon/internal/domain/contacts/condition"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
)

// The kept Events (ADR 0136): a Wait for event that took `arrived` on an Event keeps a copy of it on
// its Step record (leave), which the Steps after it read under its name, `awaited.<name>`, while
// the Enrollment runs, and a later Branch's or Wait for event's Condition tests (awaitedFor). The
// end of the Enrollment clears them (clearKept), and so does a refusal of tracking that deletes the
// open or click a copy was taken from (ForgetKept).

// KeptEvent is a copy an Enrollment keeps of the Event one of its Wait for event Steps arrived on,
// under the Step's name: its action and its properties, which are the sender's own, untrusted
// data, their System event keys in their public form (ADR 0142).
type KeptEvent struct {
	Name       string
	Action     string
	Properties map[string]any
}

// KeptEvents are the copies the Enrollment run keeps, by name in its order: run is read with its
// version and its Step records that keep a copy, as Enrollment and Enrollments read it, so it is
// none for a run read without them, and for one that has ended, which keeps none.
func KeptEvents(run *ent.AutomationRun) []KeptEvent {
	if run.Edges.Version == nil {
		return nil
	}
	kept := keptByName(automationstep.NewIndex(run.Edges.Version.Steps), run.Edges.StepRecords)
	out := make([]KeptEvent, 0, len(kept))
	for _, name := range slices.Sorted(maps.Keys(kept)) {
		a := kept[name]
		out = append(out, KeptEvent{Name: name, Action: a.Action, Properties: systemevent.PublicProperties(a.Action, a.Properties)})
	}
	return out
}

// withKept is the read of Enrollments with what KeptEvents reads: their version and their Step
// records that keep a copy, each in one query for the whole page.
func withKept(q *ent.AutomationRunQuery) *ent.AutomationRunQuery {
	return q.WithVersion().
		WithStepRecords(func(r *ent.AutomationStepRecordQuery) {
			r.Where(automationsteprecord.AwaitedEventIDNotNil())
		})
}

// keptOn is what the Enrollment runID keeps of the Events its Wait for event Steps arrived on, by
// the name each Step has in index, the tree of its version: what its send and HTTP steps bind as
// `awaited`.
func keptOn(ctx context.Context, s *ent.Scoped, runID int64, index *automationstep.Index) (map[string]outboundmessages.AwaitedEvent, error) {
	records, err := s.AutomationStepRecord().Query().
		Where(automationsteprecord.RunID(runID), automationsteprecord.AwaitedEventIDNotNil()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return keptByName(index, records), nil
}

// keptByName is the copies the Step records hold, by the name of the Wait for event of index each
// record is of. A record of no Wait for event of the tree keeps none.
func keptByName(index *automationstep.Index, records []*ent.AutomationStepRecord) map[string]outboundmessages.AwaitedEvent {
	kept := map[string]outboundmessages.AwaitedEvent{}
	for _, r := range records {
		st, ok := index.Step(r.StepID)
		if !ok || r.AwaitedAction == nil {
			continue
		}
		w, waits := st.Payload.(automationstep.WaitForEvent)
		if !waits {
			continue
		}
		kept[w.Name] = outboundmessages.AwaitedEvent{Action: *r.AwaitedAction, Properties: r.AwaitedProperties}
	}
	return kept
}

// awaitedFor is what a Step's Conditions read of the Events the Enrollment runID keeps (ADR 0136):
// the properties of each copy a test of the awaited Event in groups names, by the name of its Wait
// for event in index, the tree of its version. A name with no copy is absent, and its tests fail.
// None, and no read, when groups test no awaited Event.
func awaitedFor(ctx context.Context, s *ent.Scoped, runID int64, index *automationstep.Index, groups ...conditiontree.Group) (map[string]map[string]any, error) {
	names := lo.FlatMap(groups, func(g conditiontree.Group, _ int) []string { return condition.AwaitedNames(g) })
	if len(names) == 0 {
		return nil, nil
	}
	kept, err := keptOn(ctx, s, runID, index)
	if err != nil {
		return nil, err
	}
	out := make(map[string]map[string]any, len(names))
	for _, name := range names {
		if a, ok := kept[name]; ok {
			out[name] = a.Properties
		}
	}
	return out, nil
}

// checkAwaitedTests refuses, naming the Step, a Branch path's or a Wait for event's Condition with a
// test of the awaited Event that names no Wait for event that can come before the Step holding it
// (ADR 0136), by the rule a Branch's test of an earlier message follows (automationstep.Before): a
// later one, the Wait for event itself, one of a path the Enrollment did not take, or none of the
// version, whose test would always fail. So is one that names a Wait for event on an open or a
// click, which keeps no copy for it to test.
func checkAwaitedTests(steps automationstep.Path) error {
	for _, st := range automationstep.All(steps) {
		switch p := st.Payload.(type) {
		case automationstep.Branch:
			for i, path := range p.Paths {
				if why := awaitedProblem(steps, st.ID, path.Condition.Group, "the branch"); why != "" {
					return refuse(st, fmt.Sprintf("path %d: %s", i+1, why))
				}
			}
		case automationstep.WaitForEvent:
			cond := awaitedCondition(p)
			if cond == nil {
				continue
			}
			if why := awaitedProblem(steps, st.ID, cond.Group, "it"); why != "" {
				return refuse(st, "the condition: "+why)
			}
		}
	}
	return nil
}

// awaitedProblem is why the first test of the awaited Event in g that cannot read a copy, at the
// Step at in steps (named before in the refusal), cannot: the name it reads is held by no Wait for
// event that can come before the Step, or by one on an open or a click, which keeps none; "" when
// every test can.
func awaitedProblem(steps automationstep.Path, at automationstep.ID, g conditiontree.Group, before string) string {
	names := condition.AwaitedNames(g)
	if len(names) == 0 {
		return ""
	}
	keeps := map[string]bool{}
	for _, st := range automationstep.Before(steps, at) {
		if w, ok := st.Payload.(automationstep.WaitForEvent); ok {
			_, onEvent := w.Awaits.(automationstep.AwaitedEvent)
			keeps[w.Name] = onEvent
		}
	}
	for _, name := range names {
		onEvent, found := keeps[name]
		switch {
		case !found:
			return fmt.Sprintf("the awaited event test names %q, which is no wait for event before %s", name, before)
		case !onEvent:
			return fmt.Sprintf("the awaited event test names %q, a wait for an open or a click, which keeps no event", name)
		}
	}
	return ""
}

// ForgetKept clears every copy a Wait for event kept of one of the Events of eventIDs (ADR 0136):
// the opens and clicks a refusal of tracking deletes of its Destination's earlier messages (ADR
// 0120), cleared in the transaction that deletes them by the use case of the reach-back
// (internal/usecase/trackingwithdrawal) or by an Erasure that does its work first
// (internal/usecase/contacterasure). Each Step record keeps the path it took, and a later Step reads the name as absent.
// A copy cleared before, or of another Event, is left as it is, so a second run clears nothing more.
func (m *Module) ForgetKept(ctx context.Context, s *ent.Scoped, eventIDs []int64) error {
	for _, chunk := range lo.Chunk(eventIDs, forgetChunk) {
		if err := clearKeptWhere(ctx, s, automationsteprecord.AwaitedEventIDIn(chunk...)); err != nil {
			return err
		}
	}
	return nil
}

// forgetChunk is the Event ids one statement of ForgetKept takes: a refusal deletes every repeat
// of an open or click, more than a statement takes parameters.
const forgetChunk = 1000
