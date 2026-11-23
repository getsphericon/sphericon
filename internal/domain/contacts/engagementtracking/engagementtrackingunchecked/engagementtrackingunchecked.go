// Package engagementtrackingunchecked holds the Engagement tracking consent part of a Contact's
// Erasure. It is a step of a Contact's Erasure (ADR 0021), which harms a live Contact when it runs
// alone: the use case internal/usecase/contacterasure runs it in the Erasure's transaction, and
// only a use case may import this one (ADR 0110).
package engagementtrackingunchecked

import (
	"context"
	"fmt"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/engagementtrackingconsent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
)

// Module is the Engagement tracking consent step of a Contact's Erasure. It holds nothing: every
// call takes the scope of the Erasure's transaction.
type Module struct{}

// New builds the step.
func New() *Module { return &Module{} }

// EraseInput is what Erase erases: the consent of Destinations on Channel.
type EraseInput struct {
	Channel      channels.Channel
	Destinations []string
}

// Erase is the Engagement tracking consent part of the Erasure of a Contact that held
// in.Destinations on in.Channel (ADR 0120): a `granted` goes, as a Confirmation does, so the
// Destination falls back to the default; a `denied` stays, keyed by the Destination as an
// Unsubscribe is, so an address imported again is still not tracked.
func (m *Module) Erase(ctx context.Context, s *ent.Scoped, in EraseInput) error {
	if len(in.Destinations) == 0 {
		return nil
	}
	_, err := s.EngagementTrackingConsent().Delete().Where(
		engagementtrackingconsent.ChannelEQ(in.Channel),
		engagementtrackingconsent.DestinationIn(in.Destinations...),
		engagementtrackingconsent.StateEQ(engagementtracking.Granted),
	).Exec(ctx)
	if err != nil {
		return fmt.Errorf("erase engagement tracking consent: %w", err)
	}
	return nil
}
