package automations

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/content/emailtemplates"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/emailrender"
)

// StepWarnings are the lint warnings of one email step (ADR 0040): advice for the author, which
// never block a save or Publish.
type StepWarnings struct {
	Step     automationstep.ID
	Warnings []emailrender.Finding
}

// Linted is an Automation as its editor shows it: the row, read with its revenue on the single
// read, and the lint warnings of each email step of the version Publish arms.
type Linted struct {
	WithRevenue

	Warnings []StepWarnings
}

// Views are the Module's reads and saves answered with the lint of the email steps: the lint reads
// the Brand kit and renders each step, which the row alone does not, so only a surface whose
// contract shows the warnings calls Views, and one that shows the Automation calls the Module.
type Views struct {
	m *Module
	// lint is the one lint of a message without a save, against the Workspace's Brand kit.
	lint *emailtemplates.Module
}

// NewViews builds the views over the reads and saves of m, linting through lint.
func NewViews(m *Module, lint *emailtemplates.Module) *Views {
	return &Views{m: m, lint: lint}
}

// Get returns one Automation of the Workspace with its revenue and the warnings of its steps.
func (v *Views) Get(ctx context.Context, s *ent.Scoped, id int64) (Linted, error) {
	a, err := v.m.GetWithRevenue(ctx, s, id)
	if err != nil {
		return Linted{}, err
	}
	return v.linted(ctx, s, a)
}

// Create stores an Automation (Module.Create) and returns it with the warnings of its steps.
func (v *Views) Create(ctx context.Context, s *ent.Scoped, in CreateInput) (Linted, error) {
	a, err := v.m.Create(ctx, s, in)
	if err != nil {
		return Linted{}, err
	}
	return v.linted(ctx, s, WithRevenue{Automation: a})
}

// Update applies an edit (Module.Update) and returns the Automation with the warnings of its steps.
func (v *Views) Update(ctx context.Context, s *ent.Scoped, id int64, in UpdateInput) (Linted, error) {
	a, err := v.m.Update(ctx, s, id, in)
	if err != nil {
		return Linted{}, err
	}
	return v.linted(ctx, s, WithRevenue{Automation: a})
}

// linted lints each email step of the draft, else the live version, at any depth of its tree: the
// version Publish arms. A step's body is stored rendered, so it is linted as it was written or as
// its document rendered; a step that cannot render was refused by the save.
func (v *Views) linted(ctx context.Context, s *ent.Scoped, a WithRevenue) (Linted, error) {
	out := Linted{WithRevenue: a}
	version := a.Automation.Edges.DraftVersion
	if version == nil {
		version = a.Automation.Edges.LiveVersion
	}
	if version == nil {
		return out, nil
	}
	for _, st := range automationstep.All(version.Steps) {
		email, ok := st.Payload.(automationstep.Email)
		if !ok {
			continue
		}
		report, err := v.lint.Lint(ctx, s, emailtemplates.LintInput{Subject: email.Subject, Body: &email.Body})
		if err != nil {
			return Linted{}, err
		}
		if len(report.Warnings) > 0 {
			out.Warnings = append(out.Warnings, StepWarnings{Step: st.ID, Warnings: report.Warnings})
		}
	}
	return out, nil
}
