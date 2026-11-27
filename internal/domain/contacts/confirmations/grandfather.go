package confirmations

import (
	"context"
	"errors"
	"fmt"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/domain/contacts/consentstamps"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
)

// GrandfatherPage is how many addresses one call of Grandfather records at most: the size of the
// transactions turning confirmed opt-in on takes the list in.
const GrandfatherPage = 500

// Grandfathered is what one call of Grandfather did: how many Confirmations it recorded, the last
// Contact it read, which the next call starts after, and whether it read a whole page, so more
// may remain.
type Grandfathered struct {
	Recorded int
	Last     int64
	More     bool
}

// Grandfather records a Confirmation with provenance grandfathered for the email addresses of the
// scoped Workspace's Contacts that are mailable and unconfirmed (eligibility.UnconfirmedMailable),
// at most limit of them, in the order of the Contacts after the one after names (0 starts at the
// first), in the transaction s is bound to (ADR 0013). Each has its marketing.confirmed Event,
// recorded as history that starts no Automation and reaches no webhook. It is what turning
// confirmed opt-in on asserts for the list mailable then, through the one writer (recordIn): an
// address a stronger Confirmation or a later opt-out already covers records nothing. A Contact
// created meanwhile comes after every one read, so the pages that follow reach it.
func (m *Module) Grandfather(ctx context.Context, s *ent.Scoped, after int64, limit int) (Grandfathered, error) {
	found, err := s.Contact().Query().
		Where(contact.IDGT(after), eligibility.UnconfirmedMailable()).
		Order(ent.Asc(contact.FieldID)).
		Limit(limit).
		Select(contact.FieldID, contact.FieldEmail).
		All(ctx)
	if err != nil {
		return Grandfathered{}, fmt.Errorf("find the addresses to grandfather: %w", err)
	}
	done := Grandfathered{Last: after, More: len(found) == limit}
	at := consentstamps.At(m.now())
	// A grandfathered Confirmation is asserted, not given by the person now: its proof is history
	// that triggers nothing (events.Backfill).
	history := events.WithCause(ctx, events.Backfill())
	for _, c := range found {
		done.Last = c.ID
		_, wrote, err := m.recordIn(history, s, grant{
			channel: channels.Email, destination: *c.Email, provenance: confirmation.ProvenanceGrandfathered,
			contactID: c.ID, at: at,
		})
		switch {
		case errors.Is(err, ErrSuperseded):
			// The address's consent state was set after now by its own clock: nothing to assert.
		case err != nil:
			return Grandfathered{}, err
		case wrote:
			done.Recorded++
		}
	}
	return done, nil
}
