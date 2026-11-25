// Package messagesenders answers whether the Broadcasts and Automations a sent-by test names
// (ADR 0153, ADR 0180) are the Workspace's: a Segment, a Rule and an Automation refuse, each in its
// own words, when they are saved, a filter that names one the Workspace does not have. An archived one is the
// Workspace's: Archive keeps its row and the Events that name it, so a filter naming it still
// counts them, and neither is ever deleted by its owner (ADR 0100), so it reads both tables with
// QueryWithArchived (ADR 0102: a reader chooses the rows it takes). It is no Definition use: the
// Removal check does not read it. It is a module of its own because rules, which checks a filter,
// cannot import automations, which imports rules; it holds no state, so the modules that check a
// filter build it themselves.
package messagesenders

import (
	"context"
	"fmt"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/internal/domain/contacts/condition"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
)

// Module answers for the senders a filter names.
type Module struct{}

// New builds the module. It needs nothing: every question reads in the scope it is given.
func New() *Module { return &Module{} }

// Unknown is the first of named the Workspace of s does not have, archived ones being the
// Workspace's, or nil when it has them all.
func (m *Module) Unknown(ctx context.Context, s *ent.Scoped, named []condition.NamedSender) (*condition.NamedSender, error) {
	if len(named) == 0 {
		return nil, nil
	}
	ids := func(sender conditiontree.Sender) []int64 {
		return lo.Uniq(lo.FilterMap(named, func(n condition.NamedSender, _ int) (int64, bool) { return n.ID, n.Sender == sender }))
	}
	broadcasts, err := s.Broadcast().QueryWithArchived().Where(broadcast.IDIn(ids(conditiontree.SenderBroadcast)...)).IDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("read the named broadcasts: %w", err)
	}
	automations, err := s.Automation().QueryWithArchived().Where(automation.IDIn(ids(conditiontree.SenderAutomation)...)).IDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("read the named automations: %w", err)
	}
	has := map[conditiontree.Sender][]int64{conditiontree.SenderBroadcast: broadcasts, conditiontree.SenderAutomation: automations}
	for _, n := range named {
		if !lo.Contains(has[n.Sender], n.ID) {
			return &n, nil
		}
	}
	return nil, nil
}
