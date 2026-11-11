// Package rulesuse reads the Definition use the Rules make of a row (GLOSSARY "Definition use"):
// the kept Rules whose live version or draft names a Tag, in a Condition leaf or a tag Action, or
// tests a Custom field with a leaf a type does not take. It is a reader of the Removal check
// (internal/domain/orchestration/removalcheck), the one package easement lets import it, so every
// removal that checks a row reads every holder. The rules module does not import it.
package rulesuse

import (
	"context"
	"slices"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/ent/rule"
	"github.com/getsphericon/sphericon/internal/domain/contacts/condition"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
)

// UnfitForCustomFieldType returns the Workspace's kept Rules, in id order, whose live version or
// draft tests the Custom field key in its Condition with a leaf a field of type t does not take, so
// it would stop compiling were the field retyped. A deleted Rule acts on nothing, so it does not
// count.
func UnfitForCustomFieldType(ctx context.Context, s *ent.WorkspaceLocked, key string, t customfield.Type) ([]*ent.Rule, error) {
	kept, err := keptWithVersions(ctx, s)
	if err != nil {
		return nil, err
	}
	return lo.Filter(kept, func(r *ent.Rule, _ int) bool {
		return lo.SomeBy(versions(r), func(v *ent.RuleVersion) bool {
			g, err := condition.Parse(lo.FromPtr(v.Condition))
			return err == nil && !g.FitsCustomFieldType(key, t)
		})
	}), nil
}

// UsingTag returns the Workspace's kept Rules, in id order, whose live version or draft names the
// Tag by its name: in a Condition leaf or in a tag Action (spec #767, "In use"). A deleted Rule
// acts on nothing, so it does not hold the Tag.
func UsingTag(ctx context.Context, s *ent.WorkspaceLocked, name string) ([]*ent.Rule, error) {
	kept, err := keptWithVersions(ctx, s)
	if err != nil {
		return nil, err
	}
	var using []*ent.Rule
	for _, r := range kept {
		for _, v := range versions(r) {
			names, err := namesTag(v, name)
			if err != nil {
				return nil, err
			}
			if names {
				using = append(using, r)
				break
			}
		}
	}
	return using, nil
}

// keptWithVersions returns the Workspace's kept Rules in id order, each with its live version and
// its draft.
func keptWithVersions(ctx context.Context, s *ent.WorkspaceLocked) ([]*ent.Rule, error) {
	return s.Rule().Query().WithLiveVersion().WithDraftVersion().Order(ent.Asc(rule.FieldID)).All(ctx)
}

// versions are the Rule's live and draft versions that exist.
func versions(r *ent.Rule) []*ent.RuleVersion {
	live, draft := rules.LiveAndDraft(r)
	return lo.Compact([]*ent.RuleVersion{live, draft})
}

// namesTag reports whether the version names the Tag in its Condition or in a tag Action.
func namesTag(v *ent.RuleVersion, name string) (bool, error) {
	g, err := condition.Parse(lo.FromPtr(v.Condition))
	if err != nil {
		return false, err
	}
	if g.NamesTag(name) {
		return true, nil
	}
	actions, err := rules.DecodeActions(v.Actions)
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(actions, func(a rules.Action) bool { return a.NamesTag(name) }), nil
}
