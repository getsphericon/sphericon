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

func usage(c *ent.Client) {
	c.Usage().SetFinalizedAt(time.Now()) // want `rule 7: SetFinalizedAt writes a state moment`
}

func usageNillable(c *ent.Client, at *time.Time) {
	c.Usage().SetNillableFinalizedAt(at) // want `rule 7: SetNillableFinalizedAt writes a state moment`
}

func quota(c *ent.Client) {
	c.Integration().SetProviderQuotaFailedAt(time.Now()) // want `rule 7: SetProviderQuotaFailedAt writes a state moment`
	c.Integration().ClearProviderQuotaFailedAt()         // want `rule 7: ClearProviderQuotaFailedAt writes a state moment`
}
