package consent

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/suppression"
	"github.com/getsphericon/sphericon/internal/tracking"
	"github.com/getsphericon/sphericon/internal/validation"
)

// Intake: consent changes made by an authenticated caller (the SPA, an API token,
// an agent over MCP) rather than by a signed link. Both only narrow reach, so the
// agent may use them (ADR 0016); widening is a human's action elsewhere.

var (
	// ErrDestinationEmpty: the destination is blank after normalization.
	ErrDestinationEmpty = validation.RefuseField("destination", "consent: destination must not be empty", "errors.destination_empty")
	// ErrInvalidSource: the sending source is not broadcasts, everything or automation:<id>.
	ErrInvalidSource = validation.RefuseField("sendingSource", "consent: sendingSource must be broadcasts, everything or automation:<id>", "errors.sending_source_invalid")
	// ErrUnknownAutomation: the automation:<id> source names no Automation of this Workspace.
	ErrUnknownAutomation = validation.RefuseField("sendingSource", "consent: unknown automation in sendingSource", "errors.sending_source_automation_unknown")
)

// Unsubscribe records an opt-out of destination from a sending source, with the
// same effects as the unsubscribe link (RecordUnsubscribe): the row, the
// confirmation reset on "everything", the automation exit and the engagement event,
// atomically and idempotently. When the destination is a Contact of the Workspace
// the event is attributed to it. The source is validated first.
func Unsubscribe(ctx context.Context, bus *events.Bus, s *ent.Scoped, destination, source string) (*ent.Unsubscribe, error) {
	dest := suppression.NormalizeDestination(destination)
	if dest == "" {
		return nil, ErrDestinationEmpty
	}
	if err := validateSource(ctx, s, source); err != nil {
		return nil, err
	}

	target := tracking.UnsubTarget{Source: source, Destination: dest, WorkspaceID: s.WorkspaceID()}
	if c, err := s.Contact().Query().Where(contact.EmailEqualFold(dest)).First(ctx); err == nil {
		target.ContactID = c.ID
	} else if !ent.IsNotFound(err) {
		return nil, err
	}
	if err := RecordUnsubscribe(ctx, bus, target); err != nil {
		return nil, err
	}
	return s.Unsubscribe().Query().Where(
		unsubscribe.ChannelEQ(unsubscribe.ChannelEmail),
		unsubscribe.DestinationEQ(dest),
		unsubscribe.SendingSourceEQ(source),
	).Only(ctx)
}

func validateSource(ctx context.Context, s *ent.Scoped, source string) error {
	automationID, isAutomation := eligibility.ParseAutomationSource(source)
	switch {
	case source == eligibility.SourceBroadcasts || source == eligibility.SourceEverything:
		return nil
	case isAutomation:
		exists, err := s.Automation().QueryWithDiscarded().Where(automation.ID(automationID)).Exist(ctx)
		if err != nil {
			return err
		}
		if !exists {
			return ErrUnknownAutomation
		}
		return nil
	default:
		return ErrInvalidSource
	}
}
