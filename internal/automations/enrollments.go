package automations

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/pagination"
)

// Enrollments returns one page of an Automation's Enrollments, newest first, optionally
// only those in one status (an empty status means all). ErrNotFound when the Workspace has
// no such Automation.
func (m *Module) Enrollments(ctx context.Context, s *ent.Scoped, automationID int64, status string, p pagination.Params) (pagination.Page[*ent.AutomationRun], error) {
	if _, err := s.Automation().Get(ctx, automationID); err != nil {
		return pagination.Page[*ent.AutomationRun]{}, notFound(err)
	}
	query := func() *ent.AutomationRunQuery {
		q := s.AutomationRun().Query().Where(automationrun.AutomationID(automationID))
		if status != "" {
			q = q.Where(automationrun.StatusEQ(automationrun.Status(status)))
		}
		return q
	}
	return pagination.List(ctx, p,
		func(ctx context.Context) (int, error) { return query().Count(ctx) },
		func(ctx context.Context, limit, offset int) ([]*ent.AutomationRun, error) {
			return query().Order(ent.Desc(automationrun.FieldID)).Limit(limit).Offset(offset).All(ctx)
		})
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
