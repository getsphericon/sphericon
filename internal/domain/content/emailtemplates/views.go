package emailtemplates

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/kernel/emailrender"
)

// Saved is a Template as stored, with the lint warnings of its content (ADR 0040): advice for the
// author, which never block a save.
type Saved struct {
	Template *ent.EmailTemplate
	Warnings []emailrender.Finding
}

// Views are the Module's single read and saves answered with the lint of what is stored: the lint reads the Brand
// kit, which the row alone does not, so only a surface whose contract shows the warnings calls
// Views, and one that shows the Template calls the Module.
type Views struct {
	m *Module
}

// NewViews builds the views over the saves of m.
func NewViews(m *Module) *Views {
	return &Views{m: m}
}

// Get returns one Template (Module.Get) with the lint warnings of its content: what its editor shows
// before anything is saved.
func (v *Views) Get(ctx context.Context, s *ent.Scoped, id int64) (Saved, error) {
	tpl, err := v.m.Get(ctx, s, id)
	if err != nil {
		return Saved{}, err
	}
	return v.saved(ctx, s, tpl)
}

// Create stores a Template (Module.Create) and returns it with the lint warnings of its content.
func (v *Views) Create(ctx context.Context, s *ent.Scoped, in CreateInput) (Saved, error) {
	tpl, err := v.m.Create(ctx, s, in)
	if err != nil {
		return Saved{}, err
	}
	return v.saved(ctx, s, tpl)
}

// Update applies the given fields (Module.Update) and returns the Template with the lint warnings
// of its content.
func (v *Views) Update(ctx context.Context, s *ent.Scoped, id int64, in UpdateInput) (Saved, error) {
	tpl, err := v.m.Update(ctx, s, id, in)
	if err != nil {
		return Saved{}, err
	}
	return v.saved(ctx, s, tpl)
}

// saved lints what is stored: the body as it was written, or a document's rendered HTML. A message
// that cannot render was refused by its save.
func (v *Views) saved(ctx context.Context, s *ent.Scoped, tpl *ent.EmailTemplate) (Saved, error) {
	report, err := v.m.lint(ctx, s, tpl.Subject, tpl.Body)
	if err != nil {
		return Saved{}, err
	}
	return Saved{Template: tpl, Warnings: report.Warnings}, nil
}
