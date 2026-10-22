// Package ent stands in for the generated ORM package.
package ent

type Contact struct{ Email string }

func (c *Contact) Unwrap() *Contact          { return c }
func (c *Contact) Value(string) (any, error) { return nil, nil }

// Scoped is not an entity: it has no Unwrap.
type Scoped struct{}
