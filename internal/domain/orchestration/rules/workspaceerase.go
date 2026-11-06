package rules

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/rule"
	"github.com/getsphericon/sphericon/ent/rulefiring"
	"github.com/getsphericon/sphericon/ent/ruleversion"
	"github.com/getsphericon/sphericon/ent/ruleversiontrigger"
)

// Batches of a deleted Workspace's erase (workspacedeletion): the rows with the ids, unchecked.

func EraseRuleFiringsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.RuleFiring().Delete().Where(rulefiring.IDIn(ids...)).Exec(ctx)
	return err
}

func EraseRuleVersionTriggersIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.RuleVersionTrigger().Delete().Where(ruleversiontrigger.IDIn(ids...)).Exec(ctx)
	return err
}

func EraseRuleVersionsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.RuleVersion().Delete().Where(ruleversion.IDIn(ids...)).Exec(ctx)
	return err
}

func EraseRulesIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.Rule().Purge().Where(rule.IDIn(ids...)).Exec(ctx)
	return err
}
