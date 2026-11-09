package main

import (
	"context"

	"github.com/getsphericon/sphericon/internal/domain/identity/users"
)

func run(ctx context.Context, m *users.Module) error { // want `rule 5: run calls 2 operations of the core`
	_, _ = m.Get(ctx, 1)
	return m.Rename(ctx, 1, "x")
}

func main() {}
