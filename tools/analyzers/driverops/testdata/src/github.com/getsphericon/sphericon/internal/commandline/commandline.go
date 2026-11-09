package commandline

import (
	"context"

	"github.com/getsphericon/sphericon/internal/domain/identity/users"
)

// The command line's driver package is a driver like the commands.
func Rename(ctx context.Context, m *users.Module) error { // want `rule 5: Rename calls 2 operations of the core`
	_, _ = m.Get(ctx, 1)
	return m.Rename(ctx, 1, "x")
}
