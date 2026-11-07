// Package contacterasure is the use case that erases a Contact's personal data (GDPR Art. 17,
// ADR 0021, ADR 0102). Deleting a Contact is Erasure: there is no softer variant. Every surface
// (the /site SPA API, the external /api and so MCP) hands it a Workspace-scoped client
// and an Identifier and gets back nil or a domain error; none re-implements what is
// removed, anonymized or kept.
//
// The whole erasure is one transaction, opened through events.Bus.WithinScopedPurgeTx so
// the Contact change, its events and the purge of its queued jobs commit together. Inside it the work
// is an ordered list of steps (see UseCase.steps), each a call to the module that owns the rows over
// the resolved Target: the use case reads the tables, and changes each only through its owner's
// erase function. Extending Erasure (another identifier kind, another dependent table, another
// signal published in the same transaction) means resolving the new identifier into a Target or
// adding one step, not a second code path. The Contact row goes last, through the contacts module's
// unchecked removal contacts.DeleteIn, which easement forbids every other package to call. The
// record of an Erasure (the Erasures stream, ADR 0045) is written from the contact.erased Event by
// its own consumer, not here.
package contacterasure

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/capturedmessage"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/visitor"
	"github.com/getsphericon/sphericon/internal/domain/contacts/confirmations"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contactlock"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/contacts/visitors"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/domain/sending/bindings"
	"github.com/getsphericon/sphericon/internal/domain/sending/capturedmessages"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// The refusals of an Erasure, one per meaning.
var (
	// ErrContactNotFound means the Contact id names no Contact in the Workspace.
	ErrContactNotFound = validation.NotFound("erasure: contact not found", i18n.ErrorsContactNotFound)
	// ErrNothingFound means an address or a visitor id resolves to nothing in the Workspace:
	// neither a Contact nor anything that refers to it.
	ErrNothingFound = validation.NotFound("erasure: nothing found for the identifier", i18n.ErrorsErasureNothingFound)
	// ErrIdentifier means a request named neither or both of an address and a visitor id, or an
	// empty visitor id.
	ErrIdentifier = validation.Refuse("erasure: exactly one of email or visitorId is required", i18n.ErrorsErasureIdentifier)
)

// Identifier names the data subject to erase: exactly one of a Contact's id, an email address
// and a visitor id, an absent one nil. An address with no Contact identifies the address itself,
// so delivery records to it are anonymized; a visitor id names the Contact the visitor is bound
// to or, for an anonymous Visitor with no Contact, the Visitor and its Events.
type Identifier struct {
	ContactID int64
	Email     *string
	VisitorID *string
}

// subject is the checked Identifier Erase resolves: one of its fields is set, an address in its
// canonical form.
type subject struct {
	ContactID int64
	Email     string
	VisitorID string
}

// checked is the subject an Identifier names: exactly one of its fields is given (ErrIdentifier
// otherwise, and for an empty visitor id, which names nothing), and an address is refused
// (ErrInvalidEmail) unless it is a single plain address.
func (i Identifier) checked() (subject, error) {
	given := lo.Count([]bool{i.ContactID != 0, i.Email != nil, i.VisitorID != nil}, true)
	if given != 1 || (i.VisitorID != nil && *i.VisitorID == "") {
		return subject{}, ErrIdentifier
	}
	out := subject{ContactID: i.ContactID, VisitorID: lo.FromPtr(i.VisitorID)}
	if i.Email != nil {
		email, err := normalizeEmail(*i.Email)
		if err != nil {
			return subject{}, err
		}
		out.Email = email
	}
	return out, nil
}

// kind names the identifier for the contact.erased Event.
func (i subject) kind() string {
	switch {
	case i.Email != "":
		return "email"
	case i.VisitorID != "":
		return "visitor_id"
	}
	return "contact_id"
}

// Requester is who asked for the Erasure: a User (Site) or an API token (External, MCP).
type Requester struct {
	// Kind is RequesterUser or RequesterAPIToken.
	Kind string
	ID   int64
}

// Requester kinds, as recorded on the contact.erased Event.
const (
	RequesterUser     = "user"
	RequesterAPIToken = "api_token"
)

// Target is a resolved data subject: what every step erases. Resolving an Identifier
// into a Target is the only place the identifier kind matters.
type Target struct {
	// ContactID is the Contact being erased; zero for an address or an anonymous
	// Visitor that has no Contact.
	ContactID int64
	// VisitorIDs are the anonymous devices bound to the Contact; their Events are
	// erased with it.
	VisitorIDs []string
	// Destinations are the Contact's addresses.
	Destinations []string
	// Phone is the Contact's phone number; empty when it has none or there is no Contact.
	Phone string
	// SubjectID is the customer's own id for the person (the Contact's subject_id, or
	// its sphericon id when it has none); only the contact.erased delivery carries it.
	// Empty for an address alone: the address is never echoed into the message.
	SubjectID string
	// IdentifierKind is what the Identifier was.
	IdentifierKind string
	// Requester is who asked.
	Requester Requester
}

// destinations are the Target's addresses and its phone number: what a delivery record or a queued
// message names the subject by besides its Contact id.
func (t *Target) destinations() []string {
	if t.Phone == "" {
		return t.Destinations
	}
	return append(slices.Clone(t.Destinations), t.Phone)
}

// step is one part of the erasure, run inside the transaction.
type step func(ctx context.Context, s *ent.Scoped, t *Target, pub events.PurgingPublisher) error

// UseCase erases data subjects. Every call receives the Workspace-scoped client (ADR 0017) built by
// the entry point; the use case opens the transaction the modules it calls join.
type UseCase struct {
	bus *events.Bus
	now func() time.Time
}

// New builds the Contact erasure use case over the bus whose transaction every erasure runs in;
// now is the Instance clock a Broadcast left with nothing pending settles by.
func New(bus *events.Bus, now func() time.Time) *UseCase {
	return &UseCase{bus: bus, now: now}
}

// Erase erases the identified data subject in one transaction: either all of it
// happens or none. ErrContactNotFound (a Contact id) or ErrNothingFound (an address or a
// visitor id) means the identifier resolves to nothing in the Workspace (including a row of
// another Workspace).
func (u *UseCase) Erase(ctx context.Context, s *ent.Scoped, id Identifier, by Requester) error {
	subj, err := id.checked()
	if err != nil {
		return err
	}
	return u.bus.WithinScopedPurgeTx(ctx, s, func(ts *ent.Scoped, pub events.PurgingPublisher) error {
		target, err := resolve(ctx, ts, subj)
		if err != nil {
			return err
		}
		target.Requester = by
		for _, run := range u.steps() {
			if err := run(ctx, ts, target, pub); err != nil {
				return err
			}
		}
		return nil
	})
}

// steps is the ordered erasure: first the cancellation of work still queued for the
// subject, then the deletions, then the signal. Each owner of pending work cancels it itself, and
// each owner of the subject's rows erases them itself; Erasure only sequences them. A send a worker already handed to the
// provider is not recalled; one queued but not started finds its row gone and, at the
// send chokepoint, its Contact gone (ADR 0021).
//
// The cancellations come before every step that deletes the rows their ids are read from
// (the Contact's runs, recipients and the Contact itself) and before the signal, which
// names no Contact. The Contact row goes last of the deletions: the other steps find the
// Contact's rows through ids already resolved into the Target.
func (u *UseCase) steps() []step {
	return []step{
		// The Broadcast delivery module cancels the Contact's unsent recipients; a Broadcast left
		// with nothing pending settles.
		func(ctx context.Context, s *ent.Scoped, t *Target, pub events.PurgingPublisher) error {
			return broadcasts.CancelPending(ctx, s, t.ContactID, pub, u.now())
		},
		// The Automation module cancels the queued step calls and trigger evaluations that name the
		// Contact or its runs (a failure's response excerpt sits on the run, which goes below).
		func(ctx context.Context, s *ent.Scoped, t *Target, pub events.PurgingPublisher) error {
			return automations.CancelPending(ctx, s, t.ContactID, pub)
		},
		// The Rule module cancels the queued evaluations that name the Contact.
		func(ctx context.Context, s *ent.Scoped, t *Target, pub events.PurgingPublisher) error {
			return rules.CancelPending(ctx, s, t.ContactID, pub)
		},
		// The queued event consumer jobs whose envelope names the subject: the bus inserted them,
		// and the purge shares Erasure's transaction.
		func(ctx context.Context, s *ent.Scoped, t *Target, pub events.PurgingPublisher) error {
			return pub.PurgeEventJobs(ctx, s.WorkspaceID(), t.ContactID, t.destinations())
		},
		// The queued webhook deliveries whose payload names the subject; the event envelope's
		// owner knows that payload shape.
		func(ctx context.Context, s *ent.Scoped, t *Target, pub events.PurgingPublisher) error {
			return pub.PurgeWebhookJobs(ctx, s.WorkspaceID(), t.ContactID, t.destinations())
		},
		// The Outbound message module deletes the short links of the Contact's messages, before
		// anonymizeDelivery clears the contact_id they are found by.
		func(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
			return outboundmessages.EraseShortLinks(ctx, s, t.ContactID)
		},
		// The Binding module deletes the Bindings and Start tokens of the Contact and of its
		// Visitors, so before the Visitors go. Telegram opt-outs are not here: they are keyed by
		// the user id and stay (detachOptOuts).
		func(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
			return bindings.EraseSubject(ctx, s, t.ContactID, t.VisitorIDs)
		},
		// The bus, which stores the Events, deletes the subject's customer-tracked Events and
		// anonymizes its system Events in place.
		func(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
			return events.EraseSubject(ctx, s, t.ContactID, t.Phone, t.VisitorIDs)
		},
		func(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
			return visitors.EraseSubject(ctx, s, t.ContactID, t.VisitorIDs)
		},
		// The Confirmation and its evidence; losing it fails safe (re-confirmation).
		func(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
			return confirmations.EraseContact(ctx, s, t.ContactID, t.Phone)
		},
		func(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
			return automations.EraseRuns(ctx, s, t.ContactID)
		},
		func(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
			return rules.EraseFirings(ctx, s, t.ContactID)
		},
		detachOptOuts,
		anonymizeDelivery,
		// The Captured messages (Test inbox, ADR 0058) addressed to the subject's Destinations.
		func(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
			return capturedmessages.EraseTo(ctx, s, t.Destinations)
		},
		// The Contact itself; its Tag links and Custom field values go with the row.
		func(ctx context.Context, s *ent.Scoped, t *Target, _ events.PurgingPublisher) error {
			return contacts.DeleteIn(ctx, s, t.ContactID)
		},
		publishErased,
	}
}

func resolve(ctx context.Context, s *ent.Scoped, id subject) (*Target, error) {
	c, err := findContact(ctx, s, id)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return resolveContactless(ctx, s, id)
	}
	seen, err := s.Visitor().Query().Where(visitorOf(c.ID)).All(ctx)
	if err != nil {
		return nil, err
	}
	t := &Target{ContactID: c.ID, IdentifierKind: id.kind(), SubjectID: strconv.FormatInt(c.ID, 10)}
	if c.SubjectID != nil && *c.SubjectID != "" {
		t.SubjectID = *c.SubjectID
	}
	for _, v := range seen {
		t.VisitorIDs = append(t.VisitorIDs, v.VisitorID)
	}
	if c.Email != nil {
		t.Destinations = append(t.Destinations, strings.ToLower(*c.Email))
	}
	if c.Phone != nil {
		t.Phone = *c.Phone
	}
	return t, nil
}

// findContact returns the Contact the identifier names, locked to the end of the transaction s is
// bound to (contactlock), or nil when it names none (an address or Visitor without a Contact). A
// contact id that resolves to nothing is ErrContactNotFound.
func findContact(ctx context.Context, s *ent.Scoped, id subject) (*ent.Contact, error) {
	var (
		c   *ent.Contact
		err error
	)
	switch {
	case id.Email != "":
		c, err = s.Contact().Query().Where(contact.Email(id.Email)).Only(ctx)
	case id.VisitorID != "":
		var v *ent.Visitor
		v, err = s.Visitor().Query().Where(visitor.VisitorID(id.VisitorID)).Only(ctx)
		if err == nil && v.ContactID != nil {
			c, err = s.Contact().Get(ctx, *v.ContactID)
		}
	default:
		c, err = s.Contact().Get(ctx, id.ContactID)
		if ent.IsNotFound(err) {
			return nil, ErrContactNotFound
		}
	}
	if ent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil || c == nil {
		return nil, err
	}
	// Erasure deletes the Contact's Enrollments before the Contact, so it locks the Contact
	// first, in the order every writer of both rows takes them (contactlock, #811).
	locked, err := contactlock.Lock(ctx, s, c.ID)
	if ent.IsNotFound(err) {
		return findContact(ctx, s, id) // erased meanwhile: resolve what the identifier names now
	}
	return locked, err
}

// resolveContactless resolves an email or visitor id that has no Contact: it is a
// subject only if something in the Workspace refers to it.
func resolveContactless(ctx context.Context, s *ent.Scoped, id subject) (*Target, error) {
	var (
		t     *Target
		known bool
		err   error
	)
	switch {
	case id.Email != "":
		t = &Target{Destinations: []string{id.Email}}
		known, err = s.OutboundMessage().Query().Where(outboundmessage.Destination(id.Email)).Exist(ctx)
		if err == nil && !known {
			known, err = s.CapturedMessage().Query().Where(capturedmessage.Recipient(id.Email)).Exist(ctx)
		}
	case id.VisitorID != "":
		t = &Target{VisitorIDs: []string{id.VisitorID}, SubjectID: id.VisitorID}
		known, err = s.Visitor().Query().Where(visitor.VisitorID(id.VisitorID)).Exist(ctx)
		if err == nil && !known {
			known, err = s.Event().Query().Where(event.VisitorID(id.VisitorID)).Exist(ctx)
		}
	}
	if err != nil {
		return nil, err
	}
	if !known {
		return nil, ErrNothingFound
	}
	t.IdentifierKind = id.kind()
	return t, nil
}
