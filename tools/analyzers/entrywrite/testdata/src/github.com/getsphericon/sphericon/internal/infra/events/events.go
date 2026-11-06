// Package events is a stub of the events bus.
package events

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
)

type Bus struct{}

func (*Bus) WithinScopedTx(context.Context, *ent.Scoped, func(*ent.Scoped) error) error { return nil }
