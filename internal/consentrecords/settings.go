package consentrecords

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/consentrecord"
	"github.com/getsphericon/sphericon/ent/signupitem"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/signupitems"
	"github.com/getsphericon/sphericon/internal/validation"
)

// The refusals of a change in account settings.
var (
	// ErrNotInEffect: the item is unknown, archived or has no published version.
	ErrNotInEffect = validation.NotFound("consentrecords: item not in effect", "errors.optional_consent_not_found")
	// ErrNotOptional: a required acceptance or a notice is not the User's to change.
	ErrNotOptional = validation.Refuse("consentrecords: not an optional consent", "errors.optional_consent_not_optional")
	// ErrStaleVersion: the page showed a version that is no longer the current one.
	ErrStaleVersion = validation.RefuseField("version", "consentrecords: stale version", "errors.optional_consent_stale")
)

// OptionalConsent is an optional consent in effect with the User's current answer to it.
type OptionalConsent struct {
	Key   string
	Title string
	// Version is the label of the current version.
	Version string
	// Decision is the User's current answer, nil when they never answered (they signed up before
	// the item existed).
	Decision *consentrecord.Decision
	// DecidedAt is when the current answer was given, nil with no answer.
	DecidedAt *time.Time
}

// ChangeOptional decides a change the User makes in account settings to the optional consent key,
// answered on version: the Entry it records, or nil when the answer already is what the change
// asks. Giving the consent records it accepted on the current version unless it is given there
// already; withdrawing records it withdrawn when it is given (at any version), and nothing
// otherwise. ErrNotInEffect for an item not in effect, ErrNotOptional for a required acceptance or
// a notice, ErrStaleVersion for a version that is not the current one.
func ChangeOptional(inEffect []signupitems.Document, current []Record, key, version string, accepted bool) (*Entry, error) {
	d, ok := documentOf(inEffect, key)
	switch {
	case !ok:
		return nil, ErrNotInEffect
	case d.Kind != signupitem.KindOptionalConsent:
		return nil, ErrNotOptional
	case d.Label != version:
		return nil, ErrStaleVersion
	}
	answer, answered := recordOf(current, key)
	given := answered && answer.Decision == consentrecord.DecisionAccepted
	givenHere := given && answer.Version == d.Label
	switch {
	case accepted && !givenHere:
		return &Entry{VersionID: d.VersionID, Decision: consentrecord.DecisionAccepted}, nil
	case !accepted && given:
		return &Entry{VersionID: d.VersionID, Decision: consentrecord.DecisionWithdrawn}, nil
	}
	return nil, nil
}

func documentOf(docs []signupitems.Document, key string) (signupitems.Document, bool) {
	for _, d := range docs {
		if d.Key == key {
			return d, true
		}
	}
	return signupitems.Document{}, false
}

func recordOf(records []Record, key string) (Record, bool) {
	for _, r := range records {
		if r.Key == key {
			return r, true
		}
	}
	return Record{}, false
}

// OptionalConsents is what account settings shows (ADR 0062): every optional consent in effect, in
// the order of the sign-up form, with the User's current answer to it. A required acceptance and a
// notice are not listed.
func (m *Module) OptionalConsents(ctx context.Context, userID int64) ([]OptionalConsent, error) {
	inEffect, current, err := m.state(ctx, userID)
	if err != nil {
		return nil, err
	}
	var consents []OptionalConsent
	for _, d := range inEffect {
		if d.Kind == signupitem.KindOptionalConsent {
			consents = append(consents, optionalConsent(d, current))
		}
	}
	return consents, nil
}

// SetOptionalConsent gives (accepted) or withdraws the User's optional consent key, answered on
// version (see ChangeOptional), recording the change with the source account settings, and is the
// consent with the User's answer after it.
func (m *Module) SetOptionalConsent(ctx context.Context, userID int64, key, version string, accepted bool) (OptionalConsent, error) {
	inEffect, current, err := m.state(ctx, userID)
	if err != nil {
		return OptionalConsent{}, err
	}
	entry, err := ChangeOptional(inEffect, current, key, version, accepted)
	if err != nil {
		return OptionalConsent{}, err
	}
	if entry != nil {
		consents := Consents{Source: consentrecord.SourceAccountSettings, Entries: []Entry{*entry}}
		err := m.bus.WithinTx(ctx, func(tx *ent.Client, _ events.Publisher) error {
			return Write(ctx, tx, userID, consents)
		})
		if err != nil {
			return OptionalConsent{}, err
		}
		if current, err = m.Current(ctx, userID); err != nil {
			return OptionalConsent{}, err
		}
	}
	d, _ := documentOf(inEffect, key)
	return optionalConsent(d, current), nil
}

// state is the items in effect and the User's current answers.
func (m *Module) state(ctx context.Context, userID int64) ([]signupitems.Document, []Record, error) {
	inEffect, err := m.items.InEffect(ctx)
	if err != nil {
		return nil, nil, err
	}
	current, err := m.Current(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	return inEffect, current, nil
}

func optionalConsent(d signupitems.Document, current []Record) OptionalConsent {
	c := OptionalConsent{Key: d.Key, Title: d.Title, Version: d.Label}
	if r, ok := recordOf(current, d.Key); ok {
		c.Decision = &r.Decision
		c.DecidedAt = &r.DecidedAt
	}
	return c
}
