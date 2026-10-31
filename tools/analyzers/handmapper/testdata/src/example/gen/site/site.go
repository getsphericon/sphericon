// Package site stands in for an ogen server package.
package site

type ContactResource struct{ Email string }
type Opt[T any] struct{ Value T }

// CreateAutomationInput is a request body: the generated type, not a module input.
type CreateAutomationInput struct{ Name string }
