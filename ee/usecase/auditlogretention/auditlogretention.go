// Package auditlogretention is the use case of the Audit log's Retention (GLOSSARY, ADR 0014, ADR
// 0102), the Enterprise advanced-retention control: a Workspace sets a window in days
// (workspaces.retention_days), and one periodic river job removes the Audit entries older than
// it, reading the windows from the workspaces module and removing through the Audit log's
// (ee/domain/insights/auditentries). The Audit log's rows are otherwise append-only (ADR 0022):
// this job is their single remover. Unlicensed, the job prunes nothing.
//
// Governed by ee/LICENSE, not the AGPL.
package auditlogretention

import (
	"context"
	"fmt"
	"time"

	"github.com/getsphericon/sphericon/ee/domain/insights/auditentries"
	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
)

// Interval is how often the Edition prunes (Prune); the window is in days, so hourly is ample.
const Interval = time.Hour

// Prune deletes, in every Workspace that set a retention window, the Audit entries that
// occurred before now minus that window, and returns how many it removed. A Workspace
// with no window keeps everything. Without the retention license it removes nothing.
func Prune(ctx context.Context, client *ent.Client, lic *licensekey.License, now time.Time) (int, error) {
	if !lic.Has(licensekey.FeatureRetention) {
		return 0, nil
	}
	live, err := client.Workspace.Query().
		Where(workspace.RetentionDaysNotNil(), workspaces.Live()).
		Select(workspace.FieldID, workspace.FieldRetentionDays).
		All(ctx)
	if err != nil {
		return 0, fmt.Errorf("list workspaces with a retention window: %w", err)
	}
	var total int
	for _, ws := range live {
		cutoff := now.AddDate(0, 0, -*ws.RetentionDays)
		n, err := auditentries.PruneBefore(ctx, client.Scoped(ws.ID), cutoff)
		if err != nil {
			return total, fmt.Errorf("prune audit entries of workspace %d: %w", ws.ID, err)
		}
		total += n
	}
	return total, nil
}
