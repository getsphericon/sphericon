// Package consent holds the use-case modules that change a destination's consent
// state. It is shared by the public unsubscribe link and the external API so both
// produce identical effects.
package consent

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/events"
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
	dest := eligibility.NormalizeDestination(target.Destination)
	if dest == "" || target.WorkspaceID == 0 || target.Source == "" {
		return nil
	}
	automationID, isAutomation := eligibility.ParseAutomationSource(target.Source)

	return settled(bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		// The Workspace comes from the signed unsubscribe token (no membership or api
		// token here), so this is a scope source outside the site/external/job list.
		sc := events.Ingest(tx.Scoped(target.WorkspaceID))
		exists, err := sc.Unsubscribe().Query().Where(
			unsubscribe.ChannelEQ(unsubscribe.ChannelEmail),
			unsubscribe.DestinationEQ(dest),
			unsubscribe.SendingSourceEQ(target.Source),
		).Exist(ctx)
		if err != nil || exists {
			return err
		}
		create := sc.Unsubscribe().Create().
			SetChannel(unsubscribe.ChannelEmail).
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
		if target.Source == eligibility.SourceEverything {
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
			if _, err := automations.Exit(ctx, sc, automationID, target.ContactID); err != nil {
				return err
			}
		}

		return pub.Publish(ctx, &events.EmailEngagement{
			Action: events.NameEmailUnsubscribed, WorkspaceID: target.WorkspaceID, ContactID: target.ContactID,
			Email: dest, BroadcastID: target.BroadcastID,
		})
	}))
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
