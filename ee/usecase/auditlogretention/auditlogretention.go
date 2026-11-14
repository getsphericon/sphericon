// Package auditlogretention is the use case of the Audit log's Retention (GLOSSARY, ADR 0014, ADR
// 0102), the Enterprise advanced-retention control: a Workspace sets a window in days
// (workspaces.retention_days), and one periodic river job removes the Audit entries older than
// it, reading the windows from the workspaces module and removing through the Audit log's
// (ee/domain/insights/auditentries). The Audit log's rows are otherwise append-only (ADR 0022):
// this job is their single remover. Unlicensed, the job prunes nothing. Licensed or not, it clears
// the client's address and user agent of every entry older than ClientWindow (ADR 0050).
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

// ClientWindow is how long an Audit entry keeps the client's address and user agent: long enough to
// look into a sign-in or a change that went wrong, and no longer, since they identify a person
// (ADR 0050). It holds whatever the Workspace's retention window, and without the license.
const ClientWindow = 90 * 24 * time.Hour

// UseCase is the Retention of the Audit log over the raw client: it lists the Workspaces that set
// a window, then scopes to each one.
type UseCase struct {
	client *ent.Client
	lic    *licensekey.License
	now    func() time.Time
}

// New builds the use case over the raw client, the license that switches the retention on and
// the Instance clock the windows are measured on.
func New(client *ent.Client, lic *licensekey.License, now func() time.Time) *UseCase {
	return &UseCase{client: client, lic: lic, now: now}
}

// Prune deletes, in every Workspace that set a retention window, the Audit entries that
// occurred before now minus that window, and returns how many it removed. A Workspace
// with no window keeps everything. Without the retention license it removes nothing.
func (u *UseCase) Prune(ctx context.Context) (int, error) {
	if !u.lic.Has(licensekey.FeatureRetention) {
		return 0, nil
	}
	now := u.now()
	live, err := u.client.Workspace.Query().
		Where(workspace.RetentionDaysNotNil(), workspaces.Live()).
		Select(workspace.FieldID, workspace.FieldRetentionDays).
		All(ctx)
	if err != nil {
		return 0, fmt.Errorf("list workspaces with a retention window: %w", err)
	}
	var total int
	for _, ws := range live {
		cutoff := now.AddDate(0, 0, -*ws.RetentionDays)
		n, err := auditentries.PruneBefore(ctx, u.client.Scoped(ws.ID), cutoff)
		if err != nil {
			return total, fmt.Errorf("prune audit entries of workspace %d: %w", ws.ID, err)
		}
		total += n
	}
	return total, nil
}

// Sweep is the periodic job's one call: Prune, then ForgetClients.
func (u *UseCase) Sweep(ctx context.Context) error {
	if _, err := u.Prune(ctx); err != nil {
		return err
	}
	_, err := u.ForgetClients(ctx)
	return err
}

// ForgetClients clears, in every Workspace, the client's address and user agent of the Audit
// entries older than ClientWindow, and returns how many entries it changed. It needs no license:
// the entries were written under one, and what they keep of a person stays bounded after it ends.
func (u *UseCase) ForgetClients(ctx context.Context) (int, error) {
	ids, err := u.client.Workspace.Query().IDs(ctx)
	if err != nil {
		return 0, fmt.Errorf("list workspaces: %w", err)
	}
	cutoff := u.now().Add(-ClientWindow)
	var total int
	for _, id := range ids {
		n, err := auditentries.ForgetClientsBefore(ctx, u.client.Scoped(id), cutoff)
		if err != nil {
			return total, fmt.Errorf("forget the clients of the audit entries of workspace %d: %w", id, err)
		}
		total += n
	}
	return total, nil
}
