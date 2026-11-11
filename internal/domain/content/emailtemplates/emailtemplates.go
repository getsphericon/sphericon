// Package emailtemplates is the one place an email Template is listed, read, written
// and deleted. Handlers on /site and /api stay thin adapters: scope check, call,
// map. Transactional send reads a Template through Get, by reference (ADR 0005);
// copy-at-author-time (ADR 0003) is marketing only and lives elsewhere.
package emailtemplates

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/emailtemplate"
	"github.com/getsphericon/sphericon/internal/kernel/emaildoc"
	"github.com/getsphericon/sphericon/internal/kernel/emailrender"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// Domain errors. Callers match with errors.Is.
var (
	// ErrNotFound: no such Template in this Workspace.
	ErrNotFound = validation.NotFound("templates: template not found", i18n.ErrorsTemplateNotFound)
	// ErrRevisionChanged is an update made on a revision the Template has moved past (ADR 0119):
	// someone wrote it since the client read it. Nothing is written.
	ErrRevisionChanged = validation.RevisionChanged("templates: template changed since it was read", i18n.ErrorsTemplateRevisionChanged)

	// ErrBodyAndDoc: a write carries both a body and a document. A client writes one of them: the
	// document (the server builds the body from it) or the body (which clears the document).
	ErrBodyAndDoc = validation.Refuse("templates: write a body or a doc, not both", i18n.ErrorsBodyAndDoc)
	// ErrLintBodyOrDoc: a lint sends both a body and a document, or neither. It reads exactly one.
	ErrLintBodyOrDoc = validation.Refuse("templates: lint a body or a doc", i18n.ErrorsLintBodyOrDoc)
)

// Module holds no Workspace state: every call receives the Workspace-scoped client (ADR 0017).
// It holds the document renderer a save with a document runs, and the Brand kit a lint compares a
// message to: Lint, and the lint of a save in Views.
type Module struct {
	renderer *emaildoc.Renderer
	brands   Brands
}

// Brands reads the Workspace's brand the lint compares a message to: nil when it has saved no Brand
// kit (brandkits.Brands). It is given rather than imported: the Brand kit writes its starter
// Templates through this module.
type Brands interface {
	Brand(ctx context.Context, s *ent.Scoped) (*emailrender.Brand, error)
}

// New builds the templates module.
func New(renderer *emaildoc.Renderer, brands Brands) *Module {
	return &Module{renderer: renderer, brands: brands}
}

// Prepare renders a document to the body a save stores (emailrender.FromDocument), for a caller
// that renders before it opens a transaction (CreatePrepared) or renders without saving.
func (m *Module) Prepare(ctx context.Context, subject *string, doc []byte) (emailrender.Authored, error) {
	return emailrender.FromDocument(ctx, m.renderer, subject, doc)
}

// Lint is the lint of ADR 0040 without a save, against the Workspace's Brand kit: of body as it is,
// or of doc rendered as a save renders it (Prepare), which may set the subject. Exactly one of body
// and doc is sent, nil when absent (ErrLintBodyOrDoc). A message that cannot render, a document
// included, is the report's Error; its warnings are advice for the author.
func (m *Module) Lint(ctx context.Context, s *ent.Scoped, subject string, body *string, doc []byte) (emailrender.Report, error) {
	if (body == nil) == (doc == nil) {
		return emailrender.Report{}, ErrLintBodyOrDoc
	}
	if body == nil {
		authored, err := m.Prepare(ctx, &subject, doc)
		if errors.Is(err, emailrender.ErrInvalidContent) {
			return emailrender.Report{Error: err}, nil
		}
		if err != nil {
			return emailrender.Report{}, err
		}
		body = &authored.Body
	}
	return m.lint(ctx, s, subject, *body)
}

// lint runs emailrender.Lint over a message against the Workspace's Brand kit.
func (m *Module) lint(ctx context.Context, s *ent.Scoped, subject, body string) (emailrender.Report, error) {
	brand, err := m.brands.Brand(ctx, s)
	if err != nil {
		return emailrender.Report{}, err
	}
	return emailrender.Lint(subject, body, brand), nil
}

// CreateInput is a new Template; nil Subject and Body keep their defaults.
type CreateInput struct {
	Name    string
	Subject *string
	Body    *string
	// Doc is the editor's JSON document, empty when the Template is written as HTML. The body is
	// built from it in the same save (ADR 0040).
	Doc json.RawMessage
}

// UpdateInput is a patch of a Template: a field that keeps leaves the stored value. A Doc is
// rendered to the body in the same save and stored with it; a Body alone clears the stored
// document, so the two never disagree. None of the fields can be cleared.
type UpdateInput struct {
	// Revision is the Template's revision the edit was made on (ADR 0119).
	Revision int64
	Name     patch.Field[string]
	Subject  patch.Field[string]
	Body     patch.Field[string]
	Doc      patch.Field[json.RawMessage]
}

// List returns a page of the Workspace's Templates, ascending by id (a catalogue).
func (m *Module) List(ctx context.Context, s *ent.Scoped, p pagination.Params) (pagination.Page[*ent.EmailTemplate], error) {
	return pagination.List(ctx, p, s.EmailTemplate().Query().Order(ent.Asc(emailtemplate.FieldID)))
}

// Get returns one Template of the Workspace.
func (m *Module) Get(ctx context.Context, s *ent.Scoped, id int64) (*ent.EmailTemplate, error) {
	tpl, err := s.EmailTemplate().Get(ctx, id)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	return tpl, err
}

// Create stores a Template and returns it. The name is held to the name rule (validation.Name); a
// subject, body or document that cannot render is emailrender.ErrInvalidContent; the body is
// stored as written.
func (m *Module) Create(ctx context.Context, s *ent.Scoped, in CreateInput) (*ent.EmailTemplate, error) {
	name, err := validation.Name(in.Name)
	if err != nil {
		return nil, err
	}
	in.Name = name
	c, err := emailrender.ResolveContent(ctx, m.renderer, in.Subject, in.Body, in.Doc, ErrBodyAndDoc)
	if err != nil {
		return nil, err
	}
	return create(ctx, s, in.Name, in.Subject, c, in.Doc)
}

// CreatePrepared stores a Template of a document already rendered by Prepare, for a caller that
// renders before it opens a transaction so that no database connection is held through the render
// (the Brand kit's starters). It is not linted: the caller wrote the document.
func (m *Module) CreatePrepared(ctx context.Context, s *ent.Scoped, name, subject string, doc json.RawMessage, authored emailrender.Authored) (*ent.EmailTemplate, error) {
	name, err := validation.Name(name)
	if err != nil {
		return nil, err
	}
	return create(ctx, s, name, &subject, emailrender.Content{Body: &authored.Body, Authored: &authored}, doc)
}

func create(ctx context.Context, s *ent.Scoped, name string, subject *string, c emailrender.Content, doc json.RawMessage) (*ent.EmailTemplate, error) {
	q := s.EmailTemplate().Create().
		SetName(name).
		SetNillableSubject(subject).
		SetNillableBody(c.Body)
	if c.Authored != nil {
		q.SetDoc(doc).SetEditorVersion(c.Authored.EditorVersion)
	}
	tpl, err := q.Save(ctx)
	return tpl, err
}

// Update applies the given fields and returns the Template, conditional on the revision the edit
// was made on (ADR 0119): ErrRevisionChanged when the Template has moved past it. A new name is
// held to the name rule (validation.Name).
func (m *Module) Update(ctx context.Context, s *ent.Scoped, id int64, in UpdateInput) (*ent.EmailTemplate, error) {
	name, err := validation.NamePatch(in.Name)
	if err != nil {
		return nil, err
	}
	in.Name = name
	doc, _ := in.Doc.Get()
	c, err := emailrender.ResolveContent(ctx, m.renderer, in.Subject.Ptr(), in.Body.Ptr(), doc, ErrBodyAndDoc)
	if err != nil {
		return nil, err
	}
	update := s.EmailTemplate().UpdateOneID(id).
		Where(emailtemplate.Revision(in.Revision)).
		SetNillableName(in.Name.Ptr()).
		SetNillableSubject(in.Subject.Ptr()).
		SetNillableBody(c.Body)
	switch {
	case c.Authored != nil:
		update.SetDoc(doc).SetEditorVersion(c.Authored.EditorVersion)
	case c.Body != nil:
		update.ClearDoc().ClearEditorVersion()
	}
	tpl, err := update.Save(ctx)
	if ent.IsNotFound(err) {
		return nil, missOn(ctx, s, id)
	}
	return tpl, err
}

// missOn tells why an update conditional on its revision found no row: the Template is gone, or
// it is there at another revision.
func missOn(ctx context.Context, s *ent.Scoped, id int64) error {
	exists, err := s.EmailTemplate().Query().Where(emailtemplate.ID(id)).Exist(ctx)
	if err != nil {
		return err
	}
	if exists {
		return ErrRevisionChanged
	}
	return ErrNotFound
}

// Delete removes a Template.
func (m *Module) Delete(ctx context.Context, s *ent.Scoped, id int64) error {
	err := s.EmailTemplate().DeleteOneID(id).Exec(ctx)
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	return err
}
