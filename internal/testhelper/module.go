package testhelper

import "github.com/samber/do/v2"

// Module is the Instance's own T, as the container built it for env (see internal/AGENTS.md).
func Module[T any](env *TestEnv) T { return do.MustInvoke[T](env.injector) }
