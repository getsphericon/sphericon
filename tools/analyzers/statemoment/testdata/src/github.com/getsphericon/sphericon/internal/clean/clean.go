package clean

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
)

func fenced(ctx context.Context, c *ent.Client) error {
	return c.SendingDomain().Where(ent.Predicate{}).SetVerifiedAt(time.Now()).Exec(ctx)
}

func fencedVariable(ctx context.Context, c *ent.Client, lost bool) error {
	upd := c.SendingDomain().SetLastCheckedAt(time.Now())
	if lost {
		upd.Where(ent.Predicate{}).SetVerificationLostAt(time.Now())
	}
	return upd.Exec(ctx)
}

func fencedEarlier(c *ent.Client) {
	upd := c.WebhookEndpoint()
	upd.Where(ent.Predicate{})
	upd.SetDisabledAt(time.Now())
}

// Creating a row in its first state is not a move.
func create(c *ent.Client) {
	c.NewSendingDomain().SetVerifiedAt(time.Now())
}

// Leaving a state by the owner's own act (a clear of the webhook's moment) is not covered.
func ownerEnables(c *ent.Client) {
	c.WebhookEndpoint().ClearDisabledAt()
}

// Another entity's field of the same name is not a state moment.
func other(c *ent.Client) {
	c.Tag().SetVerifiedAt(time.Now())
}

func closesOnce(c *ent.Client) {
	c.Usage().Where(ent.Predicate{}).SetFinalizedAt(time.Now())
}

func quotaGuarded(c *ent.Client) {
	c.Integration().Where(ent.Predicate{}).SetProviderQuotaFailedAt(time.Now())
}
