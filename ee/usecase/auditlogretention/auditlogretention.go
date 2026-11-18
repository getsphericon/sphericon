// Package auditlogretention is the use case of the Audit log's Retention (GLOSSARY, ADR 0014, ADR
// 0102), the Enterprise advanced-retention control: a Workspace sets a window in days
// (workspaces.retention_days), and a periodic river job, fanned out into one job per Workspace (ADR
// 0144), removes the Audit entries older than it, reading the window from the workspaces module and
// removing through the Audit log's (ee/domain/insights/auditentries). The Audit log's rows are
// otherwise append-only (ADR 0022): this job is their single remover. Unlicensed, the job prunes
// nothing. Licensed or not, it clears the client's address and user agent of every entry older than
// ClientWindow (ADR 0050).
//
// Governed by ee/LICENSE, not the AGPL.
package auditlogretention

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/getsphericon/sphericon/ee/domain/insights/auditentries"
	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
)

// Interval is how often the Edition sweeps every Workspace (Due, Sweep); the window is in days, so
// hourly is ample.
const Interval = time.Hour

// ClientWindow is how long an Audit entry keeps the client's address and user agent: long enough to
// look into a sign-in or a change that went wrong, and no longer, since they identify a person
// (ADR 0050). It holds whatever the Workspace's retention window, and without the license.
const ClientWindow = 90 * 24 * time.Hour

// UseCase is the Retention of the Audit log over the raw client: it lists the Workspaces, and a
// sweep scopes to the one it is given.
type UseCase struct {
	client *ent.Client
	log    *auditentries.Log
	lic    *licensekey.License
	now    func() time.Time
}

// New builds the use case over the raw client, the Audit log whose entries it prunes, the license
// that switches the retention on and the Instance clock the windows are measured on.
func New(client *ent.Client, log *auditentries.Log, lic *licensekey.License, now func() time.Time) *UseCase {
	return &UseCase{client: client, log: log, lic: lic, now: now}
}

// Due is every Workspace, the ones the job sweeps one by one (Sweep): each has client details to
// clear, whatever its window and the license.
func (u *UseCase) Due(ctx context.Context) ([]int64, error) {
	ids, err := u.client.Workspace.Query().Order(ent.Asc(workspace.FieldID)).IDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	return ids, nil
}

// Sweep is the job's work in one Workspace: Prune, then ForgetClients, which runs whether or not
// Prune failed. The failures are joined.
func (u *UseCase) Sweep(ctx context.Context, workspaceID int64) error {
	_, pruneErr := u.Prune(ctx, workspaceID)
	_, forgetErr := u.ForgetClients(ctx, workspaceID)
	return errors.Join(pruneErr, forgetErr)
}

// Prune deletes the Audit entries of the Workspace that occurred before now minus its retention
// window, and returns how many it removed. A Workspace with no window keeps everything, and so do
// one in its deletion window and one that is gone. Without the retention license it removes nothing.
func (u *UseCase) Prune(ctx context.Context, workspaceID int64) (int, error) {
	if !u.lic.Has(licensekey.FeatureRetention) {
		return 0, nil
	}
	ws, err := u.client.Workspace.Query().
		Where(workspace.ID(workspaceID), workspace.RetentionDaysNotNil(), workspaces.Live()).
		Select(workspace.FieldID, workspace.FieldRetentionDays).
		Only(ctx)
	if ent.IsNotFound(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read the retention window of workspace %d: %w", workspaceID, err)
	}
	cutoff := u.now().AddDate(0, 0, -*ws.RetentionDays)
	n, err := u.log.PruneBefore(ctx, u.client.Scoped(ws.ID), cutoff)
	if err != nil {
		return n, fmt.Errorf("prune audit entries of workspace %d: %w", ws.ID, err)
	}
	return n, nil
}

// ForgetClients clears the client's address and user agent of the Workspace's Audit entries older
// than ClientWindow, and returns how many entries it changed. It needs no license: the entries
// were written under one, and what they keep of a person stays bounded after it ends. A Workspace
// that is gone has no entries left to change.
func (u *UseCase) ForgetClients(ctx context.Context, workspaceID int64) (int, error) {
	n, err := u.log.ForgetClientsBefore(ctx, u.client.Scoped(workspaceID), u.now().Add(-ClientWindow))
	if err != nil {
		return n, fmt.Errorf("forget the clients of the audit entries of workspace %d: %w", workspaceID, err)
	}
	return n, nil
}
