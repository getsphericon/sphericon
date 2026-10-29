// Package segments is the Segment domain module: named Conditions on Contacts, with the rows they
// select and the live work that uses them.
package segments

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/segment"
	"github.com/getsphericon/sphericon/ent/tag"
	"github.com/getsphericon/sphericon/internal/condition"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/inuse"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/validation"
)

// Domain errors returned by Module. Callers match with errors.Is.
var (
	// ErrInvalidCondition: the condition does not parse or does not fit the
	// Contact schema. The wrapped message is safe to show to the author.
	ErrInvalidCondition = validation.Refuse("segments: invalid condition", "errors.segment_condition_invalid")
	// ErrNotFound: no such Segment in this Workspace.
	ErrNotFound = validation.NotFound("segments: segment not found", "errors.segment_not_found")
)

// Module is the one place a Segment is written, validated, previewed and counted
// (ADR 0016). Create and update share one validation rule set; handlers stay thin
// adapters (scope check, call, map). Membership is the rule alone — Send-eligibility
// is subtracted only at send, never folded into a count (ADR 0001).
type Module struct {
	bus *events.Bus
}

// New builds the Segments module. It holds no client: every call takes the
// Workspace's scoped client (ADR 0017); the bus runs the archive in one transaction.
func New(bus *events.Bus) *Module { return &Module{bus: bus} }

// CreateInput is a new Segment. A nil or empty Condition means "no rules yet".
type CreateInput struct {
	Name      string
	Condition *string
}

// UpdateInput changes a Segment; nil fields are left as they are.
type UpdateInput struct {
	Name      *string
	Condition *string
}

// Validate checks a condition against the Contact attributes and the Workspace's Custom
// field catalogue. An empty condition is valid. This is the single rule set behind
// Create, Update, Preview and Count.
func (s *Module) Validate(ctx context.Context, ws *ent.Scoped, cond string) error {
	if cond == "" {
		return nil
	}
	if _, err := condition.ContactPredicate(ctx, ws, cond); err != nil {
		return ErrInvalidCondition.With(err.Error())
	}
	return nil
}

// Create validates the condition and stores the Segment.
func (s *Module) Create(ctx context.Context, ws *ent.Scoped, in CreateInput) (*ent.Segment, error) {
	if in.Condition != nil {
		if err := s.Validate(ctx, ws, *in.Condition); err != nil {
			return nil, err
		}
	}
	return ws.Segment().Create().
		SetName(in.Name).
		SetNillableCondition(in.Condition).
		Save(ctx)
}

// Get returns one Segment. An unknown id is ErrNotFound.
func (s *Module) Get(ctx context.Context, ws *ent.Scoped, id int64) (*ent.Segment, error) {
	seg, err := ws.Segment().Get(ctx, id)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	return seg, err
}

// Update validates a changed condition exactly as Create does, then applies the change.
func (s *Module) Update(ctx context.Context, ws *ent.Scoped, id int64, in UpdateInput) (*ent.Segment, error) {
	if in.Condition != nil {
		if err := s.Validate(ctx, ws, *in.Condition); err != nil {
			return nil, err
		}
	}
	q := ws.Segment().UpdateOneID(id).
		SetNillableName(in.Name).
		SetNillableCondition(in.Condition)
	seg, err := q.Save(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	return seg, err
}

// Preview counts the Contacts in the Workspace that match an unsaved condition.
func (s *Module) Preview(ctx context.Context, ws *ent.Scoped, cond string) (int, error) {
	pred, err := condition.ContactPredicate(ctx, ws, cond)
	if err != nil {
		return 0, ErrInvalidCondition.With(err.Error())
	}
	return ws.Contact().Query().Where(pred).Count(ctx)
}

// Count counts the Contacts matching a stored Segment, evaluated live.
func (s *Module) Count(ctx context.Context, ws *ent.Scoped, id int64) (int, error) {
	seg, err := ws.Segment().Get(ctx, id)
	if ent.IsNotFound(err) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	cond := ""
	if seg.Condition != nil {
		cond = *seg.Condition
	}
	return s.Preview(ctx, ws, cond)
}

// Contacts returns one page of the Contacts a stored Segment matches, evaluated live, ascending by id.
func (s *Module) Contacts(ctx context.Context, ws *ent.Scoped, id int64, p pagination.Params) (pagination.Page[*ent.Contact], error) {
	seg, err := ws.Segment().Get(ctx, id)
	if ent.IsNotFound(err) {
		return pagination.Page[*ent.Contact]{}, ErrNotFound
	}
	if err != nil {
		return pagination.Page[*ent.Contact]{}, err
	}
	cond := ""
	if seg.Condition != nil {
		cond = *seg.Condition
	}
	pred, err := condition.ContactPredicate(ctx, ws, cond)
	if err != nil {
		return pagination.Page[*ent.Contact]{}, ErrInvalidCondition.With(err.Error())
	}
	rows := func() *ent.ContactQuery {
		return ws.Contact().Query().Where(pred).WithTags(func(q *ent.TagQuery) { q.Order(ent.Asc(tag.FieldName)) })
	}
	return pagination.List(ctx, p,
		func(ctx context.Context) (int, error) { return rows().Count(ctx) },
		func(ctx context.Context, limit, offset int) ([]*ent.Contact, error) {
			return rows().Order(ent.Asc(contact.FieldID)).Limit(limit).Offset(offset).All(ctx)
		})
}

// List returns one page of the Workspace's Segments, ascending by id: the kept ones, or with
// archived the archived ones.
func (s *Module) List(ctx context.Context, ws *ent.Scoped, p pagination.Params, archived bool) (pagination.Page[*ent.Segment], error) {
	rows := func() *ent.SegmentQuery {
		if archived {
			return ws.Segment().QueryDiscarded()
		}
		return ws.Segment().Query()
	}
	return pagination.List(ctx, p,
		func(ctx context.Context) (int, error) { return rows().Count(ctx) },
		func(ctx context.Context, limit, offset int) ([]*ent.Segment, error) {
			return rows().Order(ent.Asc(segment.FieldID)).Limit(limit).Offset(offset).All(ctx)
		})
}

// Archive discards a Segment (ADR 0049), unless a draft or scheduled Broadcast uses it: then it
// returns an *inuse.Error naming those Broadcasts. A sent or failed Broadcast keeps its stored
// reference as history and does not block. A missing Segment is ErrNotFound; archiving an
// archived one changes nothing.
func (s *Module) Archive(ctx context.Context, ws *ent.Scoped, id int64) error {
	return s.bus.WithinScopedTx(ctx, ws, func(ts *ent.Scoped, _ events.Publisher) error {
		seg, err := ts.Segment().Get(ctx, id)
		if ent.IsNotFound(err) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if seg.DiscardedAt != nil {
			return nil
		}
		live, err := ts.Broadcast().Query().
			Where(broadcast.SegmentID(id), broadcast.StatusIn(broadcast.StatusDraft, broadcast.StatusScheduled)).
			Order(ent.Asc(broadcast.FieldID)).
			All(ctx)
		if err != nil {
			return err
		}
		if len(live) > 0 {
			return &inuse.Error{What: inuse.SubjectSegment, Users: inuse.Broadcasts(live)}
		}
		return ts.Segment().Discard(ctx, id)
	})
}

// Unarchive brings an archived Segment back to the lists. Restoring one that is not archived
// changes nothing; a missing Segment is ErrNotFound.
func (s *Module) Unarchive(ctx context.Context, ws *ent.Scoped, id int64) error {
	err := ws.Segment().Restore(ctx, id)
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	return err
}
