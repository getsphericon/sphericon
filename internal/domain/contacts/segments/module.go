// Package segments is the Segment domain module: named Conditions on Contacts, with the rows they
// select and the live work that uses them.
package segments

import (
	"context"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/segment"
	"github.com/getsphericon/sphericon/ent/tag"
	"github.com/getsphericon/sphericon/internal/domain/contacts/condition"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
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
)

// Module is the one place a Segment is written, validated, previewed and counted
// (ADR 0016). Create and update share one validation rule set; handlers stay thin
// adapters (scope check, call, map). Membership is the rule alone — Send-eligibility
// is subtracted only at send, never folded into a count (ADR 0001).
type Module struct {
	bus *events.Bus
	// now is the Instance clock a relative date of a rule counts back from.
	now func() time.Time
}

// New builds the Segments module. It holds no client: every call takes the
// Workspace's scoped client (ADR 0017); the bus runs a save and the archive each in one
// transaction under the Workspace's lock (ADR 0099). now is the Instance clock.
func New(bus *events.Bus, now func() time.Time) *Module { return &Module{bus: bus, now: now} }

// CreateInput is a new Segment. A nil or empty Condition means "no rules yet".
type CreateInput struct {
	Name      string
	Condition *string
}

// UpdateInput is a patch of a Segment: a field that keeps leaves the stored value. Neither field
// can be cleared; an empty Condition matches all contacts.
type UpdateInput struct {
	Name      patch.Field[string]
	Condition patch.Field[string]
}

// Validate checks a condition against the Contact attributes and the Workspace's Custom
// field catalogue. An empty condition is valid. This is the single rule set behind
// Create, Update, Preview and Count.
func (s *Module) Validate(ctx context.Context, ws *ent.Scoped, cond string) error {
	if cond == "" {
		return nil
	}
	if _, err := condition.ContactPredicate(ctx, ws, cond, s.now()); err != nil {
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
	if in.Condition != nil {
		if err := s.Validate(ctx, wl.Scope(), *in.Condition); err != nil {
			return nil, err
		}
	}
	return wl.Segment().Create().
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

// Update validates a changed condition exactly as Create does, then applies the change, under the
// Workspace's lock as Create, for the references the condition names. An archived Segment is
// ErrArchived: it is read-only until it is unarchived (ADR 0100), so the change is an update
// conditioned on its not being archived (ADR 0109).
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
// 0108), conditioned on the Segment's not being archived (ErrArchived).
func (s *Module) updateIn(ctx context.Context, wl *ent.WorkspaceLocked, id int64, in UpdateInput) (*ent.Segment, error) {
	if cond, ok := in.Condition.Get(); ok {
		if err := s.Validate(ctx, wl.Scope(), cond); err != nil {
			return nil, err
		}
	}
	q := wl.Segment().UpdateOneID(id).Where(segment.ArchivedAtIsNil())
	patch.ApplySet(in.Name, q.SetName)
	patch.ApplySet(in.Condition, q.SetCondition)
	seg, err := q.Save(ctx)
	if !ent.IsNotFound(err) {
		return seg, err
	}
	// The update missed: the Segment is archived, or the Workspace has none of this id.
	if _, err := wl.Segment().Get(ctx, id); err != nil {
		return nil, err
	}
	return nil, ErrArchived
}

// PreviewInput is an unsaved Condition to preview, nil for every Contact, and the Channel of a
// Broadcast the audience is asked for, with the Integration it routes through (nil: the Channel's
// default). Without a Channel the preview counts the match alone.
type PreviewInput struct {
	Condition     *string
	Channel       *channels.Channel
	IntegrationID *int64
}

// PreviewResult is what a preview counts: the Contacts the Condition matches, and, when a Channel
// was asked for, how many of them a Broadcast there would take now (eligibility.BroadcastAudience).
type PreviewResult struct {
	Count      int
	Recipients *int
}

// Preview counts the Contacts in the Workspace that match an unsaved Condition, and the recipients
// of a Broadcast on the Channel the input names. A Condition that does not compile is
// ErrInvalidCondition.
func (s *Module) Preview(ctx context.Context, ws *ent.Scoped, in PreviewInput) (PreviewResult, error) {
	pred, err := condition.ContactPredicate(ctx, ws, lo.FromPtr(in.Condition), s.now())
	if err != nil {
		return PreviewResult{}, ErrInvalidCondition.With(err.Error())
	}
	count, err := ws.Contact().Query().Where(pred).Count(ctx)
	if err != nil {
		return PreviewResult{}, err
	}
	res := PreviewResult{Count: count}
	if in.Channel == nil {
		return res, nil
	}
	reached, err := eligibility.BroadcastAudience(ctx, ws, *in.Channel, in.IntegrationID)
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

// Count counts the Contacts matching a stored Segment, evaluated live.
func (s *Module) Count(ctx context.Context, ws *ent.Scoped, id int64) (int, error) {
	seg, err := ws.Segment().Get(ctx, id)
	if ent.IsNotFound(err) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	res, err := s.Preview(ctx, ws, PreviewInput{Condition: seg.Condition})
	return res.Count, err
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
	pred, err := condition.ContactPredicate(ctx, ws, cond, s.now())
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
