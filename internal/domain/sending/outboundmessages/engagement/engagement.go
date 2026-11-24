// Package engagement records an open or a click of an Outbound message (ADR 0053, ADR 0111): it
// resolves the token of the hit, checks that the message is tracked and that its Destination still
// consents to being tracked (ADR 0120), judges whether a machine made it, and publishes the Event in
// the transaction of that check. The message's report stamp follows the Event through its consumer
// (ADR 0113).
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
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/trackedlink"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking"
	"github.com/getsphericon/sphericon/internal/domain/sending/messagetoken"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
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
	consents *engagementtracking.Module
	tracker  *messagetoken.Tracker
	detector *privaterelayranges.Detector
	now      func() time.Time
}

// New builds the Recorder. people reads the Contact a message went to and consents whether its
// Destination is tracked; now is the Instance clock a hit is observed at.
func New(client *ent.Client, bus *events.Bus, people *contacts.Module, consents *engagementtracking.Module, tracker *messagetoken.Tracker, detector *privaterelayranges.Detector, now func() time.Time) *Recorder {
	return &Recorder{client: client, bus: bus, contacts: people, consents: consents, tracker: tracker, detector: detector, now: now}
}

// Open records an open of the message the token names. Every open is an Event of its own: a
// repeated open is a real engagement, and the message's stamp counts it once.
func (r *Recorder) Open(ctx context.Context, token string, hit Hit) error {
	m, err := r.tracker.Decode(token)
	if err != nil {
		return ErrInvalidToken
	}
	return r.record(ctx, m, hit, privaterelayranges.AnOpen, func(e engaged, machine bool, at time.Time) events.DomainEvent {
		return e.engagement(systemevent.EmailOpened, "", machine, at)
	})
}

// Click records a click on the link the token signs and returns that link.
func (r *Recorder) Click(ctx context.Context, token string, hit Hit) (string, error) {
	m, dest, err := r.tracker.DecodeClick(token)
	if err != nil {
		return "", ErrInvalidToken
	}
	if err := r.record(ctx, m, hit, privaterelayranges.AClick, clickOn(dest)); err != nil {
		return "", err
	}
	return dest, nil
}

// ShortLink records a click on the short tracked link of a Telegram message or an SMS (ADR 0083)
// and returns the URL stored with its token. A Test send's link has no message: it records nothing.
// A link that is not there is ErrNotFound: its URL went with it, whether it never existed or
// message retention deleted it with its message (ADR 0143).
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
	m := messagetoken.Message{WorkspaceID: link.WorkspaceID, ID: *link.OutboundMessageID}
	if err := r.record(ctx, m, hit, privaterelayranges.AClick, clickOn(link.URL)); err != nil {
		return "", err
	}
	return link.URL, nil
}

// record publishes the Event eventOf builds for a hit on the message m names. A hit on a Tracking
// host is refused as ErrNotFound unless m is that host's Workspace's (ADR 0038), which the signed
// token says without a read. Nothing is recorded, silently, for a message that is not tracked
// (outboundmessages.IsTracked) or whose Destination's effective consent is no longer to track (ADR
// 0120), and for one that is gone (message retention deleted it, ADR 0143), whose Contact is gone
// (erased) or whose Destination was cleared; a read that fails for any other reason, or a failed
// publish, records nothing and is logged, since a tracked link never refuses its recipient (ADR
// 0025).
//
// The check and the publish are one transaction that holds the message's row FOR SHARE: a
// withdrawal of the message's tracking, which takes the row for its update, either waits for the
// Event to commit (and then removes it) or commits first, and the check sees it.
func (r *Recorder) record(ctx context.Context, m messagetoken.Message, hit Hit, kind privaterelayranges.Kind, eventOf func(e engaged, machine bool, at time.Time) events.DomainEvent) error {
	if hit.Host != 0 && hit.Host != m.WorkspaceID {
		return ErrNotFound
	}
	if !hit.MayRecord() {
		return nil
	}
	// Judged before the transaction: the detector reads its ranges through a connection of its own.
	machine := r.detector.IsMachine(ctx, privaterelayranges.IsMachineInput{Kind: kind, UserAgent: hit.UserAgent, ClientAddr: hit.Address})
	err := r.bus.WithinScopedTx(ctx, r.client.Scoped(m.WorkspaceID), func(ts *ent.Scoped, pub events.Publisher) error {
		e, ok, err := r.engagedOf(ctx, ts, m.ID)
		if err != nil || !ok {
			return err
		}
		return pub.Publish(ctx, eventOf(e, machine, r.now()))
	})
	if err != nil {
		slog.ErrorContext(ctx, "engagement: recording failed", "message_id", m.ID, "err", err)
	}
	return nil
}

// engaged is the Outbound message a hit names, with the Contact it went to when there is one. A
// Transactional send to an address with no Contact is kept by address. email is the message's own
// Destination on email, never the Contact's current address.
type engaged struct {
	msg     *ent.OutboundMessage
	contact *ent.Contact // nil: the message has no Contact
	email   string
}

// engagedOf reads the message id names, locked FOR SHARE in the transaction ts is bound to, and
// its Contact, and says whether a hit on it may be recorded: the message is there and tracked, it
// has a Destination whose effective consent is to track, and its Contact, when it has one, is
// still there. Not found anywhere is no failure.
func (r *Recorder) engagedOf(ctx context.Context, ts *ent.Scoped, id int64) (engaged, bool, error) {
	msg, err := ts.OutboundMessage().Query().Where(outboundmessage.ID(id)).ForShare().Only(ctx)
	if ent.IsNotFound(err) {
		return engaged{}, false, nil
	}
	if err != nil {
		return engaged{}, false, fmt.Errorf("read the message: %w", err)
	}
	if !outboundmessages.IsTracked(msg) || msg.Destination == "" {
		return engaged{}, false, nil
	}
	consent, err := r.consents.Of(ctx, ts, msg.Channel, msg.Destination)
	if err != nil {
		return engaged{}, false, err
	}
	if consent.Effective != engagementtracking.Track {
		return engaged{}, false, nil
	}
	e := engaged{msg: msg}
	if msg.Channel == channels.Email {
		e.email = msg.Destination
	}
	if msg.ContactID == nil {
		return e, true, nil
	}
	c, err := r.contacts.Get(ctx, ts, *msg.ContactID)
	if errors.Is(err, contacts.ErrNotFound) {
		return engaged{}, false, nil
	}
	if err != nil {
		return engaged{}, false, fmt.Errorf("read the message's contact: %w", err)
	}
	e.contact = c
	if msg.Channel != channels.Email {
		// An SMS or a Telegram click names the Contact's address, as it always has (the Outbound
		// message holds the phone or the Telegram user).
		e.email = lo.FromPtr(c.Email)
	}
	return e, true, nil
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
