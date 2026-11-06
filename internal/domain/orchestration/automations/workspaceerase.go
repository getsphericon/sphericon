package automations

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/automationversion"
)

// Batches of a deleted Workspace's erase (workspacedeletion): the rows with the ids, unchecked.

func EraseAutomationRunsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.AutomationRun().Delete().Where(automationrun.IDIn(ids...)).Exec(ctx)
	return err
}

func EraseAutomationVersionsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.AutomationVersion().Delete().Where(automationversion.IDIn(ids...)).Exec(ctx)
	return err
}

func EraseAutomationsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.Automation().Purge().Where(automation.IDIn(ids...)).Exec(ctx)
	return err
}
