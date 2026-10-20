package erasure

import (
	"context"
	"errors"
	"net/mail"
	"strings"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
)

// ErrInvalidEmail means the address of a Privacy request is not a plain email address.
var ErrInvalidEmail = errors.New("erasure: invalid email address")

// Holdings is what a Workspace holds for one email address, as counts (a Privacy
// request lookup). Nothing held is the zero value.
type Holdings struct {
	ContactHeld     bool
	Events          int
	DeliveryRecords int
	Unsubscribes    int
	Suppressions    int
}

// canonicalEmail is the one rule for the form an address is matched in: trimmed and
// lowercased. ByEmail and NormalizeEmail both use it.
func canonicalEmail(raw string) string { return strings.ToLower(strings.TrimSpace(raw)) }

// NormalizeEmail returns the canonical form of an address and rejects anything that is
// not a single plain address (no display name, no list). Callers hand the raw input to
// Lookup and ByEmail, which normalize themselves; a handler calls this only to answer
// 400 for an invalid address before it acts.
func NormalizeEmail(raw string) (string, error) {
	email := canonicalEmail(raw)
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return "", ErrInvalidEmail
	}
	return email, nil
}

// Lookup counts what the Workspace of s holds for the address: the Contact with it,
// that Contact's Events and the Events carrying it, the delivery records to it, and
// the Unsubscribes and Suppressions on it. It reads only through s, so another
// Workspace's rows never match, and changes nothing.
func Lookup(ctx context.Context, s *ent.Scoped, email string) (Holdings, error) {
	var h Holdings
	address, err := NormalizeEmail(email)
	if err != nil {
		return h, err
	}

	c, err := s.Contact().Query().Where(contact.Email(address)).Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return h, err
	}
	h.ContactHeld = err == nil

	eventMatch := event.Email(address)
	if h.ContactHeld {
		eventMatch = event.Or(eventMatch, event.ContactID(c.ID))
	}
	h.Events, err = s.Event().Query().Where(eventMatch).Count(ctx)
	if err != nil {
		return h, err
	}
	h.DeliveryRecords, err = s.OutboundMessage().Query().Where(outboundmessage.Destination(address)).Count(ctx)
	if err != nil {
		return h, err
	}
	h.Unsubscribes, err = s.Unsubscribe().Query().Where(unsubscribe.Destination(address)).Count(ctx)
	if err != nil {
		return h, err
	}
	h.Suppressions, err = s.Suppression().Query().Where(suppression.Destination(address)).Count(ctx)
	return h, err
}
