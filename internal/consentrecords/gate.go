package consentrecords

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/consentrecord"
	"github.com/getsphericon/sphericon/ent/signupitem"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/signupitems"
	"github.com/getsphericon/sphericon/internal/validation"
)

// The acceptance gate (ADR 0062): a User is gated while a required acceptance in effect has a
// current version they have no accepted record of. A new version of a required item, an item
// added to an Instance that already has Users, and an Owner made by the operator all gate alike;
// an optional consent or a notice gates no one. Every surface that serves a User's own credential
// asks Gated on each request and refuses a gated User with its 403 acceptance_pending.

// ErrAcceptanceStale refuses an acceptance that does not name every pending acceptance at its
// current version, under the field `items` that names them: a version was published after the
// screen showed the texts.
var ErrAcceptanceStale = validation.RefuseField("items", "consentrecords: stale acceptance", "errors.acceptance_stale")

// ReAcceptance is a User's acceptance of one pending required acceptance at the version the
// acceptance screen showed: the item's key and the label of that version.
type ReAcceptance struct {
	Key     string
	Version string
}

// Pending is the required acceptances in effect the User has not accepted at their current
// version, in the order of the sign-up form. With no required acceptance in effect it is one
// query.
func (m *Module) Pending(ctx context.Context, userID int64) ([]signupitems.Document, error) {
	required, err := m.items.InEffect(ctx, signupitem.KindRequiredAcceptance)
	if err != nil || len(required) == 0 {
		return nil, err
	}
	versionIDs := make([]int64, len(required))
	for i, d := range required {
		versionIDs[i] = d.VersionID
	}
	accepted, err := m.ent.ConsentRecord.Query().
		Where(
			consentrecord.UserID(userID),
			consentrecord.VersionIDIn(versionIDs...),
			consentrecord.DecisionEQ(consentrecord.DecisionAccepted),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return notAccepted(required, accepted), nil
}

// notAccepted is the documents whose version no accepted record names.
func notAccepted(required []signupitems.Document, accepted []*ent.ConsentRecord) []signupitems.Document {
	done := make(map[int64]bool, len(accepted))
	for _, r := range accepted {
		done[r.VersionID] = true
	}
	var pending []signupitems.Document
	for _, d := range required {
		if !done[d.VersionID] {
			pending = append(pending, d)
		}
	}
	return pending
}

// Gated reports whether the User has a required acceptance pending (see Pending): the one check
// every surface that serves the User's own credential makes before it serves them.
func (m *Module) Gated(ctx context.Context, userID int64) (bool, error) {
	pending, err := m.Pending(ctx, userID)
	return len(pending) > 0, err
}

// AcceptPending records every acceptance pending for the User accepted, with the source
// re-acceptance, in one transaction. accepted is what the screen showed; ErrAcceptanceStale, and
// nothing recorded, unless it names each pending acceptance at its current version. With nothing
// pending it records nothing.
func (m *Module) AcceptPending(ctx context.Context, userID int64, accepted []ReAcceptance) error {
	pending, err := m.Pending(ctx, userID)
	if err != nil {
		return err
	}
	acceptedAt := make(map[string]string, len(accepted))
	for _, a := range accepted {
		acceptedAt[a.Key] = a.Version
	}
	entries := make([]Entry, len(pending))
	for i, d := range pending {
		if v, ok := acceptedAt[d.Key]; !ok || v != d.Label {
			return ErrAcceptanceStale
		}
		entries[i] = Entry{VersionID: d.VersionID, Decision: consentrecord.DecisionAccepted}
	}
	if len(entries) == 0 {
		return nil
	}
	consents := Consents{Source: consentrecord.SourceReAcceptance, Entries: entries}
	return m.bus.WithinTx(ctx, func(tx *ent.Client, _ events.Publisher) error {
		return Write(ctx, tx, userID, consents)
	})
}
