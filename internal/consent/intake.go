package consent

import (
	"cmp"
	"context"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/internal/channels"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/linktarget"
	"github.com/getsphericon/sphericon/internal/optout"
	"github.com/getsphericon/sphericon/internal/suppression"
	"github.com/getsphericon/sphericon/internal/validation"
)

// Intake: consent changes made by an authenticated caller (the SPA, an API token,
// an agent over MCP) rather than by a signed link. The opt-outs here only narrow reach, so the
// agent may use them (ADR 0016); the one widening is the evidence of consent the customer
// collected (Confirm, ADR 0077), which never lifts an opt-out.

var (
	// ErrDestinationEmpty: the destination is blank after normalization.
	ErrDestinationEmpty = validation.RefuseField("destination", "consent: destination must not be empty", i18n.ErrorsDestinationEmpty)
	// ErrInvalidSource: the sending source is not broadcasts, everything or automation:<id>.
	ErrInvalidSource = validation.RefuseField("sendingSource", "consent: sendingSource must be broadcasts, everything or automation:<id>", i18n.ErrorsSendingSourceInvalid)
	// ErrDestinationInvalid: the destination is not what the Channel's Destinations are.
	ErrDestinationInvalid = suppression.ErrDestinationInvalid
	// ErrTelegramSource: a Telegram opt-out recorded by hand is Channel-wide.
	ErrTelegramSource = validation.RefuseField("sendingSource", "consent: a telegram opt-out is for everything", i18n.ErrorsSendingSourceTelegram)
	// ErrUnknownAutomation: the automation:<id> source names no Automation of this Workspace.
	ErrUnknownAutomation = validation.RefuseField("sendingSource", "consent: unknown automation in sendingSource", i18n.ErrorsSendingSourceAutomationUnknown)
)

// OptOut records that destination opted out on channel from a sending source, as an opt-out that
// reached the Workspace by any route: on email Unsubscribe, on telegram TelegramUnsubscribe, on sms
// SMSUnsubscribe. An empty source is the Channel's default: `broadcasts` on email, `everything` on
// telegram and sms, where a person stops all of the Workspace's messages. Idempotent.
func OptOut(ctx context.Context, bus *events.Bus, s *ent.Scoped, channel channels.Channel, destination, source string) (*ent.Unsubscribe, error) {
	switch channel {
	case channels.Telegram:
		return TelegramUnsubscribe(ctx, bus, s, destination, cmp.Or(source, eligibility.SourceEverything))
	case channels.SMS:
		return SMSUnsubscribe(ctx, bus, s, destination, cmp.Or(source, eligibility.SourceEverything))
	case channels.Email:
	}
	return Unsubscribe(ctx, bus, s, destination, cmp.Or(source, eligibility.SourceBroadcasts))
}

// Unsubscribe records an opt-out of destination from a sending source, with the
// same effects as the unsubscribe link (RecordUnsubscribe): the row, the
// confirmation reset on "everything", the automation exit and the engagement event,
// atomically and idempotently. When the destination is a Contact of the Workspace
// the event is attributed to it. The source is validated first.
func Unsubscribe(ctx context.Context, bus *events.Bus, s *ent.Scoped, destination, source string) (*ent.Unsubscribe, error) {
	dest := suppression.NormalizeDestination(destination)
	if dest == "" {
		return nil, ErrDestinationEmpty
	}
	if addr, err := mail.ParseAddress(dest); err != nil || addr.Address != dest {
		return nil, ErrDestinationInvalid
	}
	if err := validateSource(ctx, s, source); err != nil {
		return nil, err
	}

	target := linktarget.Unsubscribe{Channel: channels.Email, Source: source, Destination: dest, WorkspaceID: s.WorkspaceID()}
	if c, err := s.Contact().Query().Where(contact.EmailEQ(dest)).First(ctx); err == nil {
		target.ContactID = c.ID
	} else if !ent.IsNotFound(err) {
		return nil, err
	}
	if err := RecordUnsubscribe(ctx, bus, target); err != nil {
		return nil, err
	}
	return s.Unsubscribe().Query().Where(
		unsubscribe.ChannelEQ(channels.Email),
		unsubscribe.DestinationEQ(dest),
		unsubscribe.SendingSourceEQ(source),
	).Only(ctx)
}

// TelegramUnsubscribe records the Channel-wide opt-out of a Telegram user id, with the effects of
// the user stopping in Telegram (optout.StopTelegram): the row and telegram.unsubscribed for every
// Contact the id is Bound to. A manual opt-out is Channel-wide: only the `everything` source is
// accepted here; a source-scoped one comes from the link in a message (RecordUnsubscribe). Idempotent.
func TelegramUnsubscribe(ctx context.Context, bus *events.Bus, s *ent.Scoped, destination, source string) (*ent.Unsubscribe, error) {
	dest := strings.TrimSpace(destination)
	if dest == "" {
		return nil, ErrDestinationEmpty
	}
	if !suppression.ValidDestination("telegram", dest) {
		return nil, ErrDestinationInvalid
	}
	if source != eligibility.SourceEverything {
		return nil, ErrTelegramSource
	}
	now := time.Now()
	err := bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		return optout.StopTelegram(ctx, ts, pub, optout.TelegramStop{
			Destination: dest, Via: events.UnsubscribedByManual, Now: now,
			DedupKey: "manual:" + dest + ":" + strconv.FormatInt(now.UnixNano(), 10),
		})
	})
	if err != nil {
		return nil, err
	}
	return s.Unsubscribe().Query().Where(
		unsubscribe.ChannelEQ(channels.Telegram),
		unsubscribe.DestinationEQ(dest),
		unsubscribe.SendingSourceEQ(source),
	).Only(ctx)
}

func validateSource(ctx context.Context, s *ent.Scoped, source string) error {
	automationID, isAutomation := eligibility.ParseAutomationSource(source)
	switch {
	case source == eligibility.SourceBroadcasts || source == eligibility.SourceEverything:
		return nil
	case isAutomation:
		exists, err := s.Automation().QueryWithArchived().Where(automation.ID(automationID)).Exist(ctx)
		if err != nil {
			return err
		}
		if !exists {
			return ErrUnknownAutomation
		}
		return nil
	default:
		return ErrInvalidSource
	}
}
