package automations

import (
	"context"
	"maps"
	"slices"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationsteprecord"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
)

// The kept Events (ADR 0136): a Wait for event that took `arrived` on an Event keeps a copy of it on
// its Step record (leave), which the Steps after it read under its name, `awaited.<name>`, while
// the Enrollment runs. The end of the Enrollment clears them (clearKept), and so does a refusal of
// tracking that deletes the open or click a copy was taken from (ForgetKept).

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

// ForgetKept clears every copy a Wait for event kept of one of the Events of eventIDs (ADR 0136):
// the opens and clicks a refusal of tracking deletes of its Destination's earlier messages (ADR
// 0120), cleared by the use case internal/usecase/trackingwithdrawal in the transaction that
// deletes them. Each Step record keeps the path it took, and a later Step reads the name as absent.
// A copy cleared before, or of another Event, is left as it is, so a second run clears nothing more.
func (m *Module) ForgetKept(ctx context.Context, s *ent.Scoped, eventIDs []int64) error {
	if len(eventIDs) == 0 {
		return nil
	}
	return clearKeptWhere(ctx, s, automationsteprecord.AwaitedEventIDIn(eventIDs...))
}
