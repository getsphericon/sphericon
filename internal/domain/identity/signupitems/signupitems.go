// Package signupitems is the Instance's Sign-up items and their Legal documents (ADR 0062): what a
// Staff member sets for the market the Instance serves, a required acceptance, an optional
// consent or a notice, each with a Markdown text that is drafted freely and published as
// immutable versions. It answers what a visitor is asked: the items in effect, each with its
// current version, and the current version of one item's document.
//
// The items belong to the Instance, not to a Workspace, so the package works on the raw
// *ent.Client (it is in the closed list of docs/architecture/workspace-scoping.md): it writes the Sign-up item,
// Legal document draft and Legal document version tables and nothing else, and reads the Staff
// member who published a version. Publishing is recorded on the version itself (who and when);
// the per-Workspace Audit log is not used.
package signupitems

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/legaldocumentdraft"
	"github.com/getsphericon/sphericon/ent/legaldocumentversion"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/signupitem"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/postgres"
	"github.com/getsphericon/sphericon/internal/kernel/bounds"
	"github.com/getsphericon/sphericon/internal/kernel/fieldname"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// latestFirst orders an item's versions with the current one first: the latest published, the
// later id deciding between two published at the same moment.
var latestFirst = []legaldocumentversion.OrderOption{
	ent.Desc(legaldocumentversion.FieldPublishedAt),
	ent.Desc(legaldocumentversion.FieldID),
}

var (
	// ErrNotFound: no Sign-up item has this id.
	ErrNotFound = validation.NotFound("signupitems: not found", i18n.ErrorsSignupItemNotFound)
	// ErrKeyTaken: another item has the key.
	ErrKeyTaken = validation.ConflictField(fieldname.Key, "signupitems: key taken", i18n.ErrorsSignupItemKeyTaken)
	// ErrTitleBlank: the title is blank once trimmed.
	ErrTitleBlank = validation.RefuseField(fieldname.Title, "signupitems: title blank", i18n.ErrorsSignupItemTitleBlank)
	// ErrMailingsKind: only an optional consent can stand for consent to the operator's mailings.
	ErrMailingsKind = validation.RefuseField(fieldname.MailingsConsent, "signupitems: mailings consent on another kind", i18n.ErrorsSignupItemMailingsKind)
	// ErrMailingsTaken: another item stands for consent to the operator's mailings.
	ErrMailingsTaken = validation.ConflictField(fieldname.MailingsConsent, "signupitems: mailings consent taken", i18n.ErrorsSignupItemMailingsTaken)
	// ErrNoDraft: the item has no draft to publish.
	ErrNoDraft = validation.Refuse("signupitems: no draft", i18n.ErrorsLegalDocumentNoDraft)
	// ErrLabelTaken: the item already has a version with the label.
	ErrLabelTaken = validation.ConflictField(fieldname.Label, "signupitems: label taken", i18n.ErrorsLegalDocumentLabelTaken)
	// ErrDocumentNotFound: no item in effect has this key, or it has no published version.
	ErrDocumentNotFound = validation.NotFound("signupitems: no legal document", i18n.ErrorsLegalDocumentNotFound)
	// ErrDeleteMailings: the item that stands for consent to the operator's mailings is unmarked
	// before it is deleted, so the mark can move to another item.
	ErrDeleteMailings = validation.RefuseField(fieldname.MailingsConsent, "signupitems: deleting the mailings consent", i18n.ErrorsSignupItemDeleteMailings)
	// ErrOrder: a new order that is not the id of every item in use exactly once.
	ErrOrder = validation.RefuseField(fieldname.IDs, "signupitems: order not every item once", i18n.ErrorsSignupItemOrder)
)

// constraints are the unique constraints of the Sign-up items and their Legal documents (ADR 0106).
var constraints = db.Declare(
	db.Refusal("signup_items_key_key", ErrKeyTaken),
	db.Refusal("signupitem_mailings_consent", ErrMailingsTaken),
	db.Refusal("legaldocumentversion_item_id_label", ErrLabelTaken),
	db.Fault("legal_document_drafts_item_id_key", "SaveDraft writes with ON CONFLICT on it: an item has one draft"),
)

// Item is a Sign-up item as the console shows it: the item, its draft and its current version.
type Item struct {
	ID              int64
	Key             string
	Title           string
	Kind            signupitem.Kind
	Position        int
	MailingsConsent bool
	CreatedAt       time.Time
	// DiscardedAt is when the item was deleted, nil while it is in use.
	DiscardedAt *time.Time
	// Draft is the Markdown of the next version, nil until one is written.
	Draft *string
	// CurrentVersion is the version in effect, nil until one is published.
	CurrentVersion *Version
}

// Version is a published Legal document version.
type Version struct {
	Label       string
	Markdown    string
	PublishedAt time.Time
	// PublishedBy is the Staff member who published it.
	PublishedBy *ent.StaffMember
}

// Document is a Sign-up item in effect with its current version: what a visitor is asked and
// reads.
type Document struct {
	// VersionID is the id of the current version, the one a decision is recorded on.
	VersionID   int64
	Key         string
	Title       string
	Kind        signupitem.Kind
	Label       string
	Markdown    string
	PublishedAt time.Time
}

// Input is a new Sign-up item.
type Input struct {
	Key             string
	Title           string
	Kind            signupitem.Kind
	MailingsConsent bool
}

// Module is the Instance's Sign-up items.
type Module struct {
	ent    *ent.Client
	bus    *events.Bus
	now    func() time.Time
	record AuditRecorder
}

// AuditRecorder writes one entry of the Instance audit log (ADR 0070) on tx, the transaction of the
// change: the Staff member staffID made the change action, with diff its fields' old and new values.
// The log is a module of the Instance context, above this one, so the wiring hands its Record in.
type AuditRecorder func(ctx context.Context, tx *ent.Client, staffID int64, action string, diff map[string]any) error

// The changes of the Sign-up items the Instance audit log records (ADR 0062, ADR 0070). Each diff
// names the item by its key: {to: key} for an added item, {from: key} for a deleted one, and
// {from: key, to: key} for one that stays, beside the fields that changed.
const (
	// ActionAdd records a new Sign-up item: its key, title, kind and mailings consent.
	ActionAdd = "instance.signup_item_add"
	// ActionChange records a change of an item's title or mailings consent.
	ActionChange = "instance.signup_item_change"
	// ActionDelete records a deleted item.
	ActionDelete = "instance.signup_item_delete"
	// ActionReorder records a new order of the items, as their keys (DiffOrder).
	ActionReorder = "instance.signup_items_reorder"
	// ActionPublish records a published version of an item's Legal document (DiffVersion, its
	// label); the text is the version's own, never copied into the entry.
	ActionPublish = "instance.legal_document_publish"
)

// AuditActions is every action of the module, which the Instance audit log declares as its own.
var AuditActions = []string{ActionAdd, ActionChange, ActionDelete, ActionReorder, ActionPublish}

// The diff keys of the entries that no column holds: the items' order and a version's label.
const (
	DiffOrder   = "order"
	DiffVersion = "version"
)

// New builds the module over the raw client and the bus, whose transaction carries a change of
// several rows (a new order) and the mailings consent mark (it changes what the service feed of
// Users reports, internal/domain/identity/usertombstones); now is the Instance clock of publishing and
// deleting, and record writes each change to the Instance audit log. Both are required.
func New(client *ent.Client, bus *events.Bus, now func() time.Time, record AuditRecorder) *Module {
	if now == nil || record == nil {
		panic("signupitems: the module needs the Instance clock and the Instance audit log")
	}
	return &Module{ent: client, bus: bus, now: now, record: record}
}

// List is every Sign-up item that is not deleted, in the order of the sign-up form.
func (m *Module) List(ctx context.Context) ([]Item, error) {
	rows, err := m.ent.SignupItem.Query().
		Where(signupitem.DiscardedAtIsNil()).
		WithDraft().
		Order(ent.Asc(signupitem.FieldPosition), ent.Asc(signupitem.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	current, err := m.currentVersions(ctx, rows)
	if err != nil {
		return nil, err
	}
	items := make([]Item, len(rows))
	for i, row := range rows {
		items[i] = item(row, current[row.ID])
	}
	return items, nil
}

// checkedTitle is a Sign-up item's title trimmed, ErrTitleBlank when nothing is left, and refused
// when it is longer than the contract's bound.
func checkedTitle(raw string) (string, error) {
	title := strings.TrimSpace(raw)
	if title == "" {
		return "", ErrTitleBlank
	}
	return title, validation.Length(fieldname.Title, title, bounds.SignupItemTitleMinLength, bounds.SignupItemTitleMaxLength)
}

// checkKey refuses a key outside the contract's length and pattern.
func checkKey(key string) error {
	if err := validation.Length(fieldname.Key, key, bounds.SignupItemKeyMinLength, bounds.SignupItemKeyMaxLength); err != nil {
		return err
	}
	return validation.Pattern(fieldname.Key, key, bounds.SignupItemKeyPattern)
}

// LockKey is the advisory lock of the Instance's Sign-up items, taken by every write that reads
// their positions (Add, Reorder) and held to the end of its transaction: two Adds never take one
// position, and two Reorders, which write the rows in their callers' orders, queue instead of
// deadlocking.
const LockKey = 7_204_003

// lockItems takes LockKey in the transaction tx.
func lockItems(ctx context.Context, tx *ent.Client) error {
	if err := postgres.LockTx(ctx, tx, LockKey); err != nil {
		return fmt.Errorf("sign-up items lock: %w", err)
	}
	return nil
}

// Add makes a Sign-up item after the existing ones. ErrKeyTaken when another item has the key,
// ErrTitleBlank for a blank title, ErrMailingsKind and ErrMailingsTaken for a mailings consent the
// item cannot hold. A lost race answers as the refusal it lost to.
func (m *Module) Add(ctx context.Context, staffID int64, in Input) (Item, error) {
	title, err := checkedTitle(in.Title)
	if err != nil {
		return Item{}, err
	}
	if err := checkKey(in.Key); err != nil {
		return Item{}, err
	}
	if err := mailingsKind(in.Kind, in.MailingsConsent); err != nil {
		return Item{}, err
	}
	var row *ent.SignupItem
	err = m.bus.WithinTx(ctx, func(tx *ent.Client, _ events.Publisher) error {
		if err := lockItems(ctx, tx); err != nil {
			return err
		}
		last, err := tx.SignupItem.Query().Order(ent.Desc(signupitem.FieldPosition)).First(ctx)
		position := 1
		switch {
		case err == nil:
			position = last.Position + 1
		case !ent.IsNotFound(err):
			return err
		}
		err = constraints.Write(ctx, tx, func() error {
			row, err = tx.SignupItem.Create().
				SetKey(in.Key).
				SetTitle(title).
				SetKind(in.Kind).
				SetPosition(position).
				SetMailingsConsent(in.MailingsConsent).
				Save(ctx)
			return err
		})
		if err != nil {
			return err
		}
		return m.record(ctx, tx, staffID, ActionAdd, map[string]any{
			signupitem.FieldKey:             events.Became(row.Key),
			signupitem.FieldTitle:           events.Became(row.Title),
			signupitem.FieldKind:            events.Became(string(row.Kind)),
			signupitem.FieldMailingsConsent: events.Became(row.MailingsConsent),
		})
	})
	if err != nil {
		return Item{}, err
	}
	return item(row, nil), nil
}

// inEffect is the one definition of an item in effect: not deleted, with a published version.
func inEffect() predicate.SignupItem {
	return signupitem.And(signupitem.DiscardedAtIsNil(), signupitem.HasVersions())
}

// InEffect is what the sign-up form asks: the items that are not deleted and have a published
// version, each with its current version, in the order of the form. Named kinds keep only the
// items of those kinds: the required acceptances are what a User must have accepted to use the
// app (ADR 0062). With no item it reads no version.
func (m *Module) InEffect(ctx context.Context, kinds ...signupitem.Kind) ([]Document, error) {
	where := []predicate.SignupItem{inEffect()}
	if len(kinds) > 0 {
		where = append(where, signupitem.KindIn(kinds...))
	}
	rows, err := m.ent.SignupItem.Query().
		Where(where...).
		Order(ent.Asc(signupitem.FieldPosition), ent.Asc(signupitem.FieldID)).
		All(ctx)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	current, err := m.currentVersions(ctx, rows)
	if err != nil {
		return nil, err
	}
	docs := make([]Document, len(rows))
	for i, row := range rows {
		docs[i] = document(row, current[row.ID])
	}
	return docs, nil
}

// Asked reports whether sign-up asks any Sign-up item: some item is in effect (see InEffect).
func (m *Module) Asked(ctx context.Context) (bool, error) {
	return m.ent.SignupItem.Query().Where(inEffect()).Exist(ctx)
}

// currentVersions is the current version of each of the items that has one: its latest
// published, the later id deciding between two published at the same moment. It is the one
// definition of "the current version".
func (m *Module) currentVersions(ctx context.Context, items []*ent.SignupItem) (map[int64]*ent.LegalDocumentVersion, error) {
	ids := make([]int64, len(items))
	for i, it := range items {
		ids[i] = it.ID
	}
	versions, err := m.ent.LegalDocumentVersion.Query().
		Where(legaldocumentversion.ItemIDIn(ids...)).
		WithPublishedBy().
		Order(latestFirst...).
		All(ctx)
	if err != nil {
		return nil, err
	}
	itemOf := func(v *ent.LegalDocumentVersion) int64 { return v.ItemID }
	// The versions come latest first, so the first of each item is its current one.
	return lo.KeyBy(lo.UniqBy(versions, itemOf), itemOf), nil
}

// item is the console's view of a row, its loaded draft and its current version.
func item(row *ent.SignupItem, current *ent.LegalDocumentVersion) Item {
	it := Item{
		ID:              row.ID,
		Key:             row.Key,
		Title:           row.Title,
		Kind:            row.Kind,
		Position:        row.Position,
		MailingsConsent: row.MailingsConsent,
		CreatedAt:       row.CreatedAt,
		DiscardedAt:     row.DiscardedAt,
	}
	if row.Edges.Draft != nil {
		it.Draft = &row.Edges.Draft.Markdown
	}
	if current != nil {
		it.CurrentVersion = new(version(current))
	}
	return it
}

// version is a published version row with its loaded publisher.
func version(row *ent.LegalDocumentVersion) Version {
	return Version{Label: row.Label, Markdown: row.Markdown, PublishedAt: row.PublishedAt, PublishedBy: row.Edges.PublishedBy}
}

// document is an item in effect with its current version.
func document(row *ent.SignupItem, current *ent.LegalDocumentVersion) Document {
	return Document{
		VersionID:   current.ID,
		Key:         row.Key,
		Title:       row.Title,
		Kind:        row.Kind,
		Label:       current.Label,
		Markdown:    current.Markdown,
		PublishedAt: current.PublishedAt,
	}
}

// Get is one Sign-up item, a deleted one too: what refers to it still shows it. ErrNotFound for an
// unknown id.
func (m *Module) Get(ctx context.Context, id int64) (Item, error) {
	row, err := m.ent.SignupItem.Query().Where(signupitem.ID(id)).WithDraft().Only(ctx)
	if ent.IsNotFound(err) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, err
	}
	current, err := m.currentVersions(ctx, []*ent.SignupItem{row})
	if err != nil {
		return Item{}, err
	}
	return item(row, current[row.ID]), nil
}

// UpdateInput is a change to a Sign-up item; a nil field is left as it is.
type UpdateInput struct {
	// Title is a new title: what the form shows at once, not a new version of the document.
	Title           *string
	MailingsConsent *bool
}

// Update changes a Sign-up item, recorded in the Instance audit log as the Staff member staffID's
// unless it changes nothing. ErrNotFound for an unknown or a deleted id, ErrTitleBlank for a blank
// title, ErrMailingsKind and ErrMailingsTaken for a mailings consent the item cannot hold.
func (m *Module) Update(ctx context.Context, staffID, id int64, u UpdateInput) (Item, error) {
	row, err := m.inUse(ctx, id)
	if err != nil {
		return Item{}, err
	}
	var title *string
	if u.Title != nil {
		trimmed, err := checkedTitle(*u.Title)
		if err != nil {
			return Item{}, err
		}
		title = &trimmed
	}
	if u.MailingsConsent != nil {
		if err := mailingsKind(row.Kind, *u.MailingsConsent); err != nil {
			return Item{}, err
		}
	}
	err = m.bus.WithinTx(ctx, func(tx *ent.Client, _ events.Publisher) error {
		if err := lockItems(ctx, tx); err != nil {
			return err
		}
		// An item deleted in between is not changed.
		was, err := tx.SignupItem.Query().Where(signupitem.ID(id), signupitem.DiscardedAtIsNil()).Only(ctx)
		if ent.IsNotFound(err) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		err = constraints.Write(ctx, tx, func() error {
			return tx.SignupItem.UpdateOneID(id).
				SetNillableTitle(title).
				SetNillableMailingsConsent(u.MailingsConsent).
				Exec(ctx)
		})
		if err != nil {
			return err
		}
		diff := map[string]any{}
		if entry, changed := events.ChangeOf(&was.Title, lo.CoalesceOrEmpty(title, &was.Title)); changed {
			diff[signupitem.FieldTitle] = entry
		}
		if entry, changed := events.ChangeOf(&was.MailingsConsent, lo.CoalesceOrEmpty(u.MailingsConsent, &was.MailingsConsent)); changed {
			diff[signupitem.FieldMailingsConsent] = entry
		}
		if len(diff) == 0 {
			return nil
		}
		diff[signupitem.FieldKey] = events.Change(was.Key, was.Key)
		return m.record(ctx, tx, staffID, ActionChange, diff)
	})
	if err != nil {
		return Item{}, err
	}
	return m.Get(ctx, id)
}

// Delete discards the item: it leaves the sign-up form, its public page and the list, and is never
// restored; its published versions, and the answers Users gave to them, stay. Deleting a deleted
// item keeps the moment it was first deleted. ErrNotFound for an unknown id, ErrDeleteMailings for
// the item that stands for consent to the operator's mailings. A delete that moves the item is
// recorded in the Instance audit log as the Staff member staffID's.
func (m *Module) Delete(ctx context.Context, staffID, id int64) error {
	row, err := m.ent.SignupItem.Get(ctx, id)
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if row.MailingsConsent {
		return ErrDeleteMailings
	}
	// Only an item in use moves: a second delete leaves the first moment, and an item marked as the
	// mailings consent in between is not deleted.
	err = m.bus.WithinTx(ctx, func(tx *ent.Client, _ events.Publisher) error {
		if err := lockItems(ctx, tx); err != nil {
			return err
		}
		if err := lifecycle.Latch(ctx, func(ctx context.Context) (bool, error) {
			moved, err := tx.SignupItem.Update().
				Where(signupitem.ID(id), signupitem.DiscardedAtIsNil(), signupitem.MailingsConsent(false)).
				SetDiscardedAt(m.now()).
				Save(ctx)
			return moved > 0, err
		}); err != nil {
			return err
		}
		return m.record(ctx, tx, staffID, ActionDelete,
			map[string]any{signupitem.FieldKey: map[string]any{events.DiffFrom: row.Key}})
	})
	if !errors.Is(err, lifecycle.ErrInvalidTransition) {
		return err
	}
	marked, err := m.ent.SignupItem.Query().Where(signupitem.ID(id), signupitem.MailingsConsent(true)).Exist(ctx)
	if err == nil && marked {
		return ErrDeleteMailings
	}
	return err
}

// inUse is the item with this id that a Staff member can still change: a deleted one is read-only
// (ADR 0100). ErrNotFound for an unknown or a deleted id.
func (m *Module) inUse(ctx context.Context, id int64) (*ent.SignupItem, error) {
	row, err := m.ent.SignupItem.Query().Where(signupitem.ID(id), signupitem.DiscardedAtIsNil()).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	return row, err
}

// Reorder puts the items in the order of ids, the order the sign-up form asks them in. ErrOrder
// unless ids is the id of every item that is not deleted exactly once; a deleted item keeps its
// place, which nothing reads. A new order is recorded in the Instance audit log as the Staff member
// staffID's; the same order records nothing.
func (m *Module) Reorder(ctx context.Context, staffID int64, ids []int64) ([]Item, error) {
	err := m.bus.WithinTx(ctx, func(tx *ent.Client, _ events.Publisher) error {
		if err := lockItems(ctx, tx); err != nil {
			return err
		}
		before, err := orderOf(ctx, tx)
		if err != nil {
			return err
		}
		if err := reorder(ctx, tx, ids); err != nil {
			return err
		}
		after, err := orderOf(ctx, tx)
		if err != nil || slices.Equal(before, after) {
			return err
		}
		return m.record(ctx, tx, staffID, ActionReorder,
			map[string]any{DiffOrder: events.Change(before, after)})
	})
	if err != nil {
		return nil, err
	}
	return m.List(ctx)
}

// orderOf is the keys of the items in use, in the order of the sign-up form, read in tx.
func orderOf(ctx context.Context, tx *ent.Client) ([]string, error) {
	return tx.SignupItem.Query().
		Where(signupitem.DiscardedAtIsNil()).
		Order(ent.Asc(signupitem.FieldPosition), ent.Asc(signupitem.FieldID)).
		Select(signupitem.FieldKey).
		Strings(ctx)
}

// reorder writes the positions of ids in the transaction tx, under LockKey, once it has checked
// they are every item in use exactly once.
func reorder(ctx context.Context, tx *ent.Client, ids []int64) error {
	if err := lockItems(ctx, tx); err != nil {
		return err
	}
	existing, err := tx.SignupItem.Query().Where(signupitem.DiscardedAtIsNil()).IDs(ctx)
	if err != nil {
		return err
	}
	if len(ids) != len(existing) || len(lo.Uniq(ids)) != len(ids) || len(lo.Intersect(existing, ids)) != len(existing) {
		return ErrOrder
	}
	for i, id := range ids {
		if err := tx.SignupItem.UpdateOneID(id).SetPosition(i + 1).Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Versions is every published version of the item's Legal document with its publisher, the latest
// (the current version) first. ErrNotFound for an unknown id.
func (m *Module) Versions(ctx context.Context, id int64) ([]Version, error) {
	exists, err := m.ent.SignupItem.Query().Where(signupitem.ID(id)).Exist(ctx)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	rows, err := m.ent.LegalDocumentVersion.Query().
		Where(legaldocumentversion.ItemID(id)).
		WithPublishedBy().
		Order(latestFirst...).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return lo.Map(rows, func(row *ent.LegalDocumentVersion, _ int) Version { return version(row) }), nil
}

// mailingsKind refuses a mailings consent on an item of another kind than an optional consent. A
// second item that stands for it is refused by signupitem_mailings_consent (ErrMailingsTaken).
func mailingsKind(kind signupitem.Kind, on bool) error {
	if on && kind != signupitem.KindOptionalConsent {
		return ErrMailingsKind
	}
	return nil
}

// SaveDraft replaces the draft of the item's Legal document. A visitor sees nothing of it until it
// is published. ErrNotFound for an unknown or a deleted id.
func (m *Module) SaveDraft(ctx context.Context, id int64, markdown string) (Item, error) {
	if err := validation.MaxLength(fieldname.Markdown, markdown, bounds.LegalDocumentMarkdownMaxLength); err != nil {
		return Item{}, err
	}
	if _, err := m.inUse(ctx, id); err != nil {
		return Item{}, err
	}
	err := m.ent.LegalDocumentDraft.Create().
		SetItemID(id).
		SetMarkdown(markdown).
		OnConflictColumns(legaldocumentdraft.FieldItemID).
		UpdateMarkdown().
		UpdateUpdatedAt().
		Exec(ctx)
	if err != nil {
		return Item{}, err
	}
	return m.Get(ctx, id)
}

// Publish makes the item's draft a new version under label, published by the Staff member
// staffID now: it takes effect at once and is never changed afterwards. ErrNotFound for an
// unknown or a deleted id, ErrNoDraft for an item without a draft, ErrLabelTaken for a label the
// item has. It is recorded in the Instance audit log too, beside the version's own publisher.
func (m *Module) Publish(ctx context.Context, id int64, label string, staffID int64) (Item, error) {
	if err := validation.Length(fieldname.Label, label, bounds.LegalDocumentLabelMinLength, bounds.LegalDocumentLabelMaxLength); err != nil {
		return Item{}, err
	}
	row, err := m.ent.SignupItem.Query().Where(signupitem.ID(id), signupitem.DiscardedAtIsNil()).WithDraft().Only(ctx)
	if ent.IsNotFound(err) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, err
	}
	if row.Edges.Draft == nil {
		return Item{}, ErrNoDraft
	}
	err = m.bus.WithinTx(ctx, func(tx *ent.Client, _ events.Publisher) error {
		err := constraints.Write(ctx, tx, func() error {
			return tx.LegalDocumentVersion.Create().
				SetItemID(id).
				SetLabel(label).
				SetMarkdown(row.Edges.Draft.Markdown).
				SetPublishedAt(m.now()).
				SetPublishedByID(staffID).
				Exec(ctx)
		})
		if err != nil {
			return err
		}
		return m.record(ctx, tx, staffID, ActionPublish, map[string]any{
			signupitem.FieldKey: events.Change(row.Key, row.Key),
			DiffVersion:         events.Became(label),
		})
	})
	if err != nil {
		return Item{}, err
	}
	return m.Get(ctx, id)
}

// Document is the current version of the Legal document of the item with this key, for its
// public page: only an item in effect has one. ErrDocumentNotFound for any other key.
func (m *Module) Document(ctx context.Context, key string) (Document, error) {
	row, err := m.ent.SignupItem.Query().Where(signupitem.Key(key), inEffect()).Only(ctx)
	if ent.IsNotFound(err) {
		return Document{}, ErrDocumentNotFound
	}
	if err != nil {
		return Document{}, err
	}
	current, err := m.currentVersions(ctx, []*ent.SignupItem{row})
	if err != nil {
		return Document{}, err
	}
	return document(row, current[row.ID]), nil
}
