package automations

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/automationsteprecord"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
)

// EnrollmentsFilter names the Automation whose Enrollments are listed and, optionally, the one
// status they are in (empty for all).
type EnrollmentsFilter struct {
	AutomationID int64
	Status       string
}

// Enrollments returns one page of an Automation's Enrollments, newest first, optionally
// only those in one status. ErrNotFound when the Workspace has no such Automation.
func (m *Module) Enrollments(ctx context.Context, s *ent.Scoped, p pagination.Params, f EnrollmentsFilter) (pagination.Page[*ent.AutomationRun], error) {
	if _, err := s.Automation().Get(ctx, f.AutomationID); err != nil {
		return pagination.Page[*ent.AutomationRun]{}, db.NotFoundAs(err, ErrNotFound)
	}
	q := s.AutomationRun().Query().Where(automationrun.AutomationID(f.AutomationID))
	if f.Status != "" {
		q = q.Where(automationrun.StatusEQ(automationrun.Status(f.Status)))
	}
	return pagination.List(ctx, p, q.Order(ent.Desc(automationrun.FieldID)))
}

// Enrollment returns one Enrollment of an Automation. ErrEnrollmentNotFound when the Workspace
// has no such Automation, or the Automation no such Enrollment.
func (m *Module) Enrollment(ctx context.Context, s *ent.Scoped, automationID, id int64) (*ent.AutomationRun, error) {
	r, err := s.AutomationRun().Query().
		Where(automationrun.ID(id), automationrun.AutomationID(automationID)).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrEnrollmentNotFound
	}
	return r, err
}

// StepRecords returns what an Enrollment of an Automation kept of each Step it reached (ADR
// 0124), in the order it reached them. ErrEnrollmentNotFound when the Workspace has no such
// Automation, or the Automation no such Enrollment.
func (m *Module) StepRecords(ctx context.Context, s *ent.Scoped, automationID, enrollmentID int64) ([]*ent.AutomationStepRecord, error) {
	if _, err := m.Enrollment(ctx, s, automationID, enrollmentID); err != nil {
		return nil, err
	}
	return s.AutomationStepRecord().Query().
		Where(automationsteprecord.RunID(enrollmentID)).
		Order(ent.Asc(automationsteprecord.FieldReachedAt), ent.Asc(automationsteprecord.FieldID)).
		All(ctx)
}
