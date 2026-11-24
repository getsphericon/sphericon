// Package engagementtrackingunchecked holds the Engagement tracking consent part of a Contact's
// Erasure. It is a step of a Contact's Erasure (ADR 0021), which harms a live Contact when it runs
// alone: the use case internal/usecase/contacterasure runs it in the Erasure's transaction, and
// only a use case may import this one (ADR 0110).
package engagementtrackingunchecked

import (
	"context"
	"fmt"
	"slices"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/engagementtrackingconsent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/phone"
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
// Unsubscribe is, so an address imported again is still not tracked. On sms it stays keyed by the
// number's hash (phone.Hash) instead of the number, as an SMS opt-out does: the number leaves only
// its hash.
func (m *Module) Erase(ctx context.Context, s *ent.Scoped, in EraseInput) error {
	if len(in.Destinations) == 0 {
		return nil
	}
	held := in.Destinations
	if in.Channel == channels.SMS {
		// A number's consent may stand by its hash already, kept from a Contact erased before.
		held = append(slices.Clone(held), lo.Map(held, func(n string, _ int) string { return phone.Hash(n) })...)
	}
	_, err := s.EngagementTrackingConsent().Delete().Where(
		engagementtrackingconsent.ChannelEQ(in.Channel),
		engagementtrackingconsent.DestinationIn(held...),
		engagementtrackingconsent.StateEQ(engagementtracking.Granted),
	).Exec(ctx)
	if err != nil {
		return fmt.Errorf("erase engagement tracking consent: %w", err)
	}
	if in.Channel != channels.SMS {
		return nil
	}
	return hashRefusals(ctx, s, in.Destinations)
}

// hashRefusals rekeys the `denied` of each number to the number's hash. A number's consent stands
// by the number or by its hash, never both (engagementtracking reads either and writes the one that
// stands), so the hash is free.
func hashRefusals(ctx context.Context, s *ent.Scoped, numbers []string) error {
	rows, err := s.EngagementTrackingConsent().Query().Where(
		engagementtrackingconsent.ChannelEQ(channels.SMS),
		engagementtrackingconsent.DestinationIn(numbers...),
	).All(ctx)
	if err != nil {
		return fmt.Errorf("find sms tracking refusals: %w", err)
	}
	for _, row := range rows {
		if err := s.EngagementTrackingConsent().UpdateOneID(row.ID).SetDestination(phone.Hash(row.Destination)).Exec(ctx); err != nil {
			return fmt.Errorf("hash sms tracking refusal: %w", err)
		}
	}
	return nil
}
