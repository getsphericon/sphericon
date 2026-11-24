// Package segments is the Segment domain module: named Conditions on Contacts, with the rows they
// select and the live work that uses them.
package segments

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/segment"
	"github.com/getsphericon/sphericon/ent/tag"
	"github.com/getsphericon/sphericon/internal/domain/contacts/condition"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// Domain errors returned by Module. Callers match with errors.Is.
var (
	// ErrInvalidCondition: the condition does not parse or does not fit the
	// Contact schema. The wrapped message is safe to show to the author.
	ErrInvalidCondition = validation.Refuse("segments: invalid condition", i18n.ErrorsSegmentConditionInvalid)
	// ErrNotFound: no such Segment in this Workspace.
	ErrNotFound = validation.NotFound("segments: segment not found", i18n.ErrorsSegmentNotFound)
	// ErrArchived: the Segment is archived, so it is read-only until it is unarchived (ADR 0100).
	ErrArchived = validation.Conflict("segments: segment is archived", i18n.ErrorsSegmentArchived)
	// ErrRevisionChanged: the update was made on a revision the Segment has moved past (ADR 0119):
	// someone wrote it since the client read it. Nothing is written.
	ErrRevisionChanged = validation.RevisionChanged("segments: segment changed since it was read", i18n.ErrorsSegmentRevisionChanged)
)

// Module is the one place a Segment is written, validated, previewed and counted
// (ADR 0016). Create and update share one validation rule set; handlers stay thin
// adapters (scope check, call, map). Membership is the rule alone — Send-eligibility
// is subtracted only at send, never folded into a count (ADR 0001).
type Module struct {
	bus *events.Bus
	// now is the Instance clock an archive is dated by.
	now func() time.Time
	// conditions compiles a rule over the same clock, which a relative date counts back from.
	conditions  *condition.Module
	eligibility *eligibility.Module
}

// New builds the Segments module. It holds no client: every call takes the
// Workspace's scoped client (ADR 0017); the bus runs a save and the archive each in one
// transaction under the Workspace's lock (ADR 0099). now is the Instance clock.
func New(bus *events.Bus, now func() time.Time) *Module {
	return &Module{bus: bus, now: now, conditions: condition.New(now), eligibility: eligibility.New()}
}

// CreateInput is a new Segment. A nil Condition is the empty group: no tests yet, every Contact.
type CreateInput struct {
	Name      string
	Condition *conditiontree.Segment
}

// UpdateInput is a patch of a Segment: a field that keeps leaves the stored value. Neither field
// can be cleared; a Condition with no tests matches all contacts.
type UpdateInput struct {
	// Revision is the Segment's revision the edit was made on (ADR 0119).
	Revision  int64
	Name      patch.Field[string]
	Condition patch.Field[conditiontree.Segment]
}

// Validate checks a condition against the Contact attributes and the Workspace's Custom
// field catalogue. A condition with no tests is valid. This is the single rule set behind
// Create, Update, Preview and Count.
func (s *Module) Validate(ctx context.Context, ws *ent.Scoped, cond conditiontree.Segment) error {
	if _, err := s.conditions.Compile(ctx, ws, cond); err != nil {
		return ErrInvalidCondition.With(err.Error())
	}
	return nil
}

// Create validates the condition and stores the Segment. It holds the Workspace's lock (ADR 0099):
// the Condition may name a Tag or test a Custom field, and the delete of that Tag and the retype of
// that field check for it under the same lock.
func (s *Module) Create(ctx context.Context, ws *ent.Scoped, in CreateInput) (*ent.Segment, error) {
	name, err := validation.Name(in.Name)
	if err != nil {
		return nil, err
	}
	in.Name = name
	var seg *ent.Segment
	err = s.bus.WithinWorkspaceLockTx(ctx, ws, func(wl *ent.WorkspaceLocked, _ events.Publisher) error {
		seg, err = s.createIn(ctx, wl, in)
		return err
	})
	return seg, err
}

// createIn validates the condition and stores the Segment under the Workspace's lock (ADR 0108).
func (s *Module) createIn(ctx context.Context, wl *ent.WorkspaceLocked, in CreateInput) (*ent.Segment, error) {
	cond := conditiontree.Segment{Group: conditiontree.Empty()}
	if in.Condition != nil {
		cond = *in.Condition
	}
	if err := s.Validate(ctx, wl.Scope(), cond); err != nil {
		return nil, err
	}
	return wl.Segment().Create().
		SetName(in.Name).
		SetCondition(cond).
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

// Update validates a changed condition exactly as Create does, then applies the change, under the
// Workspace's lock as Create, for the references the condition names. An archived Segment is
// ErrArchived: it is read-only until it is unarchived (ADR 0100), so the change is an update
// conditioned on its not being archived (ADR 0109) and on the revision it was made on (ADR 0119):
// ErrRevisionChanged when the Segment has moved past it.
func (s *Module) Update(ctx context.Context, ws *ent.Scoped, id int64, in UpdateInput) (*ent.Segment, error) {
	name, err := validation.NamePatch(in.Name)
	if err != nil {
		return nil, err
	}
	in.Name = name
	var seg *ent.Segment
	err = s.bus.WithinWorkspaceLockTx(ctx, ws, func(wl *ent.WorkspaceLocked, _ events.Publisher) error {
		seg, err = s.updateIn(ctx, wl, id, in)
		return err
	})
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	return seg, err
}

// updateIn validates a changed condition and applies the change under the Workspace's lock (ADR
// 0108), conditioned on the Segment's not being archived (ErrArchived) and on its revision
// (ErrRevisionChanged).
func (s *Module) updateIn(ctx context.Context, wl *ent.WorkspaceLocked, id int64, in UpdateInput) (*ent.Segment, error) {
	if cond, ok := in.Condition.Get(); ok {
		if err := s.Validate(ctx, wl.Scope(), cond); err != nil {
			return nil, err
		}
	}
	q := wl.Segment().EditAt(id, in.Revision).Where(segment.ArchivedAtIsNil()).
		Refusing(ErrNotFound, ErrRevisionChanged, refuseArchived)
	in.Name.ApplySet(q.SetName)
	in.Condition.ApplySet(q.SetCondition)
	seg, err := q.Save(ctx)
	return seg, err
}

// refuseArchived is ErrArchived for an archived Segment, which is read-only until it is unarchived.
func refuseArchived(s *ent.Segment) error {
	if s.ArchivedAt != nil {
		return ErrArchived
	}
	return nil
}

// PreviewInput is an unsaved Condition to preview, nil for every Contact, and the Channel of a
// Broadcast the audience is asked for, with the Integration it routes through (nil: the Channel's
// default). Without a Channel the preview counts the match alone.
type PreviewInput struct {
	Condition     *conditiontree.Segment
	Channel       *channels.Channel
	IntegrationID *int64
}

// PreviewResult is what a preview counts: the Contacts the Condition matches, the ones it leaves
// out because their engagement is unknown (ADR 0123), and, when a Channel was asked for, how many
// of the matching ones a Broadcast there would take now (eligibility.Module.BroadcastAudience).
type PreviewResult struct {
	Count int
	// UnknownEngagement is the Contacts left out because a count of their opens or clicks is
	// unknown: their Destination on its Channel is not tracked now (condition.Module.LeftOut).
	UnknownEngagement int
	Recipients        *int
}

// Preview counts the Contacts in the Workspace that match an unsaved Condition, the ones it leaves
// out for unknown engagement, and the recipients of a Broadcast on the Channel the input names. A
// Condition that does not compile is ErrInvalidCondition.
func (s *Module) Preview(ctx context.Context, ws *ent.Scoped, in PreviewInput) (PreviewResult, error) {
	cond := conditiontree.Segment{Group: conditiontree.Empty()}
	if in.Condition != nil {
		cond = *in.Condition
	}
	pred, err := s.conditions.Compile(ctx, ws, cond)
	if err != nil {
		return PreviewResult{}, ErrInvalidCondition.With(err.Error())
	}
	count, err := ws.Contact().Query().Where(pred).Count(ctx)
	if err != nil {
		return PreviewResult{}, err
	}
	res := PreviewResult{Count: count}
	if condition.CanBeUnknown(cond.Group) {
		leftOut, err := s.conditions.LeftOut(ctx, ws, cond)
		if err != nil {
			return PreviewResult{}, ErrInvalidCondition.With(err.Error())
		}
		res.UnknownEngagement, err = ws.Contact().Query().Where(leftOut).Count(ctx)
		if err != nil {
			return PreviewResult{}, err
		}
	}
	if in.Channel == nil {
		return res, nil
	}
	reached, err := s.eligibility.BroadcastAudience(ctx, ws, *in.Channel, in.IntegrationID)
	if err != nil {
		return PreviewResult{}, err
	}
	recipients, err := ws.Contact().Query().Where(reached, pred).Count(ctx)
	if err != nil {
		return PreviewResult{}, err
	}
	res.Recipients = &recipients
	return res, nil
}

// Count counts the Contacts matching a stored Segment, evaluated live, and the ones it leaves out
// because their engagement is unknown, as Preview does.
func (s *Module) Count(ctx context.Context, ws *ent.Scoped, id int64) (PreviewResult, error) {
	seg, err := ws.Segment().Get(ctx, id)
	if ent.IsNotFound(err) {
		return PreviewResult{}, ErrNotFound
	}
	if err != nil {
		return PreviewResult{}, err
	}
	return s.Preview(ctx, ws, PreviewInput{Condition: &seg.Condition})
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
	pred, err := s.conditions.Compile(ctx, ws, seg.Condition)
	if err != nil {
		return pagination.Page[*ent.Contact]{}, ErrInvalidCondition.With(err.Error())
	}
	return pagination.List(ctx, p, ws.Contact().Query().Where(pred).
		WithTags(func(q *ent.TagQuery) { q.Order(ent.Asc(tag.FieldName)) }).
		Order(ent.Asc(contact.FieldID)))
}

// List returns one page of the Workspace's Segments, ascending by id: the kept ones, or with
// archived the archived ones.
func (s *Module) List(ctx context.Context, ws *ent.Scoped, p pagination.Params, archived bool) (pagination.Page[*ent.Segment], error) {
	q := ws.Segment().Query()
	if archived {
		q = ws.Segment().QueryArchived()
	}
	return pagination.List(ctx, p, q.Order(ent.Asc(segment.FieldID)))
}

// Archive puts a Segment aside (ADR 0100): it leaves the lists and new work, and Unarchive brings
// it back. Only Live use would refuse it (ADR 0099), and none needs a Segment: a draft Broadcast
// is not Live use (scheduling it is then refused), and a Broadcast past draft holds a copy of the
// Condition (ADR 0065) and keeps its reference only as provenance. A missing Segment is
// ErrNotFound; archiving an archived one changes nothing. It returns the Segment. The archive
// holds the Workspace's lock (ADR 0099), so a draft being scheduled meanwhile either took its copy
// first or finds the Segment archived.
func (s *Module) Archive(ctx context.Context, ws *ent.Scoped, id int64) (*ent.Segment, error) {
	err := s.bus.WithinWorkspaceLockTx(ctx, ws, func(wl *ent.WorkspaceLocked, _ events.Publisher) error {
		seg, err := wl.Segment().Get(ctx, id)
		if ent.IsNotFound(err) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if seg.ArchivedAt != nil {
			return nil
		}
		return wl.Segment().Archive(ctx, id, s.now())
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, ws, id)
}

// Unarchive brings an archived Segment back to the lists and returns it. Unarchiving one that is not
// archived changes nothing; a missing Segment is ErrNotFound.
func (s *Module) Unarchive(ctx context.Context, ws *ent.Scoped, id int64) (*ent.Segment, error) {
	err := ws.Segment().Unarchive(ctx, id)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, ws, id)
}
