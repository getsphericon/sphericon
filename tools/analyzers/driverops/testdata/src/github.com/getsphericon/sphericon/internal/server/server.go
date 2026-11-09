package server

import (
	"context"

	"github.com/getsphericon/sphericon/internal/domain/identity/users"
)

// The router is the wiring, which handles no input: no driver.
func Build(ctx context.Context, m *users.Module) error {
	_, _ = m.Get(ctx, 1)
	return m.Rename(ctx, 1, "x")
}
