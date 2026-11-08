// Package unsubscriberecording records an Unsubscribe together with mandatory consent and
// Enrollment changes. Attribution to a received Broadcast message follows through the internal
// event's anonymous consumer (ADR 0115), independently of that safety transaction.
package unsubscriberecording

import (
	"context"
	"fmt"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/internal/domain/contacts/confirmations"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/contacts/suppressions"
	"github.com/getsphericon/sphericon/internal/domain/contacts/unsubscribes"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
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
// "everything" is the deliberate escalation. The opt-out, automation exit, published facts
// and durable work for anonymous attribution live in one
// transaction gated on the existence check, so a repeated POST (mailbox retry or a
// double click) is a complete no-op and concurrent POSTs are counted exactly once. s is the scope
// of the token's Workspace, which the entry point builds from it; the writes go through it as the
// ingest actor (never audited), since no membership or API token stands behind them.
func (u *UseCase) RecordLink(ctx context.Context, s *ent.Scoped, target linktarget.Unsubscribe) error {
	dest := suppressions.NormalizeDestination(target.Destination)
	if dest == "" || target.WorkspaceID == 0 || target.Source == "" || target.Channel == "" {
		return nil
	}
	channel := target.Channel
	automationID, isAutomation := eligibility.ParseAutomationSource(target.Source)
	at := u.now()

	// The Workspace comes from the signed unsubscribe token (no membership or api
	// token here), so this is a scope source outside the site/external/job list.
	return u.bus.WithinScopedTx(ctx, events.Ingest(s), func(sc *ent.Scoped, pub events.Publisher) error {
		erased, err := workspaces.ErasedIn(ctx, sc)
		if err != nil || erased {
			return err
		}
		if target.Channel == channels.SMS {
			return publishSMSStop(ctx, sc, pub, target, dest, at)
		}
		o := unsubscribes.OptOut{Channel: channel, Destination: dest, Source: target.Source, ContactID: target.ContactID}
		if target.Channel == channels.Telegram {
			// On email the email.unsubscribed Event says how; on Telegram the row does.
			o.Via = unsubscribe.ViaLink
		}
		optOutID, err := unsubscribes.Add(ctx, sc, o)
		if err != nil || optOutID == 0 {
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

		if err := publishAttribution(ctx, pub, target, at); err != nil {
			return err
		}

		if target.Channel == channels.Telegram {
			return publishTelegramUnsubscribed(ctx, sc, pub, target, optOutID)
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

// publishTelegramUnsubscribed records telegram.unsubscribed for the Contact a Telegram unsubscribe
// link was issued to (the opt-out itself is the row optOutID written above). A link with no
// Contact, or one whose Contact is gone, has nobody to name. The Event is keyed by the opt-out
// row, not the link: a `/start` lifts an opt-out of everything, and a later click on the same link
// is a new opt-out with a new Event, while its source id names neither the user nor the link.
func publishTelegramUnsubscribed(ctx context.Context, sc *ent.Scoped, pub events.Publisher, target linktarget.Unsubscribe, optOutID int64) error {
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
		DedupID: fmt.Sprintf("opt-out:%d", optOutID),
	})
}

// publishSMSStop reports the stop of the number whose text's link was followed; the opt-out and its
// effects are its subscriber's (ADR 0114). The link names the Contact the text was sent to and the
// text's Broadcast but not its Automation, since an SMS link is for everything, so the Broadcast and
// the Automation are read from the text.
func publishSMSStop(ctx context.Context, sc *ent.Scoped, pub events.Publisher, target linktarget.Unsubscribe, dest string, at time.Time) error {
	stop := &events.SmsStopObserved{
		WorkspaceID: target.WorkspaceID, Phone: dest, Via: events.UnsubscribedByLink,
		OutboundMessageID: target.OutboundMessageID, ContactID: target.ContactID, ObservedAt: at,
	}
	if target.OutboundMessageID != 0 {
		origin, err := outboundmessages.FindOrigin(ctx, sc, target.OutboundMessageID)
		if err != nil {
			return err
		}
		stop.BroadcastID, stop.AutomationID = origin.BroadcastID, origin.AutomationID
	}
	return pub.Publish(ctx, stop)
}

// publishAttribution records a real opt-out attributed to the message whose signed link was
// followed. The deferred accounting fact contains no personal data and outlives Contact Erasure.
func publishAttribution(ctx context.Context, pub events.Publisher, target linktarget.Unsubscribe, at time.Time) error {
	if target.OutboundMessageID == 0 {
		return nil
	}
	return pub.Publish(ctx, &events.MessageUnsubscribed{WorkspaceID: target.WorkspaceID, OutboundMessageID: target.OutboundMessageID, ObservedAt: at})
}
