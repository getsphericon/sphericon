package segments

import (
	"context"
	"errors"
	"fmt"

	"github.com/mokevnin/sphericon/ent"
	"github.com/mokevnin/sphericon/ent/segment"
	"github.com/mokevnin/sphericon/internal/pagination"
)

// Domain errors returned by Module. Callers match with errors.Is.
var (
	// ErrInvalidDefinition: the rule definition does not parse or does not fit the
	// Contact schema. The wrapped message is safe to show to the author.
	ErrInvalidDefinition = errors.New("segments: invalid definition")
	// ErrNotFound: no such Segment in this Workspace.
	ErrNotFound = errors.New("segments: segment not found")
)

// Module is the one place a Segment is written, validated, previewed and counted
// (ADR 0016). Create and update share one validation rule set; handlers stay thin
// adapters (scope check, call, map). Membership is the rule alone — Send-eligibility
// is subtracted only at send, never folded into a count (ADR 0001).
type Module struct{}

// New builds the Segments module. It holds no client: every call takes the
// Workspace's scoped client (ADR 0017).
func New() *Module { return &Module{} }

// CreateInput is a new Segment. A nil or empty Definition means "no rules yet".
type CreateInput struct {
	Name       string
	Definition *string
}

// UpdateInput changes a Segment; nil fields are left as they are.
type UpdateInput struct {
	Name       *string
	Definition *string
}

// Validate checks a definition against the Contact attributes and the Workspace's Custom
// field catalogue. An empty definition is valid. This is the single rule set behind
// Create, Update, Preview and Count.
func (s *Module) Validate(ctx context.Context, ws *ent.Scoped, def string) error {
	if def == "" {
		return nil
	}
	if _, err := ContactPredicate(ctx, ws, def); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidDefinition, err.Error())
	}
	return nil
}

// Create validates the definition and stores the Segment.
func (s *Module) Create(ctx context.Context, ws *ent.Scoped, in CreateInput) (*ent.Segment, error) {
	if in.Definition != nil {
		if err := s.Validate(ctx, ws, *in.Definition); err != nil {
			return nil, err
		}
	}
	return ws.Segment().Create().
		SetName(in.Name).
		SetNillableDefinition(in.Definition).
		Save(ctx)
}

// Update validates a changed definition exactly as Create does, then applies the change.
func (s *Module) Update(ctx context.Context, ws *ent.Scoped, id int64, in UpdateInput) (*ent.Segment, error) {
	if in.Definition != nil {
		if err := s.Validate(ctx, ws, *in.Definition); err != nil {
			return nil, err
		}
	}
	q := ws.Segment().UpdateOneID(id).
		SetNillableName(in.Name).
		SetNillableDefinition(in.Definition)
	seg, err := q.Save(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	return seg, err
}

// Preview counts the Contacts in the Workspace that match an unsaved definition.
func (s *Module) Preview(ctx context.Context, ws *ent.Scoped, def string) (int, error) {
	pred, err := ContactPredicate(ctx, ws, def)
	if err != nil {
		return 0, fmt.Errorf("%w: %s", ErrInvalidDefinition, err.Error())
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
	def := ""
	if seg.Definition != nil {
		def = *seg.Definition
	}
	return s.Preview(ctx, ws, def)
}

// List returns one page of the Workspace's Segments, ascending by id.
func (s *Module) List(ctx context.Context, ws *ent.Scoped, p pagination.Params) (pagination.Page[*ent.Segment], error) {
	return pagination.List(ctx, p,
		func(ctx context.Context) (int, error) { return ws.Segment().Query().Count(ctx) },
		func(ctx context.Context, limit, offset int) ([]*ent.Segment, error) {
			return ws.Segment().Query().Order(ent.Asc(segment.FieldID)).Limit(limit).Offset(offset).All(ctx)
		})
}
