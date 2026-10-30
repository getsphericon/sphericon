// Package consentrecords is the Consent records of the Users (ADR 0062): every decision a User made
// on a version of a Sign-up item's Legal document, accepted, declined or withdrawn, or shown for a
// notice, with where it was made and the IP address and user agent it came from. Records are only
// ever added; the latest record of an item is the User's current answer.
//
// It answers a form's answers against the Sign-up items in effect (Decide, and Module.Check over
// internal/signupitems), writes the records in the transaction that makes the User or changes the
// answer (Write), reads a User's current answers (Module.Current) and many Users' current consent
// to the operator's mailings (Mailings, for the service feed of Users). The records belong to the
// User, not to a Workspace, so the package works on the raw *ent.Client (it is in the closed list
// of docs/architecture.md): the Consent record table and nothing else.
package consentrecords

import (
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/consentrecord"
	"github.com/getsphericon/sphericon/ent/legaldocumentversion"
	"github.com/getsphericon/sphericon/ent/signupitem"
	"github.com/getsphericon/sphericon/internal/clientip"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/signupitems"
	"github.com/getsphericon/sphericon/internal/validation"
	"github.com/samber/lo"
)

// Field is the json name of the form field that carries the answers: a refusal of the answers as a
// whole is reported against it, and a required acceptance left unticked against ItemField(key).
const Field = validation.FieldSignupItems

// ItemField is the field a refusal of the item with key is reported under.
func ItemField(key string) string { return validation.Nested(Field, key) }

// latestFirst orders Consent records the latest first: records are only added, so the first of an
// item (or of a User) in that order is the current answer, which lo.UniqBy keeps.
var latestFirst = ent.Desc(consentrecord.FieldID)

// Answer is a person's answer to one Sign-up item the form showed: the item's key, the label of
// the version shown and whether its box was ticked (a notice has none).
type Answer struct {
	Key      string
	Version  string
	Accepted bool
}

// Entry is one record a form's answers make: the decision on a version.
type Entry struct {
	VersionID int64
	Decision  consentrecord.Decision
}

// Consents is what a form's answers record, and where the form was answered, until the User they
// belong to is written.
type Consents struct {
	Source  consentrecord.Source
	Entries []Entry
}

// Decide answers a form against the Sign-up items in effect, in their order. Each answered item
// is recorded accepted or declined, and each notice in effect shown at its current version; an
// optional consent left out is not recorded. The form is refused, as a *validation.FieldsError,
// when an answer names an item not in effect, a version not its current one or an item twice
// (Field), or when a required acceptance is not answered ticked (Field.<key> for each). A
// refused form records nothing.
func Decide(inEffect []signupitems.Document, answers []Answer) ([]Entry, error) {
	byKey := make(map[string]signupitems.Document, len(inEffect))
	for _, d := range inEffect {
		byKey[d.Key] = d
	}
	given := make(map[string]Answer, len(answers))
	for _, a := range answers {
		d, known := byKey[a.Key]
		_, twice := given[a.Key]
		if !known || twice || d.Label != a.Version {
			return nil, validation.Refused(Field, i18n.T("errors.signup_items_stale", nil))
		}
		given[a.Key] = a
	}

	fields := map[string][]string{}
	var entries []Entry
	for _, d := range inEffect {
		a, answered := given[d.Key]
		switch {
		case d.Kind == signupitem.KindNotice:
			entries = append(entries, Entry{VersionID: d.VersionID, Decision: consentrecord.DecisionShown})
		case d.Kind == signupitem.KindRequiredAcceptance && !a.Accepted:
			fields[ItemField(d.Key)] = []string{i18n.T("errors.signup_item_required", map[string]any{"Title": d.Title})}
		case answered:
			entries = append(entries, Entry{VersionID: d.VersionID, Decision: decision(a.Accepted)})
		}
	}
	if len(fields) > 0 {
		return nil, &validation.FieldsError{Fields: fields}
	}
	return entries, nil
}

func decision(accepted bool) consentrecord.Decision {
	if accepted {
		return consentrecord.DecisionAccepted
	}
	return consentrecord.DecisionDeclined
}

// Write adds the records of consents for the User in tx, the transaction that writes the User or
// its answer, with the client's address and user agent of the request in ctx (resolved as the
// rate limiter resolves them, internal/clientip).
func Write(ctx context.Context, tx *ent.Client, userID int64, consents Consents) error {
	if len(consents.Entries) == 0 {
		return nil
	}
	ip := optional(clientip.FromContext(ctx))
	userAgent := optional(clientip.UserAgentFromContext(ctx))
	builders := make([]*ent.ConsentRecordCreate, len(consents.Entries))
	for i, e := range consents.Entries {
		builders[i] = tx.ConsentRecord.Create().
			SetUserID(userID).
			SetVersionID(e.VersionID).
			SetDecision(e.Decision).
			SetSource(consents.Source).
			SetNillableIP(ip).
			SetNillableUserAgent(userAgent)
	}
	return tx.ConsentRecord.CreateBulk(builders...).Exec(ctx)
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Record is a User's current answer to one Sign-up item: its latest Consent record.
type Record struct {
	// Key is the item's key.
	Key string
	// Version is the label of the version decided on.
	Version   string
	Decision  consentrecord.Decision
	Source    consentrecord.Source
	IP        string
	UserAgent string
	// DecidedAt is when the decision was made.
	DecidedAt time.Time
}

// Module is the Consent records over the Instance's Sign-up items.
type Module struct {
	ent   *ent.Client
	bus   *events.Bus
	items *signupitems.Module
}

// New builds the module over the raw client, the bus, in whose transaction a change in account
// settings writes its record, and the Sign-up items.
func New(client *ent.Client, bus *events.Bus, items *signupitems.Module) *Module {
	return &Module{ent: client, bus: bus, items: items}
}

// Check answers a form against the Sign-up items in effect now (see Decide): the Consents it
// records, made at source, or the *validation.FieldsError that refuses it.
func (m *Module) Check(ctx context.Context, answers []Answer, source consentrecord.Source) (Consents, error) {
	inEffect, err := m.items.InEffect(ctx)
	if err != nil {
		return Consents{}, err
	}
	entries, err := Decide(inEffect, answers)
	if err != nil {
		return Consents{}, err
	}
	return Consents{Source: source, Entries: entries}, nil
}

// Current is the User's current answer to each Sign-up item they decided on, in the order of the
// sign-up form: the latest record of each item.
func (m *Module) Current(ctx context.Context, userID int64) ([]Record, error) {
	rows, err := m.ent.ConsentRecord.Query().
		Where(consentrecord.UserID(userID)).
		WithVersion(func(q *ent.LegalDocumentVersionQuery) { q.WithItem() }).
		Order(latestFirst).
		All(ctx)
	if err != nil {
		return nil, err
	}
	current := lo.UniqBy(rows, func(r *ent.ConsentRecord) int64 { return r.Edges.Version.ItemID })
	records := make([]Record, len(current))
	for i, r := range sortedByForm(current) {
		records[i] = Record{
			Key: r.Edges.Version.Edges.Item.Key, Version: r.Edges.Version.Label,
			Decision: r.Decision, Source: r.Source, DecidedAt: r.CreatedAt,
			IP: deref(r.IP), UserAgent: deref(r.UserAgent),
		}
	}
	return records, nil
}

// Mailing is a User's current consent to the operator's mailings: their latest record on the
// Sign-up item that stands for it.
type Mailing struct {
	// Accepted reports whether that record is an acceptance: false when it is a decline or a
	// withdrawal.
	Accepted bool
	// DecidedAt is when the decision was made.
	DecidedAt time.Time
}

// Mailings is the current consent to the operator's mailings of each of the Users that has
// answered the item standing for it, by User id. A User who never answered it, and every User
// when no item stands for it, is absent: they have not consented. It reads the records through
// client, for the service feed of Users (ee/staff), which pages over many Users at once.
func Mailings(ctx context.Context, client *ent.Client, userIDs ...int64) (map[int64]Mailing, error) {
	rows, err := client.ConsentRecord.Query().
		Where(
			consentrecord.UserIDIn(userIDs...),
			consentrecord.HasVersionWith(legaldocumentversion.HasItemWith(signupitem.MailingsConsent(true))),
		).
		Order(latestFirst).
		All(ctx)
	if err != nil {
		return nil, err
	}
	latest := lo.UniqBy(rows, func(r *ent.ConsentRecord) int64 { return r.UserID })
	mailings := make(map[int64]Mailing, len(latest))
	for _, r := range latest {
		mailings[r.UserID] = Mailing{Accepted: r.Decision == consentrecord.DecisionAccepted, DecidedAt: r.CreatedAt}
	}
	return mailings, nil
}

// sortedByForm orders records by their item's place in the sign-up form.
func sortedByForm(rows []*ent.ConsentRecord) []*ent.ConsentRecord {
	slices.SortFunc(rows, func(a, b *ent.ConsentRecord) int {
		ia, ib := a.Edges.Version.Edges.Item, b.Edges.Version.Edges.Item
		return cmp.Or(cmp.Compare(ia.Position, ib.Position), cmp.Compare(ia.ID, ib.ID))
	})
	return rows
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
