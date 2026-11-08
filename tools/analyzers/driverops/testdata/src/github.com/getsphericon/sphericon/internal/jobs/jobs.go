package jobs

import (
	"context"

	"github.com/getsphericon/sphericon/internal/domain/identity/users"
)

type Worker struct{ users *users.Module }

func (w *Worker) Work(ctx context.Context) error { // want `rule 5: Work calls 2 operations of the core`
	_, _ = w.users.Get(ctx, 1)
	return w.users.Rename(ctx, 1, "x")
}
