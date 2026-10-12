package consent

import (
	"context"
	"errors"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/tracking"
)

// Intake: consent changes made by an authenticated caller (the SPA, an API token,
// an agent over MCP) rather than by a signed link. Both only narrow reach, so the
// agent may use them (ADR 0016); widening is a human's action elsewhere.

var (
	// ErrDestinationEmpty: the destination is blank after normalization.
	ErrDestinationEmpty = errors.New("consent: destination must not be empty")
	// ErrInvalidSource: the sending source is not broadcasts, everything or automation:<id>.
	ErrInvalidSource = errors.New("consent: sendingSource must be broadcasts, everything or automation:<id>")
	// ErrUnknownAutomation: the automation:<id> source names no Automation of this Workspace.
	ErrUnknownAutomation = errors.New("consent: unknown automation in sendingSource")
)

// Suppress adds a manual Suppression for an email destination. It is idempotent per
// (channel, destination): an existing entry, with its reason, is returned unchanged.
func Suppress(ctx context.Context, s *ent.Scoped, destination string) (*ent.Suppression, error) {
	dest := eligibility.NormalizeDestination(destination)
	if dest == "" {
		return nil, ErrDestinationEmpty
	}
	if err := s.Suppression().Create().
		SetChannel(suppression.ChannelEmail).
		SetDestination(dest).
		SetReason(suppression.ReasonManual).
		OnConflictColumns(suppression.FieldWorkspaceID, suppression.FieldChannel, suppression.FieldDestination).
		Ignore().
		Exec(ctx); err != nil {
		return nil, err
	}
	return s.Suppression().Query().
		Where(suppression.ChannelEQ(suppression.ChannelEmail), suppression.DestinationEQ(dest)).
		Only(ctx)
}

// Unsubscribe records an opt-out of destination from a sending source, with the
// same effects as the unsubscribe link (RecordUnsubscribe): the row, the
// confirmation reset on "everything", the automation exit and the engagement event,
// atomically and idempotently. When the destination is a Contact of the Workspace
// the event is attributed to it. The source is validated first.
func Unsubscribe(ctx context.Context, bus *events.Bus, s *ent.Scoped, destination, source string) (*ent.Unsubscribe, error) {
	dest := eligibility.NormalizeDestination(destination)
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
		exists, err := s.Automation().Query().Where(automation.ID(automationID)).Exist(ctx)
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
