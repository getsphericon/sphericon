package unsubscriberecording

import (
	"cmp"
	"context"
	"net/mail"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/domain/contacts/channelstop"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/contacts/suppressions"
	"github.com/getsphericon/sphericon/internal/domain/contacts/unsubscribes"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/linktarget"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// Intake: opt-outs recorded by an authenticated caller (an API token, an agent over MCP) rather
// than by a signed link. They only narrow reach, so the agent may use them (ADR 0016); the one
// widening is the evidence of consent the customer collected (confirmations.Confirm, ADR 0077),
// which never lifts an opt-out.

var (
	// ErrInvalidSource: the sending source is not broadcasts, everything or automation:<id>.
	ErrInvalidSource = validation.RefuseField("sendingSource", "consent: sendingSource must be broadcasts, everything or automation:<id>", i18n.ErrorsSendingSourceInvalid)
	// ErrUnknownAutomation: the automation:<id> source names no Automation of this Workspace.
	ErrUnknownAutomation = validation.RefuseField("sendingSource", "consent: unknown automation in sendingSource", i18n.ErrorsSendingSourceAutomationUnknown)
)

// OptOut records that destination opted out on channel from a sending source, as an opt-out that
// reached the Workspace by any route: on email Unsubscribe, on telegram channelstop.TelegramUnsubscribe,
// on sms channelstop.SMSUnsubscribe. An empty source is the Channel's default: `broadcasts` on email,
// `everything` on telegram and sms, where a person stops all of the Workspace's messages.
// Idempotent.
func (u *UseCase) OptOut(ctx context.Context, s *ent.Scoped, channel channels.Channel, destination, source string) (*ent.Unsubscribe, error) {
	switch channel {
	case channels.Telegram:
		return channelstop.TelegramUnsubscribe(ctx, u.bus, s, destination, cmp.Or(source, eligibility.SourceEverything))
	case channels.SMS:
		return channelstop.SMSUnsubscribe(ctx, u.bus, s, destination, cmp.Or(source, eligibility.SourceEverything))
	case channels.Email:
	}
	return u.Unsubscribe(ctx, s, destination, cmp.Or(source, eligibility.SourceBroadcasts))
}

// Unsubscribe records an opt-out of destination from a sending source, with the
// same effects as the unsubscribe link (RecordLink): the row, the
// confirmation reset on "everything", the automation exit and the engagement event,
// atomically and idempotently. When the destination is a Contact of the Workspace
// the event is attributed to it. The source is validated first.
func (u *UseCase) Unsubscribe(ctx context.Context, s *ent.Scoped, destination, source string) (*ent.Unsubscribe, error) {
	dest := suppressions.NormalizeDestination(destination)
	if dest == "" {
		return nil, unsubscribes.ErrDestinationEmpty
	}
	if addr, err := mail.ParseAddress(dest); err != nil || addr.Address != dest {
		return nil, unsubscribes.ErrDestinationInvalid
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
	if err := u.RecordLink(ctx, s, target); err != nil {
		return nil, err
	}
	return unsubscribes.Get(ctx, s, channels.Email, dest, source)
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
