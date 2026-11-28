package automations

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationsteprecord"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/domain/contacts/condition"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
)

// The kept Events (ADR 0136) and responses (ADR 0161): a Wait for event that took `arrived` on an
// Event keeps a copy of it on its Step record (leave), and an HTTP step that got a 2xx keeps the
// response, which the Steps after them read under their names, `awaited.<name>` and
// `http.<name>`, while the Enrollment runs, and a later Branch's Condition tests (keptFor; a Wait
// for event's tests only the awaited Events). The end of the Enrollment clears them (clearKept), and
// a refusal of tracking clears a copy of an open or click it deletes (ForgetKept).

// KeptEvent is a copy an Enrollment keeps of the Event one of its Wait for event Steps arrived on,
// under the Step's name: its action and its properties, which are the sender's own, untrusted
// data, their System event keys in their public form (ADR 0142).
type KeptEvent struct {
	Name       string
	Action     string
	Properties map[string]any
}

// KeptResponse is the response an Enrollment keeps of one of its HTTP steps, under the Step's name
// (ADR 0161): its status code and the JSON object it carried, a stranger's untrusted data, nil with
// Dropped saying why when it was not one the Step could keep.
type KeptResponse struct {
	Name    string
	Status  int32
	Body    map[string]any
	Dropped *automationstep.BodyDropped
}

// KeptEvents are the copies the Enrollment run keeps, by name in its order: run is read with its
// version and its Step records that keep something, as Enrollment and Enrollments read it, so it is
// none for a run read without them, and for one that has ended, which keeps none.
func KeptEvents(run *ent.AutomationRun) []KeptEvent {
	if run.Edges.Version == nil {
		return nil
	}
	kept := keptByName(automationstep.NewIndex(run.Edges.Version.Steps), run.Edges.StepRecords)
	out := make([]KeptEvent, 0, len(kept.Awaited))
	for _, name := range slices.Sorted(maps.Keys(kept.Awaited)) {
		a := kept.Awaited[name]
		out = append(out, KeptEvent{Name: name, Action: a.Action, Properties: systemevent.PublicProperties(a.Action, a.Properties)})
	}
	return out
}

// KeptResponses are the responses the Enrollment run keeps of its HTTP steps, by name in its order,
// read as KeptEvents reads the copies of Events.
func KeptResponses(run *ent.AutomationRun) []KeptResponse {
	if run.Edges.Version == nil {
		return nil
	}
	index := automationstep.NewIndex(run.Edges.Version.Steps)
	out := []KeptResponse{}
	for _, r := range run.Edges.StepRecords {
		name, ok := responseName(index, r)
		if !ok {
			continue
		}
		out = append(out, KeptResponse{Name: name, Status: *r.HTTPStatus, Body: r.HTTPBody, Dropped: r.HTTPDropped})
	}
	slices.SortFunc(out, func(a, b KeptResponse) int { return strings.Compare(a.Name, b.Name) })
	return out
}

// withKept is the read of Enrollments with what KeptEvents and KeptResponses read: their version
// and their Step records that keep a copy or a response, each in one query for the whole page.
func withKept(q *ent.AutomationRunQuery) *ent.AutomationRunQuery {
	return q.WithVersion().
		WithStepRecords(func(r *ent.AutomationStepRecordQuery) { r.Where(keeps()) })
}

// keeps selects the Step records that keep something for the Steps after them: a copy of an
// awaited Event or an HTTP step's response.
func keeps() predicate.AutomationStepRecord {
	return automationsteprecord.Or(automationsteprecord.AwaitedEventIDNotNil(), automationsteprecord.HTTPStatusNotNil())
}

// keptOn is what the Enrollment runID keeps by the name each of its Steps has in index, the tree of
// its version: the Events its Wait for event Steps arrived on and the responses of its HTTP steps,
// what its send and HTTP steps bind as `awaited` and `http`.
func keptOn(ctx context.Context, s *ent.Scoped, runID int64, index *automationstep.Index) (outboundmessages.Kept, error) {
	records, err := s.AutomationStepRecord().Query().
		Where(automationsteprecord.RunID(runID), keeps()).
		All(ctx)
	if err != nil {
		return outboundmessages.Kept{}, err
	}
	return keptByName(index, records), nil
}

// keptByName is what the Step records keep, by the name of the Step of index each record is of: a
// Wait for event's copy of an Event, an HTTP step's response. A record of no such Step of the tree
// keeps none.
func keptByName(index *automationstep.Index, records []*ent.AutomationStepRecord) outboundmessages.Kept {
	kept := outboundmessages.Kept{Awaited: map[string]outboundmessages.AwaitedEvent{}, HTTP: map[string]outboundmessages.KeptResponse{}}
	for _, r := range records {
		if name, ok := responseName(index, r); ok {
			kept.HTTP[name] = outboundmessages.KeptResponse{Status: int(*r.HTTPStatus), Body: r.HTTPBody}
			continue
		}
		st, ok := index.Step(r.StepID)
		if !ok || r.AwaitedAction == nil {
			continue
		}
		w, waits := st.Payload.(automationstep.WaitForEvent)
		if !waits {
			continue
		}
		kept.Awaited[w.Name] = outboundmessages.AwaitedEvent{Action: *r.AwaitedAction, Properties: r.AwaitedProperties}
	}
	return kept
}

// responseName is the name of the HTTP step of index whose response the record r keeps; false
// when it keeps none, or is of no HTTP step of the tree.
func responseName(index *automationstep.Index, r *ent.AutomationStepRecord) (string, bool) {
	if r.HTTPStatus == nil {
		return "", false
	}
	st, ok := index.Step(r.StepID)
	if !ok {
		return "", false
	}
	h, calls := st.Payload.(automationstep.HTTP)
	return h.Name, calls
}

// testedKept is what a Step's Conditions read of what the Enrollment keeps: the properties of each
// awaited Event and the body of each HTTP step's response its tests name, by name. A name with
// nothing kept is absent, and its tests fail.
type testedKept struct {
	awaited, http map[string]map[string]any
}

// keptFor is what a Step's Conditions read of what the Enrollment runID keeps (ADR 0136, ADR 0161),
// by the names of its Steps in index, the tree of its version: the copies and the responses the
// tests of groups name. Nothing, and no read, when groups test neither.
func keptFor(ctx context.Context, s *ent.Scoped, runID int64, index *automationstep.Index, groups ...conditiontree.Group) (testedKept, error) {
	awaited := lo.Uniq(lo.FlatMap(groups, func(g conditiontree.Group, _ int) []string { return condition.AwaitedNames(g) }))
	responses := lo.Uniq(lo.FlatMap(groups, func(g conditiontree.Group, _ int) []string { return condition.HTTPNames(g) }))
	if len(awaited) == 0 && len(responses) == 0 {
		return testedKept{}, nil
	}
	kept, err := keptOn(ctx, s, runID, index)
	if err != nil {
		return testedKept{}, err
	}
	out := testedKept{awaited: map[string]map[string]any{}, http: map[string]map[string]any{}}
	for _, name := range awaited {
		if a, ok := kept.Awaited[name]; ok {
			out.awaited[name] = a.Properties
		}
	}
	for _, name := range responses {
		if r, ok := kept.HTTP[name]; ok && r.Body != nil {
			out.http[name] = r.Body
		}
	}
	return out, nil
}

// checkAwaitedTests refuses, naming the Step, a Branch path's or a Wait for event's Condition with a
// test of the awaited Event that names no Wait for event that can come before the Step holding it
// (ADR 0136), by the rule a Branch's test of an earlier message follows (automationstep.Before): a
// later one, the Wait for event itself, one of a path the Enrollment did not take, or none of the
// version, whose test would always fail. So is one that names a Wait for event on an open or a
// click, which keeps no copy for it to test, and a Branch path's test of an HTTP step's response
// that names no HTTP step that can come before the Branch (ADR 0161).
func checkAwaitedTests(steps automationstep.Path) error {
	for _, st := range automationstep.All(steps) {
		switch p := st.Payload.(type) {
		case automationstep.Branch:
			for i, path := range p.Paths {
				why := awaitedProblem(steps, st.ID, path.Condition.Group, "the branch")
				if why == "" {
					why = responseProblem(steps, st.ID, path.Condition.Group)
				}
				if why != "" {
					return refuse(st, fmt.Sprintf("path %d: %s", i+1, why))
				}
			}
		case automationstep.WaitForEvent:
			cond := p.Condition()
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
			_, onMessage := w.Message()
			keeps[w.Name] = !onMessage
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

// responseProblem is why the first test of an HTTP step's response in g, a Condition of the Branch
// at in steps, cannot read one: the name it reads is held by no HTTP step that can come before the
// Branch (ADR 0161); "" when every test can.
func responseProblem(steps automationstep.Path, at automationstep.ID, g conditiontree.Group) string {
	names := condition.HTTPNames(g)
	if len(names) == 0 {
		return ""
	}
	calls := map[string]bool{}
	for _, st := range automationstep.Before(steps, at) {
		if h, ok := st.Payload.(automationstep.HTTP); ok {
			calls[h.Name] = true
		}
	}
	for _, name := range names {
		if !calls[name] {
			return fmt.Sprintf("the http response test names %q, which is no http step before the branch", name)
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
