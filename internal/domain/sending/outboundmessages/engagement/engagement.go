// Package engagement records an open or a click of an Outbound message (ADR 0053, ADR 0111): it
// resolves the token of the hit, judges whether a machine made it, and publishes the Event in its
// own transaction. The message's report stamp follows the Event through its consumer (ADR 0113).
//
// It is the part of outboundmessages that holds the raw client: an open or a click token names an
// Outbound message, and a short link's token a tracked link, before any scope exists, so the
// Workspace is read off that row (docs/architecture/workspace-scoping.md, Workspace scoping). The
// message's Contact is read through its owner, contacts, in that Workspace's scope.
package engagement

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/trackedlink"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/sending/messagetoken"
	"github.com/getsphericon/sphericon/internal/domain/sending/privaterelayranges"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
)

// ErrInvalidToken means the token of an open or a click does not decode.
var ErrInvalidToken = errors.New("engagement: invalid token")

// ErrNotFound means the token names no message or link here: an unknown short link, or one of
// another Workspace than the Tracking host the hit came in on (ADR 0038).
var ErrNotFound = errors.New("engagement: not found")

// Hit is one request to a tracked link.
type Hit struct {
	// UserAgent and Address judge whether a machine made the hit (ADR 0053); neither is kept.
	UserAgent, Address string
	// Host is the Workspace whose Tracking host the hit came in on, 0 on the Instance's own host
	// (ADR 0038).
	Host int64
	// MayRecord is asked once the hit is about to be recorded, and is false while the client
	// address has spent its tracking budget (ADR 0025): the hit is answered all the same and
	// recorded nowhere. It counts the hit against that budget, so a hit that would record nothing
	// anyway (a bad token, another Workspace's link, a Test send's) does not ask it.
	MayRecord func() bool
}

// Recorder records the opens and clicks of Outbound messages.
type Recorder struct {
	client   *ent.Client
	bus      *events.Bus
	contacts *contacts.Module
	tracker  *messagetoken.Tracker
	detector *privaterelayranges.Detector
	now      func() time.Time
}

// New builds the Recorder. people reads the Contact a message went to; now is the Instance clock a
// hit is observed at.
func New(client *ent.Client, bus *events.Bus, people *contacts.Module, tracker *messagetoken.Tracker, detector *privaterelayranges.Detector, now func() time.Time) *Recorder {
	return &Recorder{client: client, bus: bus, contacts: people, tracker: tracker, detector: detector, now: now}
}

// Open records an open of the message the token names. Every open is an Event of its own: a
// repeated open is a real engagement, and the message's stamp counts it once.
func (r *Recorder) Open(ctx context.Context, token string, hit Hit) error {
	messageID, err := r.tracker.Decode(token)
	if err != nil {
		return ErrInvalidToken
	}
	return r.record(ctx, messageID, hit, func(e engaged, machine bool, at time.Time) events.DomainEvent {
		return e.engagement(systemevent.EmailOpened, "", machine, at)
	})
}

// Click records a click on the link the token signs and returns that link.
func (r *Recorder) Click(ctx context.Context, token string, hit Hit) (string, error) {
	messageID, dest, err := r.tracker.DecodeClick(token)
	if err != nil {
		return "", ErrInvalidToken
	}
	if err := r.record(ctx, messageID, hit, clickOn(dest)); err != nil {
		return "", err
	}
	return dest, nil
}

// ShortLink records a click on the short tracked link of a Telegram message or an SMS (ADR 0083)
// and returns the URL stored with its token. A Test send's link has no message: it records nothing.
func (r *Recorder) ShortLink(ctx context.Context, token string, hit Hit) (string, error) {
	link, err := r.client.TrackedLink.Query().Where(trackedlink.Token(token)).Only(ctx)
	switch {
	case ent.IsNotFound(err):
		return "", ErrNotFound
	case err != nil:
		return "", err
	case hit.Host != 0 && hit.Host != link.WorkspaceID:
		return "", ErrNotFound
	case link.OutboundMessageID == nil:
		return link.URL, nil
	}
	hit.Host = 0 // the link's Workspace is the message's
	if err := r.record(ctx, *link.OutboundMessageID, hit, clickOn(link.URL)); err != nil {
		return "", err
	}
	return link.URL, nil
}

// record publishes the Event eventOf builds for a hit on the message. A hit on a Tracking host
// is refused unless the message is that host's Workspace's: a message that is gone, or another
// Workspace's, is ErrNotFound, and a read that fails cannot tell, so its error is returned: the
// driver logs it, answering an open its pixel and a click a 500 (ADR 0038). A message whose Contact is gone (erased), or whose
// address was cleared, records nothing silently; once the hit's Workspace is known, a read that
// fails for any other reason, or a failed publish, records nothing and is logged, since a tracked
// link never refuses its recipient (ADR 0025).
func (r *Recorder) record(ctx context.Context, messageID int64, hit Hit, eventOf func(e engaged, machine bool, at time.Time) events.DomainEvent) error {
	var (
		msg *ent.OutboundMessage
		err error
	)
	if hit.Host != 0 {
		msg, err = r.client.OutboundMessage.Get(ctx, messageID)
		switch {
		case ent.IsNotFound(err):
			return ErrNotFound
		case err != nil:
			return fmt.Errorf("engagement: read message %d: %w", messageID, err)
		case msg.WorkspaceID != hit.Host:
			return ErrNotFound
		}
	}
	if !hit.MayRecord() {
		return nil
	}
	if msg == nil {
		if msg, err = r.client.OutboundMessage.Get(ctx, messageID); err != nil {
			if !ent.IsNotFound(err) {
				logLookup(ctx, messageID, err)
			}
			return nil
		}
	}
	e, ok := r.engagedOf(ctx, msg)
	if !ok {
		return nil
	}
	machine := r.detector.IsMachine(ctx, hit.UserAgent, hit.Address)
	if err := r.bus.Publish(ctx, eventOf(e, machine, r.now())); err != nil {
		slog.ErrorContext(ctx, "engagement: recording failed", "message_id", messageID, "err", err)
	}
	return nil
}

// engaged is the Outbound message a hit names, with the Contact it went to when there is one. A
// Transactional send to an address with no Contact is kept by address.
type engaged struct {
	msg     *ent.OutboundMessage
	contact *ent.Contact // nil: the message has no Contact
	email   string
}

// engagedOf reads the message's Contact in the message's Workspace, which the message row the
// token names gives.
func (r *Recorder) engagedOf(ctx context.Context, msg *ent.OutboundMessage) (engaged, bool) {
	if msg.ContactID == nil {
		if msg.Destination == "" {
			return engaged{}, false
		}
		return engaged{msg: msg, email: msg.Destination}, true
	}
	c, err := r.contacts.Get(ctx, r.client.Scoped(msg.WorkspaceID), *msg.ContactID)
	if err != nil {
		if !errors.Is(err, contacts.ErrNotFound) {
			logLookup(ctx, msg.ID, err)
		}
		return engaged{}, false
	}
	return engaged{msg: msg, contact: c, email: lo.FromPtr(c.Email)}, true
}

// logLookup logs a failed lookup of a hit's message or Contact; a row that is simply gone is no
// failure, and its caller does not log it.
func logLookup(ctx context.Context, messageID int64, err error) {
	slog.ErrorContext(ctx, "engagement: lookup failed", "message_id", messageID, "err", err)
}

func (e engaged) contactID() int64 {
	if e.contact == nil {
		return 0 // kept by address
	}
	return e.contact.ID
}

func (e engaged) engagement(action, dest string, machine bool, at time.Time) *events.EmailEngagement {
	return &events.EmailEngagement{
		ObservedAt: at, Action: action, WorkspaceID: e.msg.WorkspaceID, ContactID: e.contactID(),
		Email: e.email, BroadcastID: lo.FromPtr(e.msg.BroadcastID), URL: dest,
		AutomationID:      lo.FromPtr(e.msg.AutomationID),
		OutboundMessageID: e.msg.ID,
		Machine:           machine,
	}
}

// clickOn builds the Event of a click on dest: the Channel's own, with the same facts.
func clickOn(dest string) func(e engaged, machine bool, at time.Time) events.DomainEvent {
	return func(e engaged, machine bool, at time.Time) events.DomainEvent {
		click := events.LinkClick{
			ObservedAt:  at,
			WorkspaceID: e.msg.WorkspaceID, ContactID: e.contactID(), Email: e.email,
			IntegrationID: lo.FromPtr(e.msg.IntegrationID), URL: dest,
			BroadcastID: lo.FromPtr(e.msg.BroadcastID), AutomationID: lo.FromPtr(e.msg.AutomationID),
			OutboundMessageID: e.msg.ID, Machine: machine,
		}
		switch e.msg.Channel {
		case channels.Telegram:
			return new(events.TelegramClicked(click))
		case channels.SMS:
			return new(events.SmsClicked(click))
		default:
			return e.engagement(systemevent.EmailClicked, dest, machine, at)
		}
	}
}
