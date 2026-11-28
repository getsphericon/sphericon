package contactimports

import (
	"context"
	"fmt"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contactimport"
	"github.com/getsphericon/sphericon/ent/contactimportreportrow"
)

// UploadKept is how long an uploaded file is kept when its import never runs (ADR 0138).
const UploadKept = 24 * time.Hour

// Expired is what an expiry deleted: the imports abandoned before they ran, with their files, and
// the reports past their expiry.
type Expired struct {
	Uploads, Reports int
}

// Swept is what a sweep of the imports of every Workspace did and found: what it expired, and the
// imports that run, each of which its river job checks for a run whose job is gone (Recover).
type Swept struct {
	Expired
	Running []Running
}

// Sweep is the periodic sweep of the imports of every Workspace (ADR 0138): it deletes what they
// keep past its time (expire) and lists the imports that run. It is Instance-wide, so it reads and
// writes through the raw client its river job hands it, by the module's clock.
func (m *Module) Sweep(ctx context.Context, client *ent.Client) (Swept, error) {
	expired, err := m.expire(ctx, client)
	if err != nil {
		return Swept{}, err
	}
	running, err := runningIn(ctx, client)
	return Swept{Expired: expired, Running: running}, err
}

// expire deletes what the imports of every Workspace keep past its time: an import uploaded or
// ready for longer than UploadKept, with its file, and the refused rows' report of an import whose
// report expired, the import itself kept with its counts.
func (m *Module) expire(ctx context.Context, client *ent.Client) (Expired, error) {
	now := m.now()
	uploads, err := client.ContactImport.Delete().
		Where(contactimport.StateIn(contactimport.StateUploaded, contactimport.StateReady), contactimport.CreatedAtLT(now.Add(-UploadKept))).
		Exec(ctx)
	if err != nil {
		return Expired{}, fmt.Errorf("contactimports: delete abandoned uploads: %w", err)
	}
	ids, err := client.ContactImport.Query().Where(contactimport.ReportExpiresAtLT(now)).IDs(ctx)
	if err != nil || len(ids) == 0 {
		return Expired{Uploads: uploads}, err
	}
	if _, err := client.ContactImportReportRow.Delete().Where(contactimportreportrow.ContactImportIDIn(ids...)).Exec(ctx); err != nil {
		return Expired{}, fmt.Errorf("contactimports: delete expired reports: %w", err)
	}
	if err := client.ContactImport.Update().Where(contactimport.IDIn(ids...)).ClearReportExpiresAt().Exec(ctx); err != nil {
		return Expired{}, err
	}
	return Expired{Uploads: uploads, Reports: len(ids)}, nil
}

// Running is an import that runs, by its id and its Workspace's.
type Running struct {
	ID, WorkspaceID int64
}

// runningIn are the running imports of every Workspace.
func runningIn(ctx context.Context, client *ent.Client) ([]Running, error) {
	imps, err := client.ContactImport.Query().Where(contactimport.StateEQ(contactimport.StateRunning)).
		Select(contactimport.FieldID, contactimport.FieldWorkspaceID).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Running, len(imps))
	for i, imp := range imps {
		out[i] = Running{ID: imp.ID, WorkspaceID: imp.WorkspaceID}
	}
	return out, nil
}
