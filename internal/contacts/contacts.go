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
// There is no repository layer over ent: the module uses the Workspace-scoped ent
// client (ADR 0017) it is handed, and writes no Workspace predicate.
package contacts

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/tag"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/patch"
	"github.com/getsphericon/sphericon/internal/tags"
	"github.com/getsphericon/sphericon/internal/validation"
)

// Alias key fields, as the conflict of each names it.
const (
	FieldEmail     = "email"
	FieldPhone     = "phone"
	FieldSubjectID = "subject_id"
)

// ErrNotFound means the Contact does not exist in the Workspace.
var ErrNotFound = validation.NotFound("contacts: contact not found", "errors.contact_not_found")

// ErrIdentityRequired means an upsert was given none of the alias keys, so there is
// nothing to resolve or anchor a Contact by.
var ErrIdentityRequired = validation.Refuse("contacts: subject_id, email or phone is required", "errors.contact_identity_required")

// An alias key already held by another Contact of the Workspace, one per key: the answer names
// the key's field.
var (
	ErrEmailTaken     = validation.ConflictField(FieldEmail, "contacts: email already exists", "errors.email_exists")
	ErrPhoneTaken     = validation.ConflictField(FieldPhone, "contacts: phone already exists", "errors.phone_exists")
	ErrSubjectIDTaken = validation.ConflictField(FieldSubjectID, "contacts: subject_id already exists", "errors.subject_id_exists")
)

// taken is the conflict of each alias key, in the order a unique violation is matched by its
// constraint's name.
var taken = []struct {
	field string
	err   error
}{{FieldSubjectID, ErrSubjectIDTaken}, {FieldPhone, ErrPhoneTaken}, {FieldEmail, ErrEmailTaken}}

// Attributes are the writable attributes of a new or upserted Contact. A nil pointer (or nil
// CustomFields) means "not given": the Contact has none.
type Attributes struct {
	SubjectID, Email, Phone       *string
	FirstName, LastName, TimeZone *string
	// CustomFields are declared by use (ADR 0006) and stored typed.
	CustomFields map[string]any
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
	SubjectID, Email, Phone       patch.Field[string]
	FirstName, LastName, TimeZone patch.Field[string]
	CustomFields                  patch.Field[map[string]any]
	Tags                          patch.Field[[]string]
}

// Result is the outcome of an Upsert.
type Result struct {
	Contact *ent.Contact
	// Created is true when no Contact matched and a new one was made.
	Created bool
}

// Module is the contacts module.
type Module struct {
	bus *events.Bus
}

// New builds the module.
func New(bus *events.Bus) *Module { return &Module{bus: bus} }

// Create makes a Contact from the given door (its Contact source, kept for good), gives it its
// Tags and publishes contact.created in one transaction, and returns it with its Tags. An alias
// key already taken in the Workspace yields its conflict (ErrEmailTaken and its kin).
func (m *Module) Create(ctx context.Context, s *ent.Scoped, src contact.Source, in CreateInput) (*ent.Contact, error) {
	attrs := in.normalized()
	var c *ent.Contact
	err := m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		typed, err := EnsureCustomFields(ctx, ts, attrs.CustomFields)
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
			SetSource(src)
		if len(typed) > 0 {
			q = q.SetCustomFields(typed)
		}
		c, err = q.Save(ctx)
		if err != nil {
			return err
		}
		if err := publishCreated(ctx, pub, ts.WorkspaceID(), c); err != nil {
			return err
		}
		if len(in.Tags) == 0 {
			return nil
		}
		return tags.New().Set(ctx, ts, c.ID, in.Tags)
	})
	if err != nil {
		return nil, domainError(err)
	}
	return m.Get(ctx, s, c.ID)
}

// Update applies a patch to a Contact, its Tags included, in one transaction and returns it with
// its Tags. ErrNotFound when the Contact is not in the Workspace.
func (m *Module) Update(ctx context.Context, s *ent.Scoped, id int64, in UpdateInput) (*ent.Contact, error) {
	in = in.normalized()
	err := m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		q := ts.Contact().UpdateOneID(id)
		patch.Apply(in.SubjectID, q.SetSubjectID, q.ClearSubjectID)
		patch.Apply(in.Email, q.SetEmail, q.ClearEmail)
		patch.Apply(in.Phone, q.SetPhone, q.ClearPhone)
		patch.Apply(in.FirstName, q.SetFirstName, q.ClearFirstName)
		patch.Apply(in.LastName, q.SetLastName, q.ClearLastName)
		patch.Apply(in.TimeZone, q.SetTimeZone, q.ClearTimeZone)
		if fields, ok := in.CustomFields.Get(); ok {
			typed, err := EnsureCustomFields(ctx, ts, fields)
			if err != nil {
				return err
			}
			q.SetCustomFields(typed)
		}
		if in.CustomFields.IsClear() {
			q.ClearCustomFields()
		}
		if err := q.Exec(ctx); err != nil {
			return err
		}
		names, ok := in.Tags.Get()
		if !ok {
			return nil
		}
		return tags.New().Set(ctx, ts, id, names)
	})
	if err != nil {
		return nil, domainError(err)
	}
	return m.Get(ctx, s, id)
}

// Upsert resolves the Contact by any present alias key (subject_id, email, phone)
// or creates one. An existing Contact is only enriched: missing attributes are
// filled and Custom fields merged, never overwritten (identity is additive), and its
// Contact source stays what it was: src only names the door of a Contact this call
// creates. ErrIdentityRequired when no alias key is given.
func (m *Module) Upsert(ctx context.Context, s *ent.Scoped, src contact.Source, attrs Attributes) (Result, error) {
	var res Result
	err := m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		var err error
		res, err = UpsertIn(ctx, ts, pub, src, attrs)
		return err
	})
	if err != nil {
		return Result{}, domainError(err)
	}
	return res, nil
}

// BatchOutcome is the result of one item of UpsertBatch: Result on success, Err (a
// domain error, or a storage error) otherwise.
type BatchOutcome struct {
	Result Result
	Err    error
}

// UpsertBatch upserts each item in its own transaction, in order, so a failing item
// neither rolls back nor blocks the others; each new Contact publishes contact.created
// with its own commit. The outcomes are parallel to items.
//
// A batch is an import (ADR 0022): its rows are written through an unaudited scope and
// the whole batch is recorded as ONE contact.import entry (counts only), not one entry
// per row. The rows commit one by one (a failing row must not poison the others), so
// the entry cannot share their transaction: if recording it fails the failure is
// logged and the committed outcomes are still returned, never a 500 for an import
// that happened.
func (m *Module) UpsertBatch(ctx context.Context, s *ent.Scoped, src contact.Source, items []Attributes) []BatchOutcome {
	rows := events.Unaudited(s)
	out := make([]BatchOutcome, len(items))
	var sum ImportSummary
	for i, attrs := range items {
		out[i].Result, out[i].Err = m.Upsert(ctx, rows, src, attrs)
		switch {
		case out[i].Err != nil:
			sum.Failed++
		case out[i].Result.Created:
			sum.Created++
		default:
			sum.Updated++
		}
	}
	if err := m.RecordImport(ctx, s, sum); err != nil {
		slog.ErrorContext(ctx, "record contact.import audit entry", "workspace_id", s.WorkspaceID(), "error", err)
	}
	return out
}

// ImportSummary counts the rows of one import.
type ImportSummary struct {
	Created, Updated, Failed int
}

// RecordImport records one contact.import Audit entry for a whole import: the actor of
// s and the row counts, never a Contact or a value. Importers write their rows through
// an unaudited scope (events.Ingest) so only this entry reaches the log.
func (m *Module) RecordImport(ctx context.Context, s *ent.Scoped, sum ImportSummary) error {
	return m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		return events.RecordAudit(ctx, pub, &events.AuditEntry{
			WorkspaceID: ts.WorkspaceID(),
			Actor:       s.Actor(),
			Action:      events.ActionContactImport,
			TargetType:  "contact",
			// contract-copy: audit diff keys, a shape of their own, not the contract's status enum
			Diff: map[string]any{
				"created": sum.Created,
				"updated": sum.Updated,
				"failed":  sum.Failed,
			},
		})
	})
}

// UpsertIn is Upsert inside a transaction the caller already owns (tx and pub come
// from events.Bus.WithinScopedTx), for flows that bind more rows to the Contact atomically
// — the tracker's Identify. It publishes contact.created when it creates.
func UpsertIn(ctx context.Context, s *ent.Scoped, pub events.Publisher, src contact.Source, attrs Attributes) (Result, error) {
	attrs = attrs.normalized()
	if lo.FromPtr(attrs.SubjectID) == "" && attrs.Email == nil && attrs.Phone == nil {
		return Result{}, ErrIdentityRequired
	}
	typed, err := EnsureCustomFields(ctx, s, attrs.CustomFields)
	if err != nil {
		return Result{}, err
	}

	existing, err := Resolve(ctx, s, attrs.SubjectID, attrs.Email, attrs.Phone)
	if err != nil {
		return Result{}, err
	}
	if existing != nil {
		q := s.Contact().UpdateOneID(existing.ID)
		if existing.SubjectID == nil {
			q.SetNillableSubjectID(attrs.SubjectID)
		}
		if existing.Email == nil {
			q.SetNillableEmail(attrs.Email)
		}
		if existing.Phone == nil {
			q.SetNillablePhone(attrs.Phone)
		}
		if existing.FirstName == nil {
			q.SetNillableFirstName(attrs.FirstName)
		}
		if existing.LastName == nil {
			q.SetNillableLastName(attrs.LastName)
		}
		if existing.TimeZone == nil {
			q.SetNillableTimeZone(attrs.TimeZone)
		}
		if len(typed) > 0 {
			q.SetCustomFields(lo.Assign(existing.CustomFields, typed))
		}
		c, err := q.Save(ctx)
		return Result{Contact: c}, err
	}

	q := s.Contact().Create().
		SetNillableSubjectID(attrs.SubjectID).
		SetNillableEmail(attrs.Email).
		SetNillablePhone(attrs.Phone).
		SetNillableFirstName(attrs.FirstName).
		SetNillableLastName(attrs.LastName).
		SetNillableTimeZone(attrs.TimeZone).
		SetSource(src)
	if len(typed) > 0 {
		q.SetCustomFields(typed)
	}
	c, err := q.Save(ctx)
	if err != nil {
		return Result{}, err
	}
	if err := publishCreated(ctx, pub, s.WorkspaceID(), c); err != nil {
		return Result{}, err
	}
	return Result{Contact: c, Created: true}, nil
}

// Resolve finds an existing Contact by any present alias key (subject_id → email →
// phone), or returns nil. It never creates. Keys are matched as normalized by
// Attributes, so callers may pass raw values.
func Resolve(ctx context.Context, s *ent.Scoped, subjectID, email, phone *string) (*ent.Contact, error) {
	a := Attributes{SubjectID: subjectID, Email: email, Phone: phone}.normalized()
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
	for _, key := range keys {
		c, err := s.Contact().Query().Where(key).First(ctx)
		if err == nil {
			return c, nil
		}
		if !ent.IsNotFound(err) {
			return nil, err
		}
	}
	return nil, nil
}

// ResolveID resolves an existing Contact by any present alias key (subject_id
// → email → phone) and returns its id, or 0 if none matches. It never creates a
// Contact — used by event ingest to attach an event to a Contact by stable identity
// when one already exists, leaving it anonymous (0) otherwise.
func ResolveID(ctx context.Context, s *ent.Scoped, subjectID string, email, phone *string) (int64, error) {
	c, err := Resolve(ctx, s, &subjectID, email, phone)
	if err != nil || c == nil {
		return 0, err
	}
	return c.ID, nil
}

func publishCreated(ctx context.Context, pub events.Publisher, workspaceID int64, c *ent.Contact) error {
	return pub.Publish(ctx, &events.ContactCreated{WorkspaceID: workspaceID, ContactID: c.ID, Email: lo.FromPtr(c.Email)})
}

// normalized trims alias keys, lower-cases the email and drops blank ones, so
// "Alice@X.com " and "alice@x.com" are the same alias key everywhere.
// normalized trims the alias keys and lowercases the email; one set blank keeps the stored value.
func (in UpdateInput) normalized() UpdateInput {
	in.SubjectID = trimmedPatch(in.SubjectID)
	in.Phone = trimmedPatch(in.Phone)
	in.Email = patch.Map(trimmedPatch(in.Email), strings.ToLower)
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
		a.Email = new(strings.ToLower(*a.Email))
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
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
		for _, k := range taken {
			if strings.Contains(pgErr.ConstraintName, k.field) {
				return k.err
			}
		}
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
}

// List returns one page of the Workspace's Contacts, ascending by id, narrowed by f.
func (m *Module) List(ctx context.Context, s *ent.Scoped, p pagination.Params, f Filter) (pagination.Page[*ent.Contact], error) {
	query := func() *ent.ContactQuery {
		q := s.Contact().Query().WithTags(func(q *ent.TagQuery) { q.Order(ent.Asc(tag.FieldName)) })
		if f.EmailContains != "" {
			q = q.Where(contact.EmailContainsFold(f.EmailContains))
		}
		return q
	}
	return pagination.List(ctx, p,
		func(ctx context.Context) (int, error) { return query().Count(ctx) },
		func(ctx context.Context, limit, offset int) ([]*ent.Contact, error) {
			return query().Order(ent.Asc(contact.FieldID)).Limit(limit).Offset(offset).All(ctx)
		})
}
