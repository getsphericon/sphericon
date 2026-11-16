// Package ent is a stub of the generated ent package: the scopes an operation takes and an entity.
package ent

import "context"

// Scoped is the client of one Workspace.
type Scoped struct{}

// WorkspaceLocked is the scope that stands for the Workspace lock.
type WorkspaceLocked struct{ *Scoped }

// Client is the raw client.
type Client struct{}

// config is what every generated entity embeds: it carries the raw client.
type config struct{}

func (*config) ExecContext(context.Context, string) error { return nil }

// User is an entity.
type User struct{ config }
