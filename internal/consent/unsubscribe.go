// Package consent holds the use-case modules that change a destination's consent
// state. It is shared by the public unsubscribe link and the external API so both
// produce identical effects.
package consent

import (
	"context"
	"fmt"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/suppression"
	"github.com/getsphericon/sphericon/internal/tracking"
)

// RecordUnsubscribe writes a per-(channel, destination, sending source) opt-out
// (ADR 0001) from a signed token — no contact-row lookup, so the opt-out records
// even if the contact was deleted between send and click (the point of
// destination-keying). The default in-email link is scoped to its sending source;
// "everything" is the deliberate escalation. All effects (the row, the broadcast
// counter, the automation enrollment exit, and the engagement event) live in one
// transaction gated on the existence check, so a repeated POST (mailbox retry or a
// double click) is a complete no-op and concurrent POSTs are counted exactly once.
func RecordUnsubscribe(ctx context.Context, bus *events.Bus, target tracking.UnsubTarget) error {
	dest := suppression.NormalizeDestination(target.Destination)
	if dest == "" || target.WorkspaceID == 0 || target.Source == "" || target.Channel == "" {
		return nil
	}
	channel := unsubscribe.Channel(target.Channel)
	automationID, isAutomation := eligibility.ParseAutomationSource(target.Source)

	return settled(bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		// The Workspace comes from the signed unsubscribe token (no membership or api
		// token here), so this is a scope source outside the site/external/job list.
		sc := events.Ingest(tx.Scoped(target.WorkspaceID))
		exists, err := sc.Unsubscribe().Query().Where(
			unsubscribe.ChannelEQ(channel),
			unsubscribe.DestinationEQ(dest),
			unsubscribe.SendingSourceEQ(target.Source),
		).Exist(ctx)
		if err != nil || exists {
			return err
		}
		create := sc.Unsubscribe().Create().
			SetChannel(channel).
			SetDestination(dest).
			SetSendingSource(target.Source)
		if target.ContactID != 0 {
			create.SetContactID(target.ContactID)
		}
		if _, err := create.Save(ctx); err != nil {
			return err
		}

		// Everything-opt-out invalidates any confirmation (ADR 0013): the deliberate
		// "leave entirely" deletes the derived Confirmation row so returning requires
		// re-confirmation (stale consent never silently reactivates). The immutable
		// marketing.confirmed Event is preserved as proof ("confirmed at T1, left at
		// T2"). A narrower per-source opt-out does NOT touch confirmation.
		if target.Source == eligibility.SourceEverything && target.Channel == messaging.ChannelEmail {
			if _, err := sc.Confirmation().Delete().Where(
				confirmation.ChannelEQ(confirmation.ChannelEmail),
				confirmation.DestinationEQ(dest),
			).Exec(ctx); err != nil {
				return err
			}
		}

		// Broadcast attribution: the triggering Broadcast counts the recipient's unsubscribe.
		if target.BroadcastID != 0 && target.ContactID != 0 {
			if _, err := broadcasts.RecordUnsubscribe(ctx, sc, target.BroadcastID, target.ContactID); err != nil {
				return err
			}
		}
		// Automation: unsubscribing from an automation also exits its active
		// enrollment (ADR: two effects from one action).
		if isAutomation && target.ContactID != 0 {
			if _, err := automations.Exit(ctx, sc, pub, automationID, target.ContactID, time.Now()); err != nil {
				return err
			}
		}

		if target.Channel == messaging.ChannelTelegram {
			return publishTelegramUnsubscribed(ctx, sc, pub, target, dest)
		}
		if target.Channel != messaging.ChannelEmail {
			return nil
		}
		return pub.Publish(ctx, &events.EmailEngagement{
			Action: events.NameEmailUnsubscribed, WorkspaceID: target.WorkspaceID, ContactID: target.ContactID,
			Email: dest, BroadcastID: target.BroadcastID, AutomationID: automationID,
		})
	}))
}

// publishTelegramUnsubscribed records telegram.unsubscribed for the Contact a Telegram unsubscribe
// link was issued to (the opt-out itself is the source-scoped row written above). A link with no
// Contact, or one whose Contact is gone, has nobody to name.
func publishTelegramUnsubscribed(ctx context.Context, sc *ent.Scoped, pub events.Publisher, target tracking.UnsubTarget, dest string) error {
	if target.ContactID == 0 {
		return nil
	}
	c, err := sc.Contact().Get(ctx, target.ContactID)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	automationID, _ := eligibility.ParseAutomationSource(target.Source)
	return pub.Publish(ctx, &events.TelegramUnsubscribed{
		WorkspaceID: target.WorkspaceID, ContactID: c.ID, Email: lo.FromPtr(c.Email), Via: events.UnsubscribedByLink,
		BroadcastID: target.BroadcastID, AutomationID: automationID,
		DedupID: fmt.Sprintf("link:%s:%s/unsubscribed/%d", target.Source, dest, c.ID),
	})
}

// settled reports a write that lost to a database constraint as done: the Workspace
// is gone (nothing is left to record) or a concurrent request recorded the same row
// first. Any other failure is a real one and is returned.
func settled(err error) error {
	if ent.IsConstraintError(err) {
		return nil
	}
	return err
}
