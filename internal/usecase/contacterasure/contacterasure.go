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

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/capturedmessage"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/visitor"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contactlock"
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

// Identifier names the data subject to erase. Build one with ByContactID, ByEmail or
// ByVisitorID.
type Identifier struct {
	contactID int64
	email     string
	visitorID string
}

// ByContactID identifies a Contact by its id.
func ByContactID(id int64) Identifier { return Identifier{contactID: id} }

// ByEmail identifies a Contact by its email address; with no such Contact it
// identifies the address itself, so delivery records to it are anonymized.
func ByEmail(email string) Identifier {
	return Identifier{email: canonicalEmail(email)}
}

// ByVisitorID identifies the Contact the visitor is bound to, or, for an anonymous
// Visitor with no Contact, the Visitor and its Events.
func ByVisitorID(visitorID string) Identifier { return Identifier{visitorID: visitorID} }

// ByEmailOrVisitorID is the identifier of a request that names exactly one of an address and a
// visitor id (nil when absent); anything else is ErrIdentifier.
func ByEmailOrVisitorID(email, visitorID *string) (Identifier, error) {
	switch {
	case email != nil && visitorID == nil:
		return ByEmail(*email), nil
	case email == nil && visitorID != nil && *visitorID != "":
		return ByVisitorID(*visitorID), nil
	}
	return Identifier{}, ErrIdentifier
}

// kind names the identifier for the contact.erased Event.
func (i Identifier) kind() string {
	switch {
	case i.email != "":
		return "email"
	case i.visitorID != "":
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
}

// New builds the Contact erasure use case over the bus whose transaction every erasure runs in.
func New(bus *events.Bus) *UseCase {
	return &UseCase{bus: bus}
}

// Erase erases the identified data subject in one transaction: either all of it
// happens or none. ErrContactNotFound (a Contact id) or ErrNothingFound (an address or a
// visitor id) means the identifier resolves to nothing in the Workspace (including a row of
// another Workspace).
func (u *UseCase) Erase(ctx context.Context, s *ent.Scoped, id Identifier, by Requester) error {
	return u.bus.WithinScopedPurgeTx(ctx, s, func(ts *ent.Scoped, pub events.PurgingPublisher) error {
		target, err := resolve(ctx, ts, id)
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
		cancelBroadcastWork,
		cancelAutomationWork,
		cancelRuleWork,
		purgeEventJobs,
		purgeWebhookDeliveries,
		eraseShortLinks,
		eraseTelegram,
		eraseEvents,
		eraseVisitors,
		eraseConfirmations,
		eraseRuns,
		eraseFirings,
		detachOptOuts,
		anonymizeDelivery,
		eraseCapturedMessages,
		eraseContact,
		publishErased,
	}
}

func resolve(ctx context.Context, s *ent.Scoped, id Identifier) (*Target, error) {
	c, err := findContact(ctx, s, id)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return resolveContactless(ctx, s, id)
	}
	visitors, err := s.Visitor().Query().Where(visitorOf(c.ID)).All(ctx)
	if err != nil {
		return nil, err
	}
	t := &Target{ContactID: c.ID, IdentifierKind: id.kind(), SubjectID: strconv.FormatInt(c.ID, 10)}
	if c.SubjectID != nil && *c.SubjectID != "" {
		t.SubjectID = *c.SubjectID
	}
	for _, v := range visitors {
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
func findContact(ctx context.Context, s *ent.Scoped, id Identifier) (*ent.Contact, error) {
	var (
		c   *ent.Contact
		err error
	)
	switch {
	case id.email != "":
		c, err = s.Contact().Query().Where(contact.Email(id.email)).Only(ctx)
	case id.visitorID != "":
		var v *ent.Visitor
		v, err = s.Visitor().Query().Where(visitor.VisitorID(id.visitorID)).Only(ctx)
		if err == nil && v.ContactID != nil {
			c, err = s.Contact().Get(ctx, *v.ContactID)
		}
	default:
		c, err = s.Contact().Get(ctx, id.contactID)
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
func resolveContactless(ctx context.Context, s *ent.Scoped, id Identifier) (*Target, error) {
	var (
		t     *Target
		known bool
		err   error
	)
	switch {
	case id.email != "":
		t = &Target{Destinations: []string{id.email}}
		known, err = s.OutboundMessage().Query().Where(outboundmessage.Destination(id.email)).Exist(ctx)
		if err == nil && !known {
			known, err = s.CapturedMessage().Query().Where(capturedmessage.Recipient(id.email)).Exist(ctx)
		}
	case id.visitorID != "":
		t = &Target{VisitorIDs: []string{id.visitorID}, SubjectID: id.visitorID}
		known, err = s.Visitor().Query().Where(visitor.VisitorID(id.visitorID)).Exist(ctx)
		if err == nil && !known {
			known, err = s.Event().Query().Where(event.VisitorID(id.visitorID)).Exist(ctx)
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
