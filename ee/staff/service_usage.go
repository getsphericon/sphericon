package staff

import (
	"context"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ee/usage"
	"github.com/getsphericon/sphericon/internal/pagination"
)

// UsageEntry is one Workspace's stored figures for a Usage period, under the Workspace's id.
type UsageEntry struct {
	// WorkspaceID is the Workspace's id (ADR 0043), the address the service responses name.
	WorkspaceID string
	Usage       usage.WorkspaceUsage
}

// UsageList is a page of every Workspace's stored figures for a Usage period, each under its
// Workspace's id. It reads tenant metadata through the raw client and never recounts.
func (m *Module) UsageList(ctx context.Context, period time.Time, p pagination.Params) (pagination.Page[UsageEntry], error) {
	if err := m.licensed(); err != nil {
		return pagination.Page[UsageEntry]{}, err
	}
	page, err := usage.List(ctx, m.ent, period, p)
	if err != nil {
		return pagination.Page[UsageEntry]{}, err
	}
	publicIDs, err := m.publicIDs(ctx, lo.Map(page.Items, func(w usage.WorkspaceUsage, _ int) int64 { return w.WorkspaceID }))
	if err != nil {
		return pagination.Page[UsageEntry]{}, err
	}
	return pagination.Map(page, func(w usage.WorkspaceUsage) UsageEntry {
		return UsageEntry{WorkspaceID: publicIDs[w.WorkspaceID], Usage: w}
	}), nil
}

// WorkspaceUsage reads one Workspace's stored figures for a period, scoped over the one
// Workspace the call names after its row is loaded. ent.IsNotFound for an unknown Workspace.
func (m *Module) WorkspaceUsage(ctx context.Context, workspaceID string, period time.Time) (usage.WorkspaceUsage, error) {
	if err := m.licensed(); err != nil {
		return usage.WorkspaceUsage{}, err
	}
	s, err := m.scopeOver(ctx, workspaceID)
	if err != nil {
		return usage.WorkspaceUsage{}, err
	}
	return usage.Get(ctx, s, period)
}
