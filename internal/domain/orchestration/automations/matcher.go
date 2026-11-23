package automations

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/domain/contacts/condition"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
)

// matching is the predicate that a stored Event matches m (ADR 0133): it is of m's action and its
// properties pass m's filter, compiled to one SQL/JSON path that Postgres runs on them (ADR 0180).
// A filter that does not compile is an error: the save refuses one (validateTrigger).
func matching(m eventmatcher.Matcher) (predicate.Event, error) {
	preds := []predicate.Event{event.Action(m.Action)}
	if m.Filter != nil {
		path, err := condition.CompileEventFilter(*m.Filter)
		if err != nil {
			return nil, err
		}
		preds = append(preds, path.Holds(event.FieldProperties))
	}
	return event.And(preds...), nil
}

// matchesEvent is the entry-time filter check: whether the Event stored with sourceID matches m,
// an Event of m's action that the trigger job hands on. A matcher with no filter matches every
// Event of its action without reading the row, which may be gone (Retention, an Erasure); one with
// a filter runs it on the stored row's properties, and an Event no longer stored matches none.
func matchesEvent(ctx context.Context, s *ent.Scoped, m eventmatcher.Matcher, sourceID string) (bool, error) {
	if m.Filter == nil {
		return true, nil
	}
	p, err := matching(m)
	if err != nil {
		return false, err
	}
	return s.Event().Query().Where(event.SourceID(sourceID), p).Exist(ctx)
}
