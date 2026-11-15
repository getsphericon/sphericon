package testhelper

import "github.com/samber/do/v2"

// Module is the Instance's own T: a module or use case as the container built it for this env,
// over its database, bus, clock (WithClock) and seams. A test reads the server's own rather than
// building one with New, which would differ from it wherever the container adds an option; New is
// for a test that hands the module a seam of its own (a clock, a sender, a client).
func Module[T any](env *TestEnv) T { return do.MustInvoke[T](env.injector) }
