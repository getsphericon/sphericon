package staff

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ee/domain/insights/usage"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
)

// UsageEntry is one Workspace's stored figures for a Usage period, under the Workspace's id.
type UsageEntry struct {
	// WorkspaceID is the Workspace's id (ADR 0043), the address the service responses name.
	WorkspaceID string
	Usage       usage.WorkspaceUsage
}

// UsageList is a page of every Workspace's stored figures for a Usage period (the current one
// when period is zero), each under its Workspace's id. It reads tenant metadata through the raw
// client and never recounts.
func (m *Module) UsageList(ctx context.Context, period time.Time, p pagination.Params) (pagination.Page[UsageEntry], error) {
	if err := m.licensed(); err != nil {
		return pagination.Page[UsageEntry]{}, err
	}
	page, err := usage.List(ctx, m.ent, m.usagePeriod(period), p)
	if err != nil {
		return pagination.Page[UsageEntry]{}, err
	}
	publicIDs, err := publicIDsOf(ctx, m, page.Items, func(w usage.WorkspaceUsage) int64 { return w.WorkspaceID })
	if err != nil {
		return pagination.Page[UsageEntry]{}, err
	}
	return pagination.Map(page, func(w usage.WorkspaceUsage) UsageEntry {
		return UsageEntry{WorkspaceID: publicIDs[w.WorkspaceID], Usage: w}
	}), nil
}

// WorkspaceUsage reads one Workspace's stored figures for a Usage period (the current one when
// period is zero), scoped over the one Workspace the call names after its row is loaded.
// ErrWorkspaceNotFound for an unknown Workspace.
func (m *Module) WorkspaceUsage(ctx context.Context, workspaceID string, period time.Time) (usage.WorkspaceUsage, error) {
	if err := m.licensed(); err != nil {
		return usage.WorkspaceUsage{}, err
	}
	s, err := m.scopeOver(ctx, workspaceID)
	if err != nil {
		return usage.WorkspaceUsage{}, err
	}
	return usage.Get(ctx, s, m.usagePeriod(period))
}

// usagePeriod is the Usage period a read names: the one given, or the current one for zero.
func (m *Module) usagePeriod(period time.Time) time.Time {
	if period.IsZero() {
		return usage.PeriodOf(m.now())
	}
	return period
}
