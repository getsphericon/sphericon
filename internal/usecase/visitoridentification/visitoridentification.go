// Package visitoridentification is the use case of a Visitor identification, the Identify of the
// glossary (ADR 0072, ADR 0102): the /collect call that binds a Visitor to a Contact. It spans the
// Contact (contacts), the Visitor (visitors) and its anonymous Events (the bus), so it composes their
// functions in one transaction.
package visitoridentification

import (
	"context"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/contacts/visitors"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/fieldname"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
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
	now func() time.Time
}

// New builds the Identify use case; now is the Instance clock a Visitor is seen by.
func New(bus *events.Bus, now func() time.Time) *UseCase { return &UseCase{bus: bus, now: now} }

// ErrVisitorIDRequired refuses an identify that names no visitor: there is no device to bind. An
// identify that names no alias key of the Contact is contacts.ErrIdentityRequired.
var ErrVisitorIDRequired = validation.RefuseField(fieldname.VisitorID, "identify: visitorId is required", i18n.ErrorsVisitorIdRequired)

// Identify binds a Visitor to a Contact and claims that Contact's alias keys (subject_id / email /
// phone), the core attributes its traits name (ADR 0072) and the defined Custom fields the other
// traits name. It comes through the public collect key, so it proves nothing (ADR 0171): it finds or
// creates the Contact by any present alias key, gives a Contact it creates everything and one that
// exists only the attributes it has no value of, and writes no undefined Custom field. It binds the
// Visitor and stitches the device's earlier anonymous Events onto the Contact only while the Visitor
// is anonymous or already that Contact's: one that belongs to another Contact stays where it is. A
// newly created Contact emits contact.created.
func (u *UseCase) Identify(ctx context.Context, s *ent.Scoped, input Input) error {
	visitorID := strings.TrimSpace(input.VisitorID)
	if visitorID == "" {
		return ErrVisitorIDRequired
	}

	return u.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		core, traits := contacts.FromTraits(input.Traits)
		fields, err := contacts.KnownCustomFields(ctx, ts, traits)
		if err != nil {
			return err
		}
		res, err := contacts.UpsertIn(ctx, ts, pub, contact.SourceTracker, contacts.UpsertInput{
			SubjectID:       input.SubjectID,
			Email:           input.Email,
			Phone:           input.Phone,
			Core:            core,
			GuessedTimeZone: input.TimeZone,
			GuessedLocale:   input.Locale,
			CustomFields:    fields,
			Claimed:         true,
		})
		if err != nil {
			return err
		}
		c := res.Contact

		vis, err := visitors.SeeIn(ctx, ts, visitorID, u.now())
		if err != nil {
			return err
		}
		if vis.ContactID != nil && *vis.ContactID != c.ID {
			return nil
		}
		if err := visitors.BindIn(ctx, ts, vis.ID, c.ID, u.now()); err != nil {
			return err
		}

		// Stitch: attach the device's earlier anonymous events onto the Contact.
		return events.StitchVisitor(ctx, ts, visitorID, c.ID)
	})
}
