// Package templates is the one place an email Template is listed, read, written
// and deleted. Handlers on /site and /api stay thin adapters: scope check, call,
// map. Transactional send reads a Template through Get, by reference (ADR 0005);
// copy-at-author-time (ADR 0003) is marketing only and lives elsewhere.
package templates

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/emailtemplate"
	"github.com/getsphericon/sphericon/internal/emailrender"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/validation"
)

// Domain errors. Callers match with errors.Is.
var (
	// ErrNotFound: no such Template in this Workspace.
	ErrNotFound = validation.Refuse("templates: template not found", "errors.template_not_found")
	// ErrBlankName: the Template name is blank.
	ErrBlankName = validation.Refuse("templates: name must not be empty", "errors.name_empty")
	// ErrBodyAndDoc: a write carries both a body and a document. A client writes one of them: the
	// document (the server builds the body from it) or the body (which clears the document).
	ErrBodyAndDoc = validation.Refuse("templates: write a body or a doc, not both", "errors.body_and_doc")
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
	// Doc is the editor's JSON document, empty when the Template is written as HTML. The body is
	// built from it in the same save (ADR 0040).
	Doc json.RawMessage
	// Prepared is Doc already rendered by emailrender.FromDocument, for a caller that renders
	// before it opens a transaction so that no database connection is held through the render.
	// Nil renders Doc here; a Body is never combined with it.
	Prepared *emailrender.Authored
}

// UpdateInput changes a Template; nil fields are left as they are. A Doc is rendered to the
// body in the same save and stored with it; a Body alone clears the stored document, so the two
// never disagree.
type UpdateInput struct {
	Name    *string
	Subject *string
	Body    *string
	Doc     json.RawMessage
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

// Create stores a Template. A blank name is ErrBlankName; a subject, body or document that
// cannot render is emailrender.ErrInvalidContent; the body is stored sanitized.
func (m *Module) Create(ctx context.Context, s *ent.Scoped, in CreateInput) (*ent.EmailTemplate, error) {
	var c emailrender.Content
	if in.Prepared != nil && len(in.Doc) > 0 && in.Body == nil {
		c = emailrender.Content{Body: &in.Prepared.Body, Authored: in.Prepared}
	} else {
		var err error
		if c, err = emailrender.ResolveContent(ctx, in.Subject, in.Body, in.Doc, ErrBodyAndDoc); err != nil {
			return nil, err
		}
	}
	create := s.EmailTemplate().Create().
		SetName(in.Name).
		SetNillableSubject(in.Subject).
		SetNillableBody(c.Body)
	if c.Authored != nil {
		create.SetDoc(in.Doc).SetEditorVersion(c.Authored.EditorVersion)
	}
	tpl, err := create.Save(ctx)
	return tpl, mapWriteError(err)
}

// Update applies the given fields. A blank name is ErrBlankName.
func (m *Module) Update(ctx context.Context, s *ent.Scoped, id int64, in UpdateInput) (*ent.EmailTemplate, error) {
	c, err := emailrender.ResolveContent(ctx, in.Subject, in.Body, in.Doc, ErrBodyAndDoc)
	if err != nil {
		return nil, err
	}
	update := s.EmailTemplate().UpdateOneID(id).
		SetNillableName(in.Name).
		SetNillableSubject(in.Subject).
		SetNillableBody(c.Body)
	switch {
	case c.Authored != nil:
		update.SetDoc(in.Doc).SetEditorVersion(c.Authored.EditorVersion)
	case c.Body != nil:
		update.ClearDoc().ClearEditorVersion()
	}
	tpl, err := update.Save(ctx)
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
