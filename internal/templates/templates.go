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
)

// Domain errors. Callers match with errors.Is.
var (
	// ErrNotFound: no such Template in this Workspace.
	ErrNotFound = errors.New("templates: template not found")
	// ErrBlankName: the Template name is blank.
	ErrBlankName = errors.New("templates: name must not be empty")
	// ErrBodyAndDoc: a write carries both a body and a document. A client writes one of them: the
	// document (the server builds the body from it) or the body (which clears the document).
	ErrBodyAndDoc = errors.New("templates: write a body or a doc, not both")
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
	c, err := content(ctx, in.Subject, in.Body, in.Doc)
	if err != nil {
		return nil, err
	}
	create := s.EmailTemplate().Create().
		SetName(in.Name).
		SetNillableSubject(in.Subject).
		SetNillableBody(c.body)
	if c.authored != nil {
		create.SetDoc(in.Doc).SetEditorVersion(c.authored.EditorVersion)
	}
	tpl, err := create.Save(ctx)
	return tpl, mapWriteError(err)
}

// Update applies the given fields. A blank name is ErrBlankName.
func (m *Module) Update(ctx context.Context, s *ent.Scoped, id int64, in UpdateInput) (*ent.EmailTemplate, error) {
	c, err := content(ctx, in.Subject, in.Body, in.Doc)
	if err != nil {
		return nil, err
	}
	update := s.EmailTemplate().UpdateOneID(id).
		SetNillableName(in.Name).
		SetNillableSubject(in.Subject).
		SetNillableBody(c.body)
	switch {
	case c.authored != nil:
		update.SetDoc(in.Doc).SetEditorVersion(c.authored.EditorVersion)
	case c.body != nil:
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

// written is what a save stores of the message content.
type written struct {
	// body is the body to store; nil when the write names none.
	body *string
	// authored is set when the content is a document: its body is body.
	authored *emailrender.Authored
}

// content resolves the body of a write. A document is rendered to it (the one render of the
// save, never on the send path); a body is validated and sanitized; neither leaves it alone.
func content(ctx context.Context, subject, body *string, doc json.RawMessage) (written, error) {
	if len(doc) == 0 {
		b, err := emailrender.PrepareFields(subject, body)
		return written{body: b}, err
	}
	if body != nil {
		return written{}, ErrBodyAndDoc
	}
	authored, err := emailrender.FromDocument(ctx, subject, doc)
	if err != nil {
		return written{}, err
	}
	return written{body: &authored.Body, authored: &authored}, nil
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
