package automations

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
)

// The owner binds its pointers, the store the lifecycle writes through.
func store(s *ent.Scoped) *ent.AutomationPointers { return s.Automation().Pointers() }

func draft(ctx context.Context, s *ent.Scoped) error { return store(s).SetDraft(ctx, 1, 2) }
