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
// end of the Enrollment clears them (clearKept).

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
// version, whose test would always fail.
func checkAwaitedTests(steps automationstep.Path) error {
	for _, st := range automationstep.All(steps) {
		switch p := st.Payload.(type) {
		case automationstep.Branch:
			for i, path := range p.Paths {
				if name, ok := awaitedNotBefore(steps, st.ID, path.Condition.Group); ok {
					return refuse(st, fmt.Sprintf("path %d: the awaited event test names %q, which is no wait for event before the branch", i+1, name))
				}
			}
		case automationstep.WaitForEvent:
			cond := awaitedCondition(p)
			if cond == nil {
				continue
			}
			if name, ok := awaitedNotBefore(steps, st.ID, cond.Group); ok {
				return refuse(st, fmt.Sprintf("the condition: the awaited event test names %q, which is no wait for event before it", name))
			}
		}
	}
	return nil
}

// awaitedNotBefore is the first name g's tests of the awaited Event read that no Wait for event
// that can come before the Step at in steps holds, and whether there is one.
func awaitedNotBefore(steps automationstep.Path, at automationstep.ID, g conditiontree.Group) (string, bool) {
	names := condition.AwaitedNames(g)
	if len(names) == 0 {
		return "", false
	}
	before := map[string]bool{}
	for _, st := range automationstep.Before(steps, at) {
		if w, ok := st.Payload.(automationstep.WaitForEvent); ok {
			before[w.Name] = true
		}
	}
	return lo.Find(names, func(name string) bool { return !before[name] })
}
