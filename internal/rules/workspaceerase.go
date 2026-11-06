package rules

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/rule"
	"github.com/getsphericon/sphericon/ent/rulefiring"
	"github.com/getsphericon/sphericon/ent/ruleversion"
	"github.com/getsphericon/sphericon/ent/ruleversiontrigger"
)

// EraseRuleFiringsIn removes the scoped Workspace's Rule Firings with the ids, inside the
// transaction s is bound to: one batch of the erase of a deleted Workspace
// (internal/usecase/workspacedeletion, ADR 0043), which lists the ids and checks nothing else.
func EraseRuleFiringsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.RuleFiring().Delete().Where(rulefiring.IDIn(ids...)).Exec(ctx)
	return err
}

// EraseRuleVersionTriggersIn removes the scoped Workspace's Rule version triggers with the ids,
// inside the transaction s is bound to: one batch of the erase of a deleted Workspace
// (internal/usecase/workspacedeletion, ADR 0043), which lists the ids and checks nothing else.
func EraseRuleVersionTriggersIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.RuleVersionTrigger().Delete().Where(ruleversiontrigger.IDIn(ids...)).Exec(ctx)
	return err
}

// EraseRuleVersionsIn removes the scoped Workspace's Rule versions with the ids, inside the
// transaction s is bound to: one batch of the erase of a deleted Workspace
// (internal/usecase/workspacedeletion, ADR 0043), which lists the ids and checks nothing else.
func EraseRuleVersionsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.RuleVersion().Delete().Where(ruleversion.IDIn(ids...)).Exec(ctx)
	return err
}

// EraseRulesIn removes the scoped Workspace's Rules with the ids, discarded or not, inside the
// transaction s is bound to: one batch of the erase of a deleted Workspace
// (internal/usecase/workspacedeletion, ADR 0043), which lists the ids and checks nothing else.
func EraseRulesIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.Rule().Purge().Where(rule.IDIn(ids...)).Exec(ctx)
	return err
}
