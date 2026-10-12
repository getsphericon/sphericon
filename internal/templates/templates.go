// Package templates is the one place an email Template is listed, read, written
// and deleted. Handlers on /site and /api stay thin adapters: scope check, call,
// map. Transactional send reads a Template through Get, by reference (ADR 0005);
// copy-at-author-time (ADR 0003) is marketing only and lives elsewhere.
package templates

import (
	"context"
	"errors"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/emailtemplate"
	"github.com/getsphericon/sphericon/internal/pagination"
)

// Domain errors. Callers match with errors.Is.
var (
	// ErrNotFound: no such Template in this Workspace.
	ErrNotFound = errors.New("templates: template not found")
	// ErrBlankName: the Template name is blank.
	ErrBlankName = errors.New("templates: name must not be empty")
)

// Module holds no state: every call receives the Workspace-scoped client (ADR 0017).
type Module struct{}

// New builds the templates module.
func New() *Module { return &Module{} }

// CreateInput is a new Template; nil Subject and Body keep their defaults.
type CreateInput struct {
	Name    string
	Subject *string
	Body    *string
}

// UpdateInput changes a Template; nil fields are left as they are.
type UpdateInput struct {
	Name    *string
	Subject *string
	Body    *string
}

// List returns a page of the Workspace's Templates, ascending by id (a catalogue).
func (m *Module) List(ctx context.Context, s *ent.Scoped, p pagination.Params) (pagination.Page[*ent.EmailTemplate], error) {
	return pagination.List(ctx, p,
		func(ctx context.Context) (int, error) { return s.EmailTemplate().Query().Count(ctx) },
		func(ctx context.Context, limit, offset int) ([]*ent.EmailTemplate, error) {
			return s.EmailTemplate().Query().Order(ent.Asc(emailtemplate.FieldID)).Limit(limit).Offset(offset).All(ctx)
		})
}

// Get returns one Template of the Workspace.
func (m *Module) Get(ctx context.Context, s *ent.Scoped, id int64) (*ent.EmailTemplate, error) {
	tpl, err := s.EmailTemplate().Get(ctx, id)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	return tpl, err
}

// Create stores a Template. A blank name is ErrBlankName.
func (m *Module) Create(ctx context.Context, s *ent.Scoped, in CreateInput) (*ent.EmailTemplate, error) {
	tpl, err := s.EmailTemplate().Create().
		SetName(in.Name).
		SetNillableSubject(in.Subject).
		SetNillableBody(in.Body).
		Save(ctx)
	return tpl, mapWriteError(err)
}

// Update applies the given fields. A blank name is ErrBlankName.
func (m *Module) Update(ctx context.Context, s *ent.Scoped, id int64, in UpdateInput) (*ent.EmailTemplate, error) {
	tpl, err := s.EmailTemplate().UpdateOneID(id).
		SetNillableName(in.Name).
		SetNillableSubject(in.Subject).
		SetNillableBody(in.Body).
		Save(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	return tpl, mapWriteError(err)
}

// Delete removes a Template.
func (m *Module) Delete(ctx context.Context, s *ent.Scoped, id int64) error {
	err := s.EmailTemplate().DeleteOneID(id).Exec(ctx)
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	return err
}

// mapWriteError relabels only a validation failure of the name field; any other
// validation error passes through unchanged.
func mapWriteError(err error) error {
	var ve *ent.ValidationError
	if errors.As(err, &ve) && ve.Name == emailtemplate.FieldName {
		return ErrBlankName
	}
	return err
}
