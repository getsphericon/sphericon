// Package compose is a use case: composing operations is what it is for.
package compose

import (
	"context"

	"github.com/getsphericon/sphericon/internal/domain/identity/users"
)

func Run(ctx context.Context, m *users.Module) error {
	_, _ = m.Get(ctx, 1)
	return m.Rename(ctx, 1, "x")
}
