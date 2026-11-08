// Package scope stands for what the middleware puts in the context: the scope and the actor.
package scope

import "context"

type Scoped struct{}

func From(ctx context.Context) *Scoped { return nil }
func Actor(ctx context.Context) int64  { return 0 }
