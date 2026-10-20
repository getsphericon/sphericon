// Package erasure is the one place a Contact's personal data is erased (GDPR Art. 17,
// ADR 0021). Deleting a Contact is Erasure: there is no softer variant. Every surface
// (the /site SPA API, the external /api and so MCP) hands it a Workspace-scoped client
// and an Identifier and gets back nil or a domain error; none re-implements what is
// removed, anonymized or kept.
//
// The whole erasure is one transaction, opened through events.Bus.WithinScopedPurgeTx so
// the Contact change and the domain-event outbox commit together. Inside it the work
// is an ordered list of steps (see Module.steps), each a small function over the
// resolved Target; extending Erasure (another identifier kind, another dependent
// table, another signal published in the same transaction) means resolving the new
// identifier into a Target or adding one step, not a second code path.
package erasure

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/visitor"
	"github.com/getsphericon/sphericon/internal/events"
)

// ErrNotFound means the identifier resolves to nothing in the Workspace.
var ErrNotFound = errors.New("erasure: not found")

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
	// SubjectID is the customer's own id for the person (the Contact's subject_id, or
	// its sphericon id when it has none); only the contact.erased delivery carries it.
	SubjectID string
	// IdentifierKind is what the Identifier was.
	IdentifierKind string
	// Requester is who asked.
	Requester Requester
}

// step is one part of the erasure, run inside the transaction.
type step func(ctx context.Context, s *ent.Scoped, t *Target, pub events.PurgingPublisher) error

// Module erases data subjects.
type Module struct {
	bus *events.Bus
}

// New builds the module over the bus whose transaction every erasure runs in.
func New(bus *events.Bus) *Module {
	return &Module{bus: bus}
}

// Erase erases the identified data subject in one transaction: either all of it
// happens or none. ErrNotFound means the identifier resolves to nothing in the
// Workspace (including a row of another Workspace).
func (m *Module) Erase(ctx context.Context, s *ent.Scoped, id Identifier, by Requester) error {
	return m.bus.WithinScopedPurgeTx(ctx, s, func(ts *ent.Scoped, pub events.PurgingPublisher) error {
		target, err := resolve(ctx, ts, id)
		if err != nil {
			return err
		}
		target.Requester = by
		for _, run := range m.steps() {
			if err := run(ctx, ts, target, pub); err != nil {
				return err
			}
		}
		return nil
	})
}

// steps is the ordered erasure. The Contact row goes last: the other steps find the
// Contact's rows through ids already resolved into the Target.
func (m *Module) steps() []step {
	return []step{
		cancelInFlight,
		eraseEvents,
		eraseVisitors,
		eraseConfirmations,
		eraseRuns,
		detachOptOuts,
		anonymizeDelivery,
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
	return t, nil
}

// findContact returns the Contact the identifier names, or nil when it names none
// (an address or Visitor without a Contact). A contact id that resolves to nothing is
// ErrNotFound.
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
			return nil, ErrNotFound
		}
	}
	if ent.IsNotFound(err) {
		return nil, nil
	}
	return c, err
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
		t = &Target{Destinations: []string{id.email}, SubjectID: id.email}
		known, err = s.OutboundMessage().Query().Where(outboundmessage.Destination(id.email)).Exist(ctx)
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
		return nil, ErrNotFound
	}
	t.IdentifierKind = id.kind()
	return t, nil
}
