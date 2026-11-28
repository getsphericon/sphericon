package unsubscriberecording

import (
	"cmp"
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/domain/contacts/channelstop"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contactimports"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/contacts/unsubscribes"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/fieldname"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/linktarget"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// Intake: opt-outs recorded by an authenticated caller (an API token, an agent over MCP) rather
// than by a signed link. They only narrow reach, so the agent may use them (ADR 0016); the one
// widening is the evidence of consent the customer collected (the confirmations' Confirm, ADR 0077),
// which never lifts an opt-out.

var (
	// ErrInvalidSource: the sending source is not broadcasts, everything or automation:<id>.
	ErrInvalidSource = validation.RefuseField(fieldname.SendingSource, "consent: sendingSource must be broadcasts, everything or automation:<id>", i18n.ErrorsSendingSourceInvalid)
	// ErrUnknownAutomation: the automation:<id> source names no Automation of this Workspace.
	ErrUnknownAutomation = validation.RefuseField(fieldname.SendingSource, "consent: unknown automation in sendingSource", i18n.ErrorsSendingSourceAutomationUnknown)
)

// OptOutInput is an opt-out that reached the Workspace by any route: the Channel, the destination
// that opted out and the sending source it opted out of (empty is the Channel's default), and, for
// an internal caller only, how and when it was made.
type OptOutInput struct {
	Channel     channels.Channel
	Destination string
	Source      string
	// Via is how the opt-out was made: events.UnsubscribedByImported for one a CSV import brings
	// (ADR 0138). Empty is events.UnsubscribedByManual, the way of an opt-out recorded by hand
	// through the API, which never sets it.
	Via string
	// At is when the person opted out (ADR 0154), stated by an import. The zero time, and one
	// after now, is now: a refusal is never dated ahead of the clock that orders it. The API
	// never sets it.
	At time.Time
}

// OptOut records that the destination opted out on the Channel from a sending source: on email
// Unsubscribe's effects, on telegram the Channel stops' TelegramUnsubscribe, on sms their
// SMSUnsubscribe. An empty source is the Channel's default: `broadcasts` on email, `everything` on
// telegram and sms, where a person stops all of the Workspace's messages. Idempotent.
func (u *UseCase) OptOut(ctx context.Context, s *ent.Scoped, in OptOutInput) (*ent.Unsubscribe, error) {
	st := stop{via: cmp.Or(in.Via, events.UnsubscribedByManual), at: u.now()}
	if !in.At.IsZero() && in.At.Before(st.at) {
		st.at = in.At
	}
	switch in.Channel {
	case channels.Telegram:
		return u.stops.TelegramUnsubscribe(ctx, s, channelstop.HandStop{
			Destination: in.Destination, Source: cmp.Or(in.Source, eligibility.SourceEverything), Via: st.via, At: st.at,
		})
	case channels.SMS:
		return u.stops.SMSUnsubscribe(ctx, s, channelstop.HandStop{
			Destination: in.Destination, Source: cmp.Or(in.Source, eligibility.SourceEverything), Via: st.via, At: st.at,
		})
	case channels.Email:
	}
	return u.unsubscribe(ctx, s, in.Destination, cmp.Or(in.Source, eligibility.SourceBroadcasts), st)
}

// OptOutImported records an opt-out a CSV import brings (ADR 0138): OptOut `via: imported`, at the
// time the import states. It reports whether the opt-out stands: one older than the Destination's
// return changes nothing (ADR 0154), and leaves no opt-out to read back.
func (u *UseCase) OptOutImported(ctx context.Context, s *ent.Scoped, in contactimports.ImportedOptOut) (bool, error) {
	_, err := u.OptOut(ctx, s, OptOutInput{
		Channel: in.Channel, Destination: in.Destination, Source: in.Source, Via: events.UnsubscribedByImported, At: in.At,
	})
	if ent.IsNotFound(err) {
		return false, nil
	}
	return err == nil, err
}

var _ contactimports.OptOuts = (*UseCase)(nil)

// Unsubscribe records an opt-out of an email address from a sending source by hand, now (OptOut on
// email).
func (u *UseCase) Unsubscribe(ctx context.Context, s *ent.Scoped, destination, source string) (*ent.Unsubscribe, error) {
	return u.unsubscribe(ctx, s, destination, source, stop{via: events.UnsubscribedByManual, at: u.now()})
}

// unsubscribe records an opt-out of destination from a sending source, made as st says, with the
// same effects as the unsubscribe link (RecordLink): the row, the confirmation reset on
// "everything", the automation exit and the engagement event, atomically and idempotently. When
// the destination is a Contact of the Workspace the event is attributed to it. The source is
// validated first.
func (u *UseCase) unsubscribe(ctx context.Context, s *ent.Scoped, destination, source string, st stop) (*ent.Unsubscribe, error) {
	dest, err := unsubscribes.Destination(channels.Email, destination)
	if err != nil {
		return nil, err
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
	if err := u.record(ctx, s, target, st); err != nil {
		return nil, err
	}
	return u.unsubs.Get(ctx, s, unsubscribes.Key{Channel: channels.Email, Destination: dest, Source: source})
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
