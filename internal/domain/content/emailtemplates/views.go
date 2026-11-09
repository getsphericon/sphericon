package emailtemplates

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/kernel/emailrender"
)

// Saved is a Template a save stored, with the lint warnings of its content (ADR 0040): advice for
// the author, which never block the save.
type Saved struct {
	Template *ent.EmailTemplate
	Warnings []emailrender.Finding
}

// Views are the Module's saves answered with the lint of what they stored: the lint reads the Brand
// kit, which the row alone does not, so only a surface whose contract shows the warnings calls
// Views, and one that shows the Template calls the Module.
type Views struct {
	m *Module
}

// NewViews builds the views over the saves of m.
func NewViews(m *Module) *Views {
	return &Views{m: m}
}

// Create stores a Template (Module.Create) and returns it with the lint warnings of its content.
func (v *Views) Create(ctx context.Context, s *ent.Scoped, in CreateInput) (Saved, error) {
	tpl, err := v.m.Create(ctx, s, in)
	if err != nil {
		return Saved{}, err
	}
	return v.saved(ctx, s, tpl, in.Body)
}

// Update applies the given fields (Module.Update) and returns the Template with the lint warnings
// of its content.
func (v *Views) Update(ctx context.Context, s *ent.Scoped, id int64, in UpdateInput) (Saved, error) {
	tpl, err := v.m.Update(ctx, s, id, in)
	if err != nil {
		return Saved{}, err
	}
	return v.saved(ctx, s, tpl, in.Body.Ptr())
}

// saved lints what a save stored: the body as the write gave it, before the sanitizer, so a
// construct the save removed is still named; otherwise the stored body (a document's rendered HTML,
// or a write of the subject alone). A message that cannot render was refused by the save.
func (v *Views) saved(ctx context.Context, s *ent.Scoped, tpl *ent.EmailTemplate, written *string) (Saved, error) {
	body := tpl.Body
	if written != nil {
		body = *written
	}
	report, err := v.m.lint(ctx, s, tpl.Subject, body)
	if err != nil {
		return Saved{}, err
	}
	return Saved{Template: tpl, Warnings: report.Warnings}, nil
}
