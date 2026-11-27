package jobs

import (
	"context"
	"log/slog"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contactimports"
)

// ExpireContactImportsArgs is the hourly tick that deletes what CSV imports keep past its time
// (ADR 0138): an upload that never ran after a day, a report seven days after its run ended.
type ExpireContactImportsArgs struct{}

func (ExpireContactImportsArgs) Kind() string { return "contact_imports_expire" }

// ExpireContactImportsWorker runs the expiry of the imports of every Workspace. It is
// Instance-wide, so it hands the module the raw ent client.
type ExpireContactImportsWorker struct {
	river.WorkerDefaults[ExpireContactImportsArgs]

	imports *contactimports.Module
	ent     *ent.Client
}

// NewExpireContactImportsWorker builds the expiry of the imports of imports over client.
func NewExpireContactImportsWorker(imports *contactimports.Module, client *ent.Client) *ExpireContactImportsWorker {
	return &ExpireContactImportsWorker{imports: imports, ent: client}
}

func (w *ExpireContactImportsWorker) Work(ctx context.Context, _ *river.Job[ExpireContactImportsArgs]) error {
	expired, err := w.imports.Expire(ctx, w.ent)
	if err != nil {
		return err
	}
	if expired.Uploads > 0 || expired.Reports > 0 {
		slog.InfoContext(ctx, "expired contact imports pruned", "uploads", expired.Uploads, "reports", expired.Reports)
	}
	return nil
}
