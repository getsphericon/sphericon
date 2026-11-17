// Package events is a stub of the events bus.
package events

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
)

type Bus struct{}

func (*Bus) WithinScopedTx(context.Context, *ent.Scoped, func(*ent.Scoped) error) error { return nil }

// InTx opens a transaction under another name: its callback is handed the transaction's client.
func (*Bus) InTx(context.Context, func(*ent.Client) error) error { return nil }

// WithinBudget opens none, whatever its name says.
func (*Bus) WithinBudget(context.Context, func(int) error) error { return nil }
