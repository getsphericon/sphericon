package automations

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/automationversion"
)

// EraseAutomationRunsIn removes the scoped Workspace's Enrollments (Automation runs) with the ids,
// inside the transaction s is bound to: one batch of the erase of a deleted Workspace
// (internal/usecase/workspacedeletion, ADR 0043), which lists the ids and checks nothing else.
func EraseAutomationRunsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.AutomationRun().Delete().Where(automationrun.IDIn(ids...)).Exec(ctx)
	return err
}

// EraseAutomationVersionsIn removes the scoped Workspace's Automation versions with the ids, inside
// the transaction s is bound to: one batch of the erase of a deleted Workspace
// (internal/usecase/workspacedeletion, ADR 0043), which lists the ids and checks nothing else.
func EraseAutomationVersionsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.AutomationVersion().Delete().Where(automationversion.IDIn(ids...)).Exec(ctx)
	return err
}

// EraseAutomationsIn removes the scoped Workspace's Automations with the ids, archived or not,
// inside the transaction s is bound to: one batch of the erase of a deleted Workspace
// (internal/usecase/workspacedeletion, ADR 0043), which lists the ids and checks nothing else.
func EraseAutomationsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.Automation().Purge().Where(automation.IDIn(ids...)).Exec(ctx)
	return err
}
