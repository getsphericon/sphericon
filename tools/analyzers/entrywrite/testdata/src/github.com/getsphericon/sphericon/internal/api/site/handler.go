package site

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
)

type tags interface {
	Create(ctx context.Context, s *ent.Scoped, name string) error
}

// A handler reads through ent and calls a module for every write.
func handle(ctx context.Context, s *ent.Scoped, m tags) error {
	if _, err := s.Tag().Query().All(ctx); err != nil {
		return err
	}
	if err := s.Tag().Get(ctx, 1); err != nil {
		return err
	}
	return m.Create(ctx, s, "x")
}
