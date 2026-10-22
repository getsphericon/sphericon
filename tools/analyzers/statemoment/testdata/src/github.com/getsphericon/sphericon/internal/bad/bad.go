package bad

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
)

func chain(ctx context.Context, c *ent.Client) error {
	return c.SendingDomain().SetVerifiedAt(time.Now()).Exec(ctx) // want `rule 7: SetVerifiedAt writes a state moment without a condition on the state it leaves`
}

func viaVariable(ctx context.Context, c *ent.Client, lost bool) error {
	upd := c.SendingDomain().SetLastCheckedAt(time.Now())
	if lost {
		upd.SetVerificationLostAt(time.Now()) // want `rule 7: SetVerificationLostAt writes a state moment without a condition on the state it leaves`
	}
	return upd.Exec(ctx)
}

func scoped(ctx context.Context, c *ent.Client) error {
	return c.ScopedSendingDomain().SetVerificationLostAt(time.Now()).Exec(ctx) // want `rule 7: SetVerificationLostAt writes a state moment`
}

func webhook(c *ent.Client) {
	c.WebhookEndpoint().SetDisabledAt(time.Now()) // want `rule 7: SetDisabledAt writes a state moment`
}

func join(c *ent.Client) {
	c.JoinDomain().ClearVerifiedAt() // want `rule 7: ClearVerifiedAt writes a state moment`
}
