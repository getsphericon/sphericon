package instance

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
)

// DefaultSendCap is the Instance's default Send cap (ADR 0067): what every Workspace created from
// now on starts with, in messages per rolling 24 hours; nil when there is none.
func DefaultSendCap(ctx context.Context, client *ent.Client) (*int, error) {
	row, err := ensureRow(ctx, client)
	if err != nil {
		return nil, err
	}
	return row.DefaultSendCap, nil
}

// SetDefaultSendCap sets the Instance's default Send cap, or removes it when sendCap is nil. It
// returns the value before and whether it changed. Existing Workspaces keep their Send caps: the
// default is copied only onto a Workspace being created.
func SetDefaultSendCap(ctx context.Context, client *ent.Client, sendCap *int) (prev *int, changed bool, err error) {
	row, err := ensureRow(ctx, client)
	if err != nil {
		return nil, false, err
	}
	prev = row.DefaultSendCap
	if equalCap(prev, sendCap) {
		return prev, false, nil
	}
	upd := client.InstanceState.UpdateOneID(stateID)
	if sendCap == nil {
		upd.ClearDefaultSendCap()
	} else {
		upd.SetDefaultSendCap(*sendCap)
	}
	return prev, true, upd.Exec(ctx)
}

func equalCap(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
