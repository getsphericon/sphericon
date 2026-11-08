// Package unsubscriberecording is the use case that records an Unsubscribe (ADR 0001, ADR 0102):
// from a signed unsubscribe link, or by an authenticated caller (the external API, an agent over
// MCP) for an opt-out that reached the Workspace by another route. Beside the row (unsubscribes)
// an unsubscribe ends the Enrollment of the Automation it names (automations), in one transaction:
// that module sits above the opt-out's, so the operation across them lives here. The opt-out's
// Event (email.unsubscribed, telegram.unsubscribed, sms.unsubscribed) names the Broadcast that
// provoked it, which counts it from the event, as it counts an opt-out made on any other path
// (ADR 0113). The link and the API produce identical effects.
package unsubscriberecording

import (
	"context"
	"fmt"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/internal/domain/contacts/channelstop"
	"github.com/getsphericon/sphericon/internal/domain/contacts/confirmations"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/contacts/suppressions"
	"github.com/getsphericon/sphericon/internal/domain/contacts/unsubscribes"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/linktarget"
)

// UseCase records Unsubscribes. Every call receives the Workspace-scoped client (ADR 0017) built
// by the entry point; the use case opens the transaction the modules it calls join.
type UseCase struct {
	bus *events.Bus
	now func() time.Time
}

// New builds the unsubscribe use case over the bus and the Instance clock.
func New(bus *events.Bus, now func() time.Time) *UseCase { return &UseCase{bus: bus, now: now} }

// RecordLink writes a per-(channel, destination, sending source) opt-out
// (ADR 0001) from a signed token — no contact-row lookup, so the opt-out records
// even if the contact was deleted between send and click (the point of
// destination-keying). The default in-email link is scoped to its sending source;
// "everything" is the deliberate escalation. All effects (the row, the automation
// enrollment exit, and the opt-out's event, from which the Broadcast counts it) live in
// one transaction gated on the existence check, so a repeated POST (mailbox retry or a
// double click) is a complete no-op and concurrent POSTs are recorded exactly once. s is the scope
// of the token's Workspace, which the entry point builds from it; the writes go through it as the
// ingest actor (never audited), since no membership or API token stands behind them.
func (u *UseCase) RecordLink(ctx context.Context, s *ent.Scoped, target linktarget.Unsubscribe) error {
	dest := suppressions.NormalizeDestination(target.Destination)
	if dest == "" || target.WorkspaceID == 0 || target.Source == "" || target.Channel == "" {
		return nil
	}
	channel := target.Channel
	automationID, isAutomation := eligibility.ParseAutomationSource(target.Source)

	// The Workspace comes from the signed unsubscribe token (no membership or api
	// token here), so this is a scope source outside the site/external/job list.
	return u.bus.WithinScopedTx(ctx, events.Ingest(s), func(sc *ent.Scoped, pub events.Publisher) error {
		// A link of an erased Workspace has nothing left to record into.
		erased, err := workspaces.ErasedIn(ctx, sc)
		if err != nil || erased {
			return err
		}
		if target.Channel == channels.SMS {
			return recordSMSLink(ctx, sc, pub, target, dest)
		}
		o := unsubscribes.OptOut{Channel: channel, Destination: dest, Source: target.Source, ContactID: target.ContactID}
		if target.Channel == channels.Telegram {
			// On email the email.unsubscribed Event says how; on Telegram the row does.
			o.Via = unsubscribe.ViaLink
		}
		// A repeated or concurrent click finds the opt-out written: the rest is done already.
		added, err := unsubscribes.Add(ctx, sc, o)
		if err != nil || !added {
			return err
		}

		// Everything-opt-out invalidates any confirmation (ADR 0013): the deliberate
		// "leave entirely" deletes the derived Confirmation row so returning requires
		// re-confirmation (stale consent never silently reactivates). The immutable
		// marketing.confirmed Event is preserved as proof ("confirmed at T1, left at
		// T2"). A narrower per-source opt-out does NOT touch confirmation.
		if target.Source == eligibility.SourceEverything && target.Channel == channels.Email {
			if err := confirmations.Withdraw(ctx, sc, channels.Email, dest); err != nil {
				return err
			}
		}

		// Automation: unsubscribing from an automation also exits its active
		// enrollment (ADR: two effects from one action).
		if isAutomation && target.ContactID != 0 {
			if _, err := automations.Exit(ctx, sc, pub, automationID, target.ContactID, u.now()); err != nil {
				return err
			}
		}

		if target.Channel == channels.Telegram {
			return publishTelegramUnsubscribed(ctx, sc, pub, target, dest)
		}
		if target.Channel != channels.Email {
			return nil
		}
		return pub.Publish(ctx, &events.EmailEngagement{
			Action: events.NameEmailUnsubscribed, WorkspaceID: target.WorkspaceID, ContactID: target.ContactID,
			Email: dest, BroadcastID: target.BroadcastID, AutomationID: automationID,
			OutboundMessageID: target.OutboundMessageID,
		})
	})
}

// recordSMSLink records the click on a text's unsubscribe link: on SMS every opt-out is the
// number's Channel-wide one (ADR 0077), whatever source the link names, with the same effects as a
// STOP (channelstop.StopSMS), and its sms.unsubscribed names the Broadcast the text came from, which
// counts it. A repeated or concurrent click finds the number stopped and records nothing. The
// Automation's Enrollment goes on: its next text is Skipped like any other, and a step on another
// Channel still runs.
func recordSMSLink(ctx context.Context, sc *ent.Scoped, pub events.Publisher, target linktarget.Unsubscribe, number string) error {
	automationID, _ := eligibility.ParseAutomationSource(target.Source)
	return channelstop.StopSMS(ctx, sc, pub, channelstop.SMSStop{
		Number: number, Via: events.UnsubscribedByLink,
		ContactID: target.ContactID, BroadcastID: target.BroadcastID, AutomationID: automationID,
	})
}

// publishTelegramUnsubscribed records telegram.unsubscribed for the Contact a Telegram unsubscribe
// link was issued to (the opt-out itself is the source-scoped row written above). A link with no
// Contact, or one whose Contact is gone, has nobody to name. The event is keyed by the opt-out row,
// new with every opt-out: a key of the source and the Destination alone would make an opt-out made
// again after a `/start` lifted it a repeat, which the bus drops with its count.
func publishTelegramUnsubscribed(ctx context.Context, sc *ent.Scoped, pub events.Publisher, target linktarget.Unsubscribe, dest string) error {
	if target.ContactID == 0 {
		return nil
	}
	row, err := unsubscribes.Get(ctx, sc, channels.Telegram, dest, target.Source)
	if err != nil {
		return fmt.Errorf("read the opt-out: %w", err)
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
		DedupID: fmt.Sprintf("%s:%d/%d", events.NameTelegramUnsubscribed, row.ID, c.ID),
	})
}
