// Package visitoridentification is the use case of a Visitor identification, the Identify of the
// glossary (ADR 0072, ADR 0102): the /collect call that binds a Visitor to a Contact, and the
// tracker's Telegram start link, which an Identity token gives to the Contact it proves (ADR 0171).
// It spans the proof (identitytokens), the Contact (contacts), the Visitor (visitors), its chats
// (bindings) and its anonymous Events (the bus), so it composes their functions in one transaction.
package visitoridentification

import (
	"context"
	"strings"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/contacts/identitytokens"
	"github.com/getsphericon/sphericon/internal/domain/contacts/visitors"
	"github.com/getsphericon/sphericon/internal/domain/sending/bindings"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/bounds"
	"github.com/getsphericon/sphericon/internal/kernel/emailcase"
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
	// IdentityToken proves the keys (ADR 0171): with it the keys are the token's, and a key sent
	// beside it must be the same. Empty for a claim.
	IdentityToken string
	Traits        map[string]any
	// The browser's time zone and language, guesses that only fill the Contact's (ADR 0072).
	TimeZone, Locale *string
}

// StartLinkInput is the tracker's request of a Telegram start link for its Visitor.
type StartLinkInput struct {
	VisitorID string
	// IdentityToken, when given, makes the link the Contact's that the token proves.
	IdentityToken string
}

// UseCase identifies Visitors. Every call receives the Workspace-scoped client (ADR 0017) built by
// the entry point; the use case opens the transaction the modules it calls join.
type UseCase struct {
	bus      *events.Bus
	contacts *contacts.Module
	visitors *visitors.Module
	tokens   *identitytokens.Module
	bindings *bindings.Module
	now      func() time.Time
}

// New builds the Identify use case; people resolves and upserts the Contact, visits sees and binds
// the Visitor, tokens checks an Identity token, bindings issues start links and hands a Visitor's
// over, and now is the Instance clock a hand-over is dated by.
func New(bus *events.Bus, people *contacts.Module, visits *visitors.Module, tokens *identitytokens.Module, b *bindings.Module, now func() time.Time) *UseCase {
	return &UseCase{bus: bus, contacts: people, visitors: visits, tokens: tokens, bindings: b, now: now}
}

var (
	// ErrVisitorIDRequired refuses an identify that names no visitor: there is no device to bind.
	// An identify that names no alias key of the Contact is contacts.ErrIdentityRequired.
	ErrVisitorIDRequired = validation.RefuseField(fieldname.VisitorID, "identify: visitorId is required", i18n.ErrorsVisitorIdRequired)
	// ErrKeyMismatch refuses a proven identify that sends a key other than the one its token proves.
	ErrKeyMismatch = validation.Refuse("identify: a key sent with the identity token differs from the token's", i18n.ErrorsIdentityTokenKeyMismatch)
)

// Identify binds a Visitor to a Contact and asserts that Contact's alias keys (subject_id / email /
// phone), the core attributes its traits name (ADR 0072) and the defined Custom fields the other
// traits name; the collect key defines none (ADR 0171).
//
// Without an Identity token it is a claim: it finds or creates the Contact by any present alias key,
// gives a Contact it creates everything and one that exists only the attributes it has no value of,
// and binds the Visitor and stitches its earlier anonymous Events onto the Contact only while the
// Visitor is anonymous or already that Contact's.
//
// With one it is proven: the keys are the token's, it fills the Contact's alias keys and writes its
// attributes, moves the Visitor to it, stitches its Events and hands the Contact the Visitor's chats.
// A token that does not verify refuses the call. A newly created Contact emits contact.created.
func (u *UseCase) Identify(ctx context.Context, s *ent.Scoped, input Input) error {
	visitorID := strings.TrimSpace(input.VisitorID)
	if visitorID == "" {
		return ErrVisitorIDRequired
	}
	proven := input.IdentityToken != ""
	if proven {
		p, err := u.tokens.Verify(ctx, s, input.IdentityToken)
		if err != nil {
			return err
		}
		if !sameKey(input.SubjectID, p.SubjectID, strings.TrimSpace) || !sameKey(input.Email, p.Email, normalEmail) ||
			!sameKey(input.Phone, p.Phone, strings.TrimSpace) {
			return ErrKeyMismatch
		}
		input.SubjectID, input.Email, input.Phone = p.SubjectID, p.Email, p.Phone
	}
	// The keys are checked as they are written: a proven Identify's are the token's.
	if err := checkBounds(input); err != nil {
		return err
	}

	return u.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		core, traits := contacts.FromTraits(input.Traits)
		fields, err := u.contacts.KnownCustomFields(ctx, ts, traits)
		if err != nil {
			return err
		}
		res, err := u.contacts.UpsertIn(ctx, ts, contact.SourceTracker, contacts.UpsertInput{
			SubjectID:       input.SubjectID,
			Email:           input.Email,
			Phone:           input.Phone,
			Core:            core,
			GuessedTimeZone: input.TimeZone,
			GuessedLocale:   input.Locale,
			CustomFields:    fields,
			Claimed:         !proven,
		})
		if err != nil {
			return err
		}
		c := res.Contact

		vis, err := u.visitors.SeeIn(ctx, ts, visitorID)
		if err != nil {
			return err
		}
		if !proven && vis.ContactID != nil && *vis.ContactID != c.ID {
			return nil
		}
		if err := u.visitors.BindIn(ctx, ts, vis.ID, c.ID); err != nil {
			return err
		}
		if proven {
			if err := bindings.HandOver(ctx, ts, pub, vis, c, u.now()); err != nil {
				return err
			}
		}
		// Stitch: attach the device's earlier anonymous events onto the Contact.
		return events.StitchVisitor(ctx, ts, visitorID, c.ID)
	})
}

// checkBounds refuses the keys and the browser's guesses of an Identify that are longer than the
// contract allows (bounds.IdentifyPhone, IdentifySubjectID, IdentifyTimeZone, IdentifyLocale).
func checkBounds(input Input) error {
	return validation.Fields(
		validation.MaxLength(fieldname.Phone, lo.FromPtr(input.Phone), bounds.IdentifyPhoneMaxLength),
		validation.MaxLength(fieldname.SubjectID, lo.FromPtr(input.SubjectID), bounds.IdentifySubjectIDMaxLength),
		validation.MaxLength(fieldname.TimeZone, lo.FromPtr(input.TimeZone), bounds.IdentifyTimeZoneMaxLength),
		validation.MaxLength(fieldname.Locale, lo.FromPtr(input.Locale), bounds.IdentifyLocaleMaxLength),
	)
}

// StartLink issues the tracker's Telegram start link on the default bot. With an Identity token that
// verifies and a Contact its keys find, the link is that Contact's; otherwise it is the Visitor's,
// whose chat reaches no Contact until a proven Identify hands it over (ADR 0171). A token that does
// not verify refuses the call.
func (u *UseCase) StartLink(ctx context.Context, s *ent.Scoped, input StartLinkInput) (bindings.StartLink, error) {
	if input.IdentityToken != "" {
		p, err := u.tokens.Verify(ctx, s, input.IdentityToken)
		if err != nil {
			return bindings.StartLink{}, err
		}
		c, err := u.contacts.Resolve(ctx, s, contacts.AliasKeys{SubjectID: p.SubjectID, Email: p.Email, Phone: p.Phone})
		if err != nil {
			return bindings.StartLink{}, err
		}
		if c != nil {
			return u.bindings.IssueStartLink(ctx, s, c.ID, nil)
		}
	}
	return u.bindings.IssueVisitorStartLink(ctx, s, input.VisitorID)
}

// sameKey reports whether a key sent beside a token is the token's, both read through norm: one
// not sent is the token's.
func sameKey(sent, proven *string, norm func(string) string) bool {
	if sent == nil || norm(*sent) == "" {
		return true
	}
	return proven != nil && norm(*sent) == norm(*proven)
}

func normalEmail(v string) string { return emailcase.Of(strings.TrimSpace(v)) }
