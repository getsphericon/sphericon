// Package frequencycapunchecked holds the Frequency cap's part of a Contact's Erasure (ADR 0021,
// ADR 0178): the deletion of the count rows of the Contact's messages, so the count of its
// Destinations restarts. It is a step of the Erasure, which harms a live Contact when it runs alone:
// the use case internal/usecase/contacterasure runs it in the one transaction of the Erasure. Only a
// use case may import this one (ADR 0110). The frequencycap module does not import it.
package frequencycapunchecked

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/frequencycount"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/predicate"
)

// Module holds the Frequency cap module's unchecked operations. Each use case that runs them builds
// its own in its constructor (docs/architecture/modules.md, the unchecked operations).
type Module struct{}

// New builds the unchecked operations; they need nothing but the scope each call is given.
func New() *Module { return &Module{} }

// EraseContact deletes the count rows of the Contact's Outbound messages and of every message to its
// destinations (its addresses and number, which the Erasure anonymizes too), in the Erasure's
// transaction (s comes from it). The rows name the Destination in the clear, and the Erasure
// restarts its count (ADR 0137). The messages are found by their Contact, which the anonymizing
// clears, so this runs before it. contactID is 0 for a subject without a Contact.
func (m *Module) EraseContact(ctx context.Context, s *ent.Scoped, contactID int64, destinations []string) error {
	var of []predicate.FrequencyCount
	if contactID != 0 {
		of = append(of, frequencycount.HasOutboundMessageWith(outboundmessage.ContactID(contactID)))
	}
	if len(destinations) > 0 {
		of = append(of, frequencycount.DestinationIn(destinations...))
	}
	if len(of) == 0 {
		return nil
	}
	_, err := s.FrequencyCount().Delete().Where(frequencycount.Or(of...)).Exec(ctx)
	return err
}
