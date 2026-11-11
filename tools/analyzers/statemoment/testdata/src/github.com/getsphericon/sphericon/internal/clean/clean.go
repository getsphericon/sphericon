package clean

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/invitation"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
)

// The machine's storage is the one place its field is written, as a compare-and-swap.
func storage(c *ent.Client) lifecycle.Storage[bool] {
	return lifecycle.Storage[bool]{
		Read: func(context.Context) (bool, error) { return false, nil },
		Write: func(ctx context.Context, _, to bool) (bool, error) {
			upd := c.SendingDomain().Where(ent.Predicate{})
			if to {
				upd.SetVerifiedAt(time.Now())
			} else {
				upd.ClearVerifiedAt()
			}
			n, err := upd.Save(ctx)
			return n == 1, err
		},
	}
}

// A write in a closure nested inside the storage's Write is still inside it.
func nested(c *ent.Client) lifecycle.Storage[bool] {
	return lifecycle.Storage[bool]{
		Write: func(ctx context.Context, _, _ bool) (bool, error) {
			set := func() { c.SendingDomain().Where(ent.Predicate{}).SetVerifiedAt(time.Now()) }
			set()
			return true, nil
		},
	}
}

func latch(ctx context.Context, c *ent.Client, at time.Time) error {
	return lifecycle.Latch(ctx, func(ctx context.Context) (bool, error) {
		n, err := c.Invitation().Where(invitation.ID(1), invitation.AcceptedAtIsNil()).SetAcceptedAt(at).Save(ctx)
		return n == 1, err
	})
}

// Creating a row in its first state is not a move.
func create(c *ent.Client) {
	c.NewSendingDomain().SetVerifiedAt(time.Now())
}

// An attribute, here the latest check, is written freely.
func attribute(c *ent.Client) {
	c.SendingDomain().SetLastCheckedAt(time.Now())
}

// Another entity's field of the same name declares no form.
func other(c *ent.Client) {
	c.Tag().SetVerifiedAt(time.Now())
}
