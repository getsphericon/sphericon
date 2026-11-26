// Package contacts is the one place a Contact is created, updated or upserted
// (ADR 0016). Every surface — the /site SPA API, the external /api, the tracker's
// Identify — hands it Attributes and gets back a Contact or a domain error; none
// re-implements what lies between "this person is wanted" and "the row is
// committed":
//
//   - alias-key normalization and resolution (subject_id, email, phone — ADR 0002);
//   - declared-by-use Custom fields (ADR 0006);
//   - the transaction, and the contact.created event published inside it so the
//     event is committed iff the row is (Automations enroll off it);
//   - domain errors (ErrNotFound, ErrEmailTaken and its kin, ErrIdentityRequired),
//     sentinel refusals the server's error handler answers alike on every surface.
//
// The Contact intake of an import (UpsertBatch) is the module's too: each row upserts its Contact
// and records the SMS Confirmation it carries (confirmations) in one transaction (ADR 0112).
//
// There is no repository layer over ent: the module uses the Workspace-scoped ent
// client (ADR 0017) it is handed, and writes no Workspace predicate.
package contacts

import (
	"context"
	"reflect"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/internal/kernel/optional"
	"github.com/getsphericon/sphericon/internal/kernel/tagname"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/tag"
	"github.com/getsphericon/sphericon/internal/domain/contacts/confirmations"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contactlock"
	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking"
	"github.com/getsphericon/sphericon/internal/domain/contacts/tags"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/addresscheck"
	"github.com/getsphericon/sphericon/internal/kernel/emailcase"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// Alias key fields, as the conflict of each names it.
const (
	FieldEmail     = "email"
	FieldPhone     = "phone"
	FieldSubjectID = "subject_id"
)

// ErrNotFound means the Contact does not exist in the Workspace.
var ErrNotFound = validation.NotFound("contacts: contact not found", i18n.ErrorsContactNotFound)

// ErrRevisionChanged means the update was made on a revision the Contact has moved past (ADR
// 0119): someone, or something (an Identify, an import, a Tag a Rule or an Automation applied),
// wrote it since the client read it. Nothing is written.
var ErrRevisionChanged = validation.RevisionChanged("contacts: contact changed since it was read", i18n.ErrorsContactRevisionChanged)

// ErrIdentityRequired means an upsert was given none of the alias keys, so there is
// nothing to resolve or anchor a Contact by.
var ErrIdentityRequired = validation.Refuse("contacts: subject_id, email or phone is required", i18n.ErrorsContactIdentityRequired)

// An alias key already held by another Contact of the Workspace, one per key: the answer names
// the key's field.
var (
	ErrEmailTaken     = validation.ConflictField(FieldEmail, "contacts: email already exists", i18n.ErrorsEmailExists)
	ErrPhoneTaken     = validation.ConflictField(FieldPhone, "contacts: phone already exists", i18n.ErrorsPhoneExists)
	ErrSubjectIDTaken = validation.ConflictField(FieldSubjectID, "contacts: subject_id already exists", i18n.ErrorsSubjectIdExists)
)

// A phone that cannot be stored in E.164 (ADR 0075), refused like an invalid email.
var (
	ErrPhoneInvalid         = validation.RefuseField(FieldPhone, "contacts: phone is not a phone number", i18n.ErrorsPhoneInvalid)
	ErrPhoneCountryRequired = validation.RefuseField(FieldPhone, "contacts: phone has no country code and the contact no country", i18n.ErrorsPhoneCountryRequired)
	ErrPhoneExtension       = validation.RefuseField(FieldPhone, "contacts: phone has an extension", i18n.ErrorsPhoneExtension)
)

// ErrEmailDomainNoMail means a Contact intake row brings an address new to the Workspace whose
// domain cannot receive mail (ADR 0117): it does not exist, has no mail exchanger or publishes a
// null MX.
var ErrEmailDomainNoMail = validation.RefuseField(FieldEmail, "contacts: the email's domain does not accept mail", i18n.ErrorsEmailDomainNoMail)

// constraints are the unique constraints of the Contact's tables (ADR 0106): an alias key is the
// Workspace's once, so a key another Contact holds is its conflict; a Custom field definition is
// written only by EnsureCustomFields, whose insert does nothing on a key the Workspace has.
var constraints = db.Declare(
	db.Refusal("contacts_subject_id_workspace_id", ErrSubjectIDTaken),
	db.Refusal("contacts_email_workspace_id", ErrEmailTaken),
	db.Refusal("contacts_phone_workspace_id", ErrPhoneTaken),
	db.Fault("custom_fields_workspace_id_key", "a definition is inserted ON CONFLICT (workspace_id, key) DO NOTHING, and its key never changes"),
)

// Attributes are the writable attributes of a new Contact. A nil pointer (or nil CustomFields)
// means "not given": the Contact has none.
type Attributes struct {
	SubjectID, Email, Phone       *string
	FirstName, LastName, TimeZone *string
	// Country is an ISO 3166-1 alpha-2 code and Locale a BCP 47 tag, both stored canonical.
	Country, Locale, City, Avatar *string
	Birthday                      *time.Time
	// CustomFields are declared by use (ADR 0006) and stored typed.
	CustomFields map[string]any
}

// Core is a patch of a Contact's core attributes (ADR 0072): a field that keeps leaves the stored
// value, a clear drops it, a set replaces it.
type Core struct {
	FirstName, LastName, TimeZone patch.Field[string]
	Country, Locale, City, Avatar patch.Field[string]
	Birthday                      patch.Field[time.Time]
}

// CreateInput is a new Contact: its Attributes and the Tags it is given, created on first use.
type CreateInput struct {
	Attributes

	Tags []string
}

// UpdateInput is a patch of a Contact (JSON Merge Patch): a field that keeps leaves the stored
// value, a clear drops it. ADR 0002 keeps every alias key optional, so clearing even the last one
// is valid; an alias key set blank keeps the stored one. A set CustomFields replaces the stored
// set, and set Tags become the Contact's complete tag set.
type UpdateInput struct {
	Core

	// Revision is the Contact's revision the edit was made on (ADR 0119).
	Revision int64

	SubjectID, Email, Phone patch.Field[string]
	CustomFields            patch.Field[map[string]any]
	Tags                    patch.Field[[]string]
}

// UpsertInput is a Contact asserted by the external API's upsert or the tracker's Identify (ADR
// 0072). Its alias keys resolve the Contact and fill only those it has none of (ADR 0002), its Core
// overwrites, and its CustomFields merge into the stored set. The guesses are the browser's time
// zone and language: they set an attribute only while the Contact has none and the Core leaves it.
type UpsertInput struct {
	Core

	SubjectID, Email, Phone        *string
	GuessedTimeZone, GuessedLocale *string
	CustomFields                   map[string]any

	// Claimed marks an assertion nobody proved: an Identify through the public collect key
	// without the customer's proof (ADR 0171). Its keys still resolve or create the Contact, and
	// a Contact it creates gets everything; a Contact that exists gains no alias key and only the
	// attributes and Custom fields it has no value of, never a replaced or cleared one.
	Claimed bool
}

// Result is the outcome of an Upsert.
type Result struct {
	Contact *ent.Contact
	// Created is true when no Contact matched and a new one was made.
	Created bool
	// Phone is the phone the upsert asserted, in E.164 (read with a country as upsertedPhone
	// reads it), nil when it asserted none. It is not the Contact's when the Contact had one
	// already: an upsert fills an alias key, never overwrites it.
	Phone *string
	// Flags are the traits of the address a Contact intake row asserted that the importer should
	// see (ADR 0117): a disposable address, a role's. Only UpsertBatch sets them.
	Flags []addresscheck.Flag
}

// Module is the contacts module.
type Module struct {
	bus   *events.Bus
	tags  *tags.Module
	locks *contactlock.Module
	// confirmations records the SMS consent an intake row carries.
	confirmations *confirmations.Module
	// tracking records the Engagement tracking consent an intake row carries.
	tracking *engagementtracking.Module
	mail     addresscheck.Resolver
	// now is the Instance clock a date a Contact holds is written by (keepDates).
	now func() time.Time
}

// New builds the module. The bus opens the transaction of a change and publishes its System events
// in the transaction its scope is bound to; catalogue is the Tags a Contact is given from; confs
// records the SMS consent an intake row carries, over the same bus, dated by its clock: when it
// reached the Workspace (ADR 0154); tracking records its Engagement tracking consent; mail is the
// DNS the Contact intake reads an address's domain from (ADR 0117), nil when there is none, so
// every domain is Unknown and the intake refuses no address for its domain; now is the Instance
// clock the dates a Contact holds are written by.
func New(bus *events.Bus, catalogue *tags.Module, confs *confirmations.Module, tracking *engagementtracking.Module, mail addresscheck.Resolver, now func() time.Time) *Module {
	return &Module{bus: bus, tags: catalogue, locks: contactlock.New(), confirmations: confs, tracking: tracking, mail: mail, now: now}
}

// Create makes a Contact from the given door (its Contact source, kept for good), gives it its
// Tags and publishes contact.created in one transaction, and returns it with its Tags. An alias
// key already taken in the Workspace yields its conflict (ErrEmailTaken and its kin).
func (m *Module) Create(ctx context.Context, s *ent.Scoped, src contact.Source, in CreateInput) (*ent.Contact, error) {
	attrs := in.normalized()
	var err error
	if attrs.Email, err = checked(attrs.Email, attrs.City, attrs.Country, attrs.Locale, attrs.Avatar); err != nil {
		return nil, err
	}
	if attrs.Phone, err = e164(attrs.Phone, attrs.Country); err != nil {
		return nil, err
	}
	var c *ent.Contact
	err = m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		typed, err := m.EnsureCustomFields(ctx, ts, attrs.CustomFields)
		if err != nil {
			return err
		}
		q := ts.Contact().Create().
			SetNillableSubjectID(attrs.SubjectID).
			SetNillableEmail(attrs.Email).
			SetNillablePhone(attrs.Phone).
			SetNillableFirstName(attrs.FirstName).
			SetNillableLastName(attrs.LastName).
			SetNillableTimeZone(attrs.TimeZone).
			SetNillableCountry(attrs.Country).
			SetNillableLocale(attrs.Locale).
			SetNillableBirthday(attrs.Birthday).
			SetNillableCity(attrs.City).
			SetNillableAvatar(attrs.Avatar).
			SetSource(src)
		if len(typed) > 0 {
			q = q.SetCustomFields(typed)
		}
		if err := constraints.Write(ctx, ts, func() (err error) { c, err = q.Save(ctx); return err }); err != nil {
			return err
		}
		if err := m.keepDates(ctx, ts, nil, c); err != nil {
			return err
		}
		if err := m.publishCreated(ctx, ts, c); err != nil {
			return err
		}
		if err := m.publishFieldChanges(ctx, ts, c, nil); err != nil {
			return err
		}
		if len(in.Tags) == 0 {
			return nil
		}
		return m.SetTagsIn(ctx, ts, c.ID, in.Tags)
	})
	if err != nil {
		return nil, domainError(err)
	}
	return m.Get(ctx, s, c.ID)
}

// Update applies a patch to a Contact, its Tags included, in one transaction and returns it with
// its Tags; each Custom field and Tag it changes is recorded as its System event. ErrNotFound when
// the Contact is not in the Workspace, and ErrRevisionChanged when it moved past the revision the
// edit was made on (ADR 0119): compared under the Contact lock every writer takes before anything
// else is checked, and again by the write, which is made on that revision.
func (m *Module) Update(ctx context.Context, s *ent.Scoped, id int64, in UpdateInput) (*ent.Contact, error) {
	in = in.normalized()
	if _, err := checked(in.Email.Ptr(), in.City.Ptr(), in.Country.Ptr(), in.Locale.Ptr(), in.Avatar.Ptr()); err != nil {
		return nil, err
	}
	err := m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		// Read under the update's lock, so the Custom fields compared are the ones it replaces.
		locked, err := m.locks.Lock(ctx, ts, id)
		if err != nil {
			return err
		}
		before := locked.Contact()
		if before.Revision != in.Revision {
			return ErrRevisionChanged
		}
		phone, err := patchedPhone(in, before.Country)
		if err != nil {
			return err
		}
		q := ts.Contact().EditAt(id, in.Revision).Refusing(ErrNotFound, ErrRevisionChanged)
		in.SubjectID.Apply(q.SetSubjectID, q.ClearSubjectID)
		in.Email.Apply(q.SetEmail, q.ClearEmail)
		phone.Apply(q.SetPhone, q.ClearPhone)
		applyCore(q, in.Core)
		if fields, ok := in.CustomFields.Get(); ok {
			typed, err := m.EnsureCustomFields(ctx, ts, fields)
			if err != nil {
				return err
			}
			q.SetCustomFields(typed)
		}
		if in.CustomFields.IsClear() {
			q.ClearCustomFields()
		}
		var after *ent.Contact
		err = constraints.Write(ctx, ts, func() (err error) { after, err = q.Save(ctx); return err })
		if err != nil {
			return err
		}
		if err := m.keepDates(ctx, ts, before, after); err != nil {
			return err
		}
		if err := m.publishFieldChanges(ctx, ts, after, before.CustomFields); err != nil {
			return err
		}
		names, ok := in.Tags.Get()
		if !ok {
			return nil
		}
		return m.SetTagsIn(ctx, ts, id, names)
	})
	if err != nil {
		return nil, domainError(err)
	}
	return m.Get(ctx, s, id)
}

// Upsert resolves the Contact by any present alias key (subject_id, email, phone)
// or creates one. On an existing Contact a missing alias key is filled (identity is
// additive), the core attributes given overwrite and null clears them (ADR 0072), the
// Custom fields merge, and its Contact source stays what it was: src only names the door
// of a Contact this call creates. ErrIdentityRequired when no alias key is given.
func (m *Module) Upsert(ctx context.Context, s *ent.Scoped, src contact.Source, in UpsertInput) (Result, error) {
	var res Result
	err := m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		var err error
		res, err = m.upsertIn(ctx, ts, src, in, recordChanges)
		return err
	})
	if err != nil {
		return Result{}, domainError(err)
	}
	return res, nil
}

// changes says whether an upsert records the changes of the Custom fields it writes: every
// upsert does but an import's (importIn).
type changes bool

const (
	recordChanges changes = true
	importChanges changes = false
)

// importSummary counts the rows of one import.
type importSummary struct {
	Created, Updated, Failed int
}

// recordImport records one contact.import Audit entry for a whole import: the actor of
// s and the row counts, never a Contact or a value. The Contact intake writes its rows through
// an unaudited scope (events.Unaudited) so only this entry reaches the log.
// The keys of a Contact import's Audit diff: the counts of its rows, a shape of the Audit log's own.
const (
	importDiffCreated = "created"
	importDiffUpdated = "updated"
	importDiffFailed  = "failed"
)

func (m *Module) recordImport(ctx context.Context, s *ent.Scoped, sum importSummary) error {
	return m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		return events.RecordAudit(ctx, pub, &events.AuditEntry{
			WorkspaceID: ts.WorkspaceID(),
			Actor:       s.Actor(),
			Action:      events.ActionContactImport,
			TargetType:  contact.Label,
			Diff: map[string]any{
				importDiffCreated: sum.Created,
				importDiffUpdated: sum.Updated,
				importDiffFailed:  sum.Failed,
			},
		})
	})
}

// fillMissing sets an attribute the matched Contact lacks: an Upsert fills, never overwrites.
func fillMissing(have, given *string, set func(*string) *ent.ContactScopedUpdateOne) {
	if have == nil {
		set(given)
	}
}

// UpsertIn is Upsert inside a transaction the caller already owns (s is bound to a transaction
// the module's bus opened), for flows that bind more rows to the Contact atomically — the
// tracker's Identify. It publishes contact.created when it creates, and contact.field_changed for
// each Custom field whose value it changes, in that transaction.
func (m *Module) UpsertIn(ctx context.Context, s *ent.Scoped, src contact.Source, in UpsertInput) (Result, error) {
	return m.upsertIn(ctx, s, src, in, recordChanges)
}

// importIn is UpsertIn for one row of an import (ADR 0022), inside the row's transaction of the
// Contact intake (UpsertBatch): it brings earlier state rather than something happening now, so it records no
// contact.field_changed (#897). Its refusals are the domain errors Upsert returns (ErrEmailTaken
// and its kin, ErrNotFound).
func (m *Module) importIn(ctx context.Context, s *ent.Scoped, src contact.Source, in UpsertInput) (Result, error) {
	res, err := m.upsertIn(ctx, s, src, in, importChanges)
	if err != nil {
		return Result{}, domainError(err)
	}
	return res, nil
}

func (m *Module) upsertIn(ctx context.Context, s *ent.Scoped, src contact.Source, in UpsertInput, ch changes) (Result, error) {
	in = in.normalized()
	if lo.FromPtr(in.SubjectID) == "" && in.Email == nil && in.Phone == nil {
		return Result{}, ErrIdentityRequired
	}
	email, err := checked(in.Email, in.City.Ptr(), in.Country.Ptr(), in.Locale.Ptr(), in.Avatar.Ptr())
	if err != nil {
		return Result{}, err
	}
	in.Email = email
	phone, err := upsertedPhone(ctx, s, in)
	if err != nil {
		return Result{}, err
	}
	in.Phone = phone
	typed, err := m.EnsureCustomFields(ctx, s, in.CustomFields)
	if err != nil {
		return Result{}, err
	}

	existing, err := resolve(ctx, s, AliasKeys{SubjectID: in.SubjectID, Email: in.Email, Phone: in.Phone})
	if err != nil {
		return Result{}, err
	}
	if existing != nil {
		c, err := m.enrich(ctx, s, existing.ID, in, typed, ch)
		if err != nil {
			return Result{}, err
		}
		return Result{Contact: c, Phone: in.Phone}, nil
	}

	q := s.Contact().Create().
		SetNillableSubjectID(in.SubjectID).
		SetNillableEmail(in.Email).
		SetNillablePhone(in.Phone).
		SetSource(src)
	createCore(q, in.withGuesses(nil, nil))
	if len(typed) > 0 {
		q.SetCustomFields(typed)
	}
	var c *ent.Contact
	if err := constraints.Write(ctx, s, func() (err error) { c, err = q.Save(ctx); return err }); err != nil {
		return Result{}, err
	}
	if err := m.keepDates(ctx, s, nil, c); err != nil {
		return Result{}, err
	}
	if err := m.publishCreated(ctx, s, c); err != nil {
		return Result{}, err
	}
	if ch == recordChanges {
		if err := m.publishFieldChanges(ctx, s, c, nil); err != nil {
			return Result{}, err
		}
	}
	return Result{Contact: c, Created: true, Phone: in.Phone}, nil
}

// withGuesses is the Core with the browser's guesses taking a time zone or locale the Core leaves
// and the Contact (its stored values given) has none of.
func (in UpsertInput) withGuesses(timeZone, locale *string) Core {
	c := in.Core
	if c.TimeZone.IsKeep() && timeZone == nil && in.GuessedTimeZone != nil {
		c.TimeZone = patch.Set(*in.GuessedTimeZone)
	}
	if c.Locale.IsKeep() && locale == nil && in.GuessedLocale != nil {
		c.Locale = patch.Set(*in.GuessedLocale)
	}
	return c
}

// applyCore writes a Core patch onto an update of a Contact.
func applyCore(q *ent.ContactScopedUpdateOne, c Core) {
	c.FirstName.Apply(q.SetFirstName, q.ClearFirstName)
	c.LastName.Apply(q.SetLastName, q.ClearLastName)
	c.TimeZone.Apply(q.SetTimeZone, q.ClearTimeZone)
	c.Country.Apply(q.SetCountry, q.ClearCountry)
	c.Locale.Apply(q.SetLocale, q.ClearLocale)
	c.City.Apply(q.SetCity, q.ClearCity)
	c.Avatar.Apply(q.SetAvatar, q.ClearAvatar)
	c.Birthday.Apply(q.SetBirthday, q.ClearBirthday)
}

// createCore sets the attributes a Core patch sets on a new Contact; a clear of one it never had
// is nothing.
func createCore(q *ent.ContactScopedCreate, c Core) {
	c.FirstName.ApplySet(q.SetFirstName)
	c.LastName.ApplySet(q.SetLastName)
	c.TimeZone.ApplySet(q.SetTimeZone)
	c.Country.ApplySet(q.SetCountry)
	c.Locale.ApplySet(q.SetLocale)
	c.City.ApplySet(q.SetCity)
	c.Avatar.ApplySet(q.SetAvatar)
	c.Birthday.ApplySet(q.SetBirthday)
}

// enrich fills the Contact's missing alias keys, writes the core attributes in gives and merges
// the typed Custom fields into its own, recording each field it changes unless ch is an import's.
// The Contact is read again under the lock the update takes anyway, so two Identifies of one
// Contact merge, and record, one after the other. An enrichment that changes nothing writes
// nothing: the Contact keeps its revision and updated_at, so an editor open on it can still save
// (ADR 0119), and a tracker that identifies on every page view does not rewrite the row.
func (m *Module) enrich(ctx context.Context, s *ent.Scoped, id int64, in UpsertInput, typed map[string]any, ch changes) (*ent.Contact, error) {
	locked, err := m.locks.Lock(ctx, s, id)
	if err != nil {
		return nil, err
	}
	existing := locked.Contact()
	if in.Claimed {
		in, typed = in.claimedOver(existing), fillOnlyFields(typed, existing.CustomFields)
	}
	core := in.withGuesses(existing.TimeZone, existing.Locale)
	if !enriches(existing, in, core, typed) {
		return existing, nil
	}
	q := s.Contact().UpdateOneID(existing.ID)
	fillMissing(existing.SubjectID, in.SubjectID, q.SetNillableSubjectID)
	fillMissing(existing.Email, in.Email, q.SetNillableEmail)
	fillMissing(existing.Phone, in.Phone, q.SetNillablePhone)
	applyCore(q, core)
	if len(typed) > 0 {
		q.SetCustomFields(lo.Assign(existing.CustomFields, typed))
	}
	var c *ent.Contact
	if err := constraints.Write(ctx, s, func() (err error) { c, err = q.Save(ctx); return err }); err != nil {
		return nil, err
	}
	if err := m.keepDates(ctx, s, existing, c); err != nil {
		return nil, err
	}
	if ch != recordChanges {
		return c, nil
	}
	return c, m.publishFieldChanges(ctx, s, c, existing.CustomFields)
}

// claimedOver is a claimed assertion as it may touch existing (ADR 0171): no alias key, since one
// would hand the Contact's mail or messages to whoever claimed it, and only the core attributes
// existing has no value of.
func (in UpsertInput) claimedOver(existing *ent.Contact) UpsertInput {
	in.SubjectID, in.Email, in.Phone = nil, nil, nil
	in.Core = Core{
		FirstName: fillOnly(in.FirstName, existing.FirstName),
		LastName:  fillOnly(in.LastName, existing.LastName),
		TimeZone:  fillOnly(in.TimeZone, existing.TimeZone),
		Country:   fillOnly(in.Country, existing.Country),
		Locale:    fillOnly(in.Locale, existing.Locale),
		City:      fillOnly(in.City, existing.City),
		Avatar:    fillOnly(in.Avatar, existing.Avatar),
		Birthday:  fillOnly(in.Birthday, existing.Birthday),
	}
	return in
}

// fillOnly is f where the Contact has no value of the attribute, and a keep where it has one or f
// would clear it.
func fillOnly[T any](f patch.Field[T], have *T) patch.Field[T] {
	if have != nil || f.IsClear() {
		return patch.Keep[T]()
	}
	return f
}

// fillOnlyFields is the Custom fields of typed that stored holds no value of.
func fillOnlyFields(typed, stored map[string]any) map[string]any {
	return lo.PickBy(typed, func(key string, _ any) bool { return stored[key] == nil })
}

// enriches reports whether enrich writes anything into existing: an alias key it lacks, a core
// attribute core sets to another value or clears, or a Custom field typed holds another value of,
// compared as publishFieldChanges compares them (storedValue), so the two never disagree.
func enriches(existing *ent.Contact, in UpsertInput, core Core, typed map[string]any) bool {
	fills := func(have, given *string) bool { return have == nil && given != nil }
	if fills(existing.SubjectID, in.SubjectID) || fills(existing.Email, in.Email) || fills(existing.Phone, in.Phone) {
		return true
	}
	same := func(a, b string) bool { return a == b }
	if differs(core.FirstName, existing.FirstName, same) || differs(core.LastName, existing.LastName, same) ||
		differs(core.TimeZone, existing.TimeZone, same) || differs(core.Country, existing.Country, same) ||
		differs(core.Locale, existing.Locale, same) || differs(core.City, existing.City, same) ||
		differs(core.Avatar, existing.Avatar, same) || differs(core.Birthday, existing.Birthday, time.Time.Equal) {
		return true
	}
	for key, value := range typed {
		if !reflect.DeepEqual(storedValue(existing.CustomFields[key]), storedValue(value)) {
			return true
		}
	}
	return false
}

// differs reports whether f laid over have leaves another value than have.
func differs[T any](f patch.Field[T], have *T, eq func(a, b T) bool) bool {
	return !optional.EqualFunc(f.Over(have), have, eq)
}

// AliasKeys are the alias keys a Contact is resolved by; a nil key is not asserted.
type AliasKeys struct {
	SubjectID, Email, Phone *string
}

// Resolve finds an existing Contact by any present alias key (subject_id → email →
// phone), or returns nil. It never creates. Keys are matched as normalized by
// Attributes, so callers may pass raw values: a phone is read as an international number, and one
// that is none (a local number, garbage) resolves nothing.
func (m *Module) Resolve(ctx context.Context, s *ent.Scoped, keys AliasKeys) (*ent.Contact, error) {
	return resolve(ctx, s, keys)
}

func resolve(ctx context.Context, s *ent.Scoped, by AliasKeys) (*ent.Contact, error) {
	a := Attributes{SubjectID: by.SubjectID, Email: by.Email, Phone: by.Phone}.normalized()
	if a.Phone != nil {
		if p, err := e164(a.Phone, nil); err == nil {
			a.Phone = p
		} else {
			a.Phone = nil
		}
	}
	var keys []predicate.Contact
	if a.SubjectID != nil {
		keys = append(keys, contact.SubjectID(*a.SubjectID))
	}
	if a.Email != nil {
		keys = append(keys, contact.Email(*a.Email))
	}
	if a.Phone != nil {
		keys = append(keys, contact.Phone(*a.Phone))
	}
	if len(keys) == 0 {
		return nil, nil
	}
	// One query for all the keys: each is unique in the Workspace, so it matches a row at most,
	// and the row of the first key in the order above wins.
	rows, err := s.Contact().Query().Where(contact.Or(keys...)).All(ctx)
	if err != nil {
		return nil, err
	}
	rank := func(c *ent.Contact) int {
		switch {
		case a.SubjectID != nil && lo.FromPtr(c.SubjectID) == *a.SubjectID:
			return 0
		case a.Email != nil && lo.FromPtr(c.Email) == *a.Email:
			return 1
		default:
			return 2
		}
	}
	return lo.MinBy(rows, func(x, y *ent.Contact) bool { return rank(x) < rank(y) }), nil
}

// ResolveID resolves an existing Contact by any present alias key (subject_id
// → email → phone) and returns its id, or 0 if none matches. It never creates a
// Contact — used by event ingest to attach an event to a Contact by stable identity
// when one already exists, leaving it anonymous (0) otherwise.
func (m *Module) ResolveID(ctx context.Context, s *ent.Scoped, keys AliasKeys) (int64, error) {
	c, err := resolve(ctx, s, keys)
	if err != nil || c == nil {
		return 0, err
	}
	return c.ID, nil
}

func (m *Module) publishCreated(ctx context.Context, s *ent.Scoped, c *ent.Contact) error {
	return m.bus.PublishIn(ctx, s, &events.ContactCreated{WorkspaceID: s.WorkspaceID(), ContactID: c.ID, Email: lo.FromPtr(c.Email)})
}

// normalized trims alias keys, lower-cases the email and drops blank ones, so
// "Alice@X.com " and "alice@x.com" are the same alias key everywhere.
// normalized trims the alias keys and lowercases the email; one set blank keeps the stored value.
func (in UpdateInput) normalized() UpdateInput {
	in.SubjectID = trimmedPatch(in.SubjectID)
	in.Phone = trimmedPatch(in.Phone)
	in.Email = trimmedPatch(in.Email).Map(emailcase.Of)
	in.Core = in.Core.normalized()
	return in
}

func (in UpsertInput) normalized() UpsertInput {
	keys := Attributes{SubjectID: in.SubjectID, Email: in.Email, Phone: in.Phone}.normalized()
	in.SubjectID, in.Email, in.Phone = keys.SubjectID, keys.Email, keys.Phone
	in.Core = in.Core.normalized()
	in.GuessedTimeZone = guess(in.GuessedTimeZone, timeZoneName)
	in.GuessedLocale = guess(in.GuessedLocale, localeTag)
	return in
}

func trimmedPatch(f patch.Field[string]) patch.Field[string] {
	v, ok := f.Get()
	if !ok {
		return f
	}
	t := trimmed(&v)
	if t == nil {
		return patch.Keep[string]()
	}
	return patch.Set(*t)
}

func (a Attributes) normalized() Attributes {
	a.SubjectID = trimmed(a.SubjectID)
	a.Phone = trimmed(a.Phone)
	if a.Email = trimmed(a.Email); a.Email != nil {
		a.Email = new(emailcase.Of(*a.Email))
	}
	if a.Country != nil {
		a.Country = new(strings.ToUpper(strings.TrimSpace(*a.Country)))
	}
	if a.Locale != nil {
		a.Locale = new(canonicalLocale(*a.Locale))
	}
	return a
}

func trimmed(s *string) *string {
	if s == nil {
		return nil
	}
	v := strings.TrimSpace(*s)
	if v == "" {
		return nil
	}
	return &v
}

// domainError turns storage errors into the module's domain errors.
func domainError(err error) error {
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	return err
}

// Get returns the Contact with its Tags loaded, sorted by name, or ErrNotFound.
func (m *Module) Get(ctx context.Context, s *ent.Scoped, id int64) (*ent.Contact, error) {
	c, err := s.Contact().Query().Where(contact.ID(id)).
		WithTags(func(q *ent.TagQuery) { q.Order(ent.Asc(tag.FieldName)) }).
		Only(ctx)
	if err != nil {
		return nil, domainError(err)
	}
	return c, nil
}

// Filter narrows a list of Contacts; the zero value keeps them all.
type Filter struct {
	// EmailContains keeps the Contacts whose address contains it, ignoring case.
	EmailContains string
	// Tag keeps the Contacts that carry the Tag of this exact name.
	Tag string
	// Country keeps the Contacts in this ISO 3166-1 alpha-2 country.
	Country string
}

// List returns one page of the Workspace's Contacts, ascending by id, narrowed by f.
func (m *Module) List(ctx context.Context, s *ent.Scoped, p pagination.Params, f Filter) (pagination.Page[*ent.Contact], error) {
	q := s.Contact().Query().WithTags(func(q *ent.TagQuery) { q.Order(ent.Asc(tag.FieldName)) })
	if f.EmailContains != "" {
		q = q.Where(contact.EmailContainsFold(f.EmailContains))
	}
	if f.Tag != "" {
		q = q.Where(contact.HasTagsWith(tag.NameKey(tagname.Key(f.Tag))))
	}
	if f.Country != "" {
		q = q.Where(contact.Country(f.Country))
	}
	return pagination.List(ctx, p, q.Order(ent.Asc(contact.FieldID)))
}
