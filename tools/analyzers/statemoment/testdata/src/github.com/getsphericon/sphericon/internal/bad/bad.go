package bad

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/invitation"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
)

// A machine field written by a bare update, fenced or not, skips the machine's table.
func bareMachine(ctx context.Context, c *ent.Client) error {
	_, err := c.SendingDomain().Where(ent.Predicate{}).SetVerifiedAt(time.Now()).Save(ctx) // want `rule 7: SetVerifiedAt writes the machine field SendingDomain.VerifiedAt outside a lifecycle.Storage Write`
	return err
}

func scopedMachine(c *ent.Client) {
	c.ScopedSendingDomain().SetVerifiedAt(time.Now()) // want `rule 7: SetVerifiedAt writes the machine field SendingDomain.VerifiedAt outside`
}

func upsertMachine(c *ent.Client) {
	c.SendingDomainUpsert().UpdateVerifiedAt() // want `rule 7: UpdateVerifiedAt writes the machine field SendingDomain.VerifiedAt outside`
}

// A method value escapes the check no more than a call.
func methodValue(c *ent.Client, apply func(func(time.Time) *ent.SendingDomainUpdateOne)) {
	apply(c.SendingDomain().SetVerifiedAt) // want `rule 7: SetVerifiedAt writes the machine field`
}

// Inside the storage, the write is still the compare-and-swap on the state it leaves.
func unfencedWrite(c *ent.Client) lifecycle.Storage[bool] {
	return lifecycle.Storage[bool]{
		Write: func(ctx context.Context, _, _ bool) (bool, error) {
			n, err := c.SendingDomain().ClearVerifiedAt().Save(ctx) // want `rule 7: ClearVerifiedAt writes the machine field SendingDomain.VerifiedAt without a condition`
			return n == 1, err
		},
	}
}

// A machine's field in a latch is still outside its machine.
func machineInLatch(ctx context.Context, c *ent.Client) error {
	return lifecycle.Latch(ctx, func(ctx context.Context) (bool, error) {
		n, err := c.SendingDomain().Where(ent.Predicate{}).SetVerifiedAt(time.Now()).Save(ctx) // want `rule 7: SetVerifiedAt writes the machine field`
		return n == 1, err
	})
}

func bareLatch(ctx context.Context, c *ent.Client) error {
	_, err := c.Invitation().Where(invitation.AcceptedAtIsNil()).SetAcceptedAt(time.Now()).Save(ctx) // want `rule 7: SetAcceptedAt writes the latch Invitation.AcceptedAt outside lifecycle.Latch`
	return err
}

func latchWithoutCondition(ctx context.Context, c *ent.Client) error {
	return lifecycle.Latch(ctx, func(ctx context.Context) (bool, error) {
		n, err := c.Invitation().Where(invitation.ID(1)).SetAcceptedAt(time.Now()).Save(ctx) // want `rule 7: SetAcceptedAt writes the latch Invitation.AcceptedAt without AcceptedAtIsNil\(\) in its condition`
		return n == 1, err
	})
}

func clearLatch(ctx context.Context, c *ent.Client) error {
	return lifecycle.Latch(ctx, func(ctx context.Context) (bool, error) {
		n, err := c.Invitation().Where(invitation.AcceptedAtIsNil()).ClearAcceptedAt().Save(ctx) // want `rule 7: ClearAcceptedAt clears the latch Invitation.AcceptedAt, and a latch is never cleared`
		return n == 1, err
	})
}

func nillableLatch(c *ent.Client, at *time.Time) {
	c.Invitation().SetNillableAcceptedAt(at) // want `rule 7: SetNillableAcceptedAt writes the latch`
}
