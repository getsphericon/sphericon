// Package identify is the use case of Identify (GLOSSARY, ADR 0072, ADR 0102): the /collect call
// that binds a Visitor to a Contact. It spans the Contact (contacts), the Visitor (visitors), the
// chats the Visitor started a bot with (bindings) and its anonymous Events (the bus), so it
// composes their functions in one transaction.
package identify

import (
	"context"
	"errors"
	"strings"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/bindings"
	"github.com/getsphericon/sphericon/internal/contacts"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/visitors"
)

// Input is an Identify call: the device and what it asserts about its Contact.
type Input struct {
	VisitorID string
	Email     *string
	Phone     *string
	SubjectID *string
	Traits    map[string]any
	// The browser's time zone and language, guesses that only fill the Contact's (ADR 0072).
	TimeZone, Locale *string
}

// UseCase identifies Visitors. Every call receives the Workspace-scoped client (ADR 0017) built by
// the entry point; the use case opens the transaction the modules it calls join.
type UseCase struct {
	bus *events.Bus
}

// New builds the Identify use case.
func New(bus *events.Bus) *UseCase { return &UseCase{bus: bus} }

// Identify binds a Visitor to a Contact and asserts that Contact's alias keys
// (subject_id / email / phone) and the core attributes its traits name (ADR 0072). It
// upserts the Contact by any present alias key, auto-creates typed Custom fields from the
// other traits, binds the device, and stitches the device's earlier anonymous Events onto
// the Contact so pre-identify behavior becomes visible to segmentation. A newly created
// Contact emits contact.created.
func (u *UseCase) Identify(ctx context.Context, s *ent.Scoped, input Input) error {
	visitorID := strings.TrimSpace(input.VisitorID)
	if visitorID == "" {
		return errors.New("visitorId is required")
	}

	return u.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		core, fields := contacts.FromTraits(input.Traits)
		res, err := contacts.UpsertIn(ctx, ts, pub, contact.SourceTracker, contacts.UpsertInput{
			SubjectID:       input.SubjectID,
			Email:           input.Email,
			Phone:           input.Phone,
			Core:            core,
			GuessedTimeZone: input.TimeZone,
			GuessedLocale:   input.Locale,
			CustomFields:    fields,
		})
		if errors.Is(err, contacts.ErrIdentityRequired) {
			return errors.New("identify requires subjectId, email, or phone")
		}
		if err != nil {
			return err
		}
		c := res.Contact

		vis, err := visitors.SeeIn(ctx, ts, visitorID)
		if err != nil {
			return err
		}
		if err := visitors.BindIn(ctx, ts, vis.ID, c.ID); err != nil {
			return err
		}
		// The chats the Visitor started a bot with now reach the Contact: announce them.
		if err := bindings.AnnounceVisitor(ctx, ts, pub, vis, c); err != nil {
			return err
		}

		// Stitch: attach the device's earlier anonymous events onto the Contact.
		return events.StitchVisitor(ctx, ts, visitorID, c.ID)
	})
}
