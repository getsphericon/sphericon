package clean

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
)

type UseCase struct{ now func() time.Time }

// New takes the clock: a dependency, not an argument of a method.
func New(now func() time.Time) *UseCase { return &UseCase{now: now} }

func (*UseCase) Do(context.Context) error { return nil }

func (*UseCase) Two(context.Context, *ent.Scoped, int64, string) (*Result, error) { return nil, nil }

func (*UseCase) RawTwo(context.Context, *ent.Client, int64, string) error { return nil }

func (*UseCase) UnscopedTwo(context.Context, int64, string) error { return nil }

// Window is an input that is a time, not the clock.
func (*UseCase) Since(_ context.Context, _ *ent.Scoped, from time.Time) error { return nil }

type Result struct {
	ID    int64
	Codes []string
}
