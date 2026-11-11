// Package lifecycle is a stub of internal/kernel/lifecycle: the machine storage and the latch.
package lifecycle

import "context"

type Storage[S comparable] struct {
	Read  func(ctx context.Context) (S, error)
	Write func(ctx context.Context, from, to S) (bool, error)
}

func Latch(ctx context.Context, write func(context.Context) (bool, error)) error {
	_, err := write(ctx)
	return err
}
