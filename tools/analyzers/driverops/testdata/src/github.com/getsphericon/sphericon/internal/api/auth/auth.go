package auth

import (
	"context"

	"github.com/getsphericon/sphericon/internal/domain/identity/users"
)

// The security handlers the surfaces share receive a request's credentials: a driver.
func Check(ctx context.Context, m *users.Module) error { // want `rule 5: Check calls 2 operations of the core`
	_, _ = m.Get(ctx, 1)
	return m.Rename(ctx, 1, "x")
}
