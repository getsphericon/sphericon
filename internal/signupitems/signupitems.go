// Package signupitems is the Instance's Sign-up items and their Legal documents (ADR 0062): what a
// Staff member sets for the market the Instance serves, a required acceptance, an optional
// consent or a notice, each with a Markdown text that is drafted freely and published as
// immutable versions. It answers what a visitor is asked: the items in effect, each with its
// current version, and the current version of one item's document.
//
// The items belong to the Instance, not to a Workspace, so the package works on the raw
// *ent.Client (it is in the closed list of docs/architecture.md): it writes the Sign-up item,
// Legal document draft and Legal document version tables and nothing else, and reads the Staff
// member who published a version. Publishing is recorded on the version itself (who and when);
// the per-Workspace Audit log is not used.
package signupitems

import (
	"context"
	"strings"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/legaldocumentdraft"
	"github.com/getsphericon/sphericon/ent/legaldocumentversion"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/signupitem"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/validation"
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
	ErrKeyTaken = validation.RefuseField("key", "signupitems: key taken", i18n.ErrorsSignupItemKeyTaken)
	// ErrTitleBlank: the title is blank once trimmed.
	ErrTitleBlank = validation.RefuseField("title", "signupitems: title blank", i18n.ErrorsSignupItemTitleBlank)
	// ErrMailingsKind: only an optional consent can stand for consent to the operator's mailings.
	ErrMailingsKind = validation.RefuseField("mailingsConsent", "signupitems: mailings consent on another kind", i18n.ErrorsSignupItemMailingsKind)
	// ErrMailingsTaken: another item stands for consent to the operator's mailings.
	ErrMailingsTaken = validation.RefuseField("mailingsConsent", "signupitems: mailings consent taken", i18n.ErrorsSignupItemMailingsTaken)
	// ErrNoDraft: the item has no draft to publish.
	ErrNoDraft = validation.Refuse("signupitems: no draft", i18n.ErrorsLegalDocumentNoDraft)
	// ErrLabelTaken: the item already has a version with the label.
	ErrLabelTaken = validation.RefuseField("label", "signupitems: label taken", i18n.ErrorsLegalDocumentLabelTaken)
	// ErrDocumentNotFound: no item in effect has this key, or it has no published version.
	ErrDocumentNotFound = validation.NotFound("signupitems: no legal document", i18n.ErrorsLegalDocumentNotFound)
	// ErrMailingsArchived: an archived item cannot stand for consent to the operator's mailings.
	ErrMailingsArchived = validation.RefuseField("mailingsConsent", "signupitems: mailings consent on an archived item", i18n.ErrorsSignupItemMailingsArchived)
	// ErrArchiveMailings: the item that stands for consent to the operator's mailings is unmarked
	// before it is archived, so the mark can move to another item.
	ErrArchiveMailings = validation.RefuseField("mailingsConsent", "signupitems: archiving the mailings consent", i18n.ErrorsSignupItemArchiveMailings)
	// ErrOrder: a new order that is not the id of every item exactly once.
	ErrOrder = validation.RefuseField("ids", "signupitems: order not every item once", i18n.ErrorsSignupItemOrder)
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
	// ArchivedAt is when the item was archived, nil while it is in use.
	ArchivedAt *time.Time
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
	ent *ent.Client
	bus *events.Bus
	now func() time.Time
}

// New builds the module over the raw client and the bus, whose transaction carries a change of
// several rows (a new order) and the mailings consent mark (it changes what the service feed of
// Users reports, internal/userfeed); now is the clock of publishing and archiving, nil is
// time.Now.
func New(client *ent.Client, bus *events.Bus, now func() time.Time) *Module {
	if now == nil {
		now = time.Now
	}
	return &Module{ent: client, bus: bus, now: now}
}

// List is every Sign-up item, archived ones included, in the order of the sign-up form.
func (m *Module) List(ctx context.Context) ([]Item, error) {
	rows, err := m.ent.SignupItem.Query().
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

// Add makes a Sign-up item after the existing ones. ErrKeyTaken when another item has the key,
// ErrTitleBlank for a blank title, ErrMailingsKind and ErrMailingsTaken for a mailings consent the
// item cannot hold. A lost race answers as the refusal it lost to.
func (m *Module) Add(ctx context.Context, in Input) (Item, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return Item{}, ErrTitleBlank
	}
	taken, err := m.ent.SignupItem.Query().Where(signupitem.Key(in.Key)).Exist(ctx)
	if err != nil {
		return Item{}, err
	}
	if taken {
		return Item{}, ErrKeyTaken
	}
	if err := m.checkMailings(ctx, 0, in.Kind, in.MailingsConsent); err != nil {
		return Item{}, err
	}
	last, err := m.ent.SignupItem.Query().Order(ent.Desc(signupitem.FieldPosition)).First(ctx)
	position := 1
	switch {
	case err == nil:
		position = last.Position + 1
	case !ent.IsNotFound(err):
		return Item{}, err
	}
	row, err := m.ent.SignupItem.Create().
		SetKey(in.Key).
		SetTitle(title).
		SetKind(in.Kind).
		SetPosition(position).
		SetMailingsConsent(in.MailingsConsent).
		Save(ctx)
	if ent.IsConstraintError(err) {
		return Item{}, m.lostRace(ctx, in.Key)
	}
	if err != nil {
		return Item{}, err
	}
	return item(row, nil), nil
}

// lostRace is the refusal of an Add that a unique index refused: the key, or else the mailings
// consent, was taken in between.
func (m *Module) lostRace(ctx context.Context, key string) error {
	taken, err := m.ent.SignupItem.Query().Where(signupitem.Key(key)).Exist(ctx)
	if err != nil {
		return err
	}
	if taken {
		return ErrKeyTaken
	}
	return ErrMailingsTaken
}

// inEffect is the one definition of an item in effect: not archived, with a published version.
func inEffect() predicate.SignupItem {
	return signupitem.And(signupitem.DiscardedAtIsNil(), signupitem.HasVersions())
}

// InEffect is what the sign-up form asks: the items that are not archived and have a published
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
		ArchivedAt:      row.DiscardedAt,
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

// Get is one Sign-up item. ErrNotFound for an unknown id.
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

// Update is a change to a Sign-up item; a nil field is left as it is.
type Update struct {
	// Title is a new title: what the form shows at once, not a new version of the document.
	Title           *string
	MailingsConsent *bool
}

// Update changes a Sign-up item. ErrNotFound for an unknown id, ErrTitleBlank for a blank title,
// ErrMailingsKind, ErrMailingsArchived and ErrMailingsTaken for a mailings consent the item cannot
// hold.
func (m *Module) Update(ctx context.Context, id int64, u Update) (Item, error) {
	row, err := m.ent.SignupItem.Get(ctx, id)
	if ent.IsNotFound(err) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, err
	}
	var title *string
	if u.Title != nil {
		trimmed := strings.TrimSpace(*u.Title)
		if trimmed == "" {
			return Item{}, ErrTitleBlank
		}
		title = &trimmed
	}
	if u.MailingsConsent != nil {
		if *u.MailingsConsent && row.DiscardedAt != nil {
			return Item{}, ErrMailingsArchived
		}
		if err := m.checkMailings(ctx, row.ID, row.Kind, *u.MailingsConsent); err != nil {
			return Item{}, err
		}
	}
	err = m.bus.WithinTx(ctx, func(tx *ent.Client, _ events.Publisher) error {
		return tx.SignupItem.UpdateOneID(id).
			SetNillableTitle(title).
			SetNillableMailingsConsent(u.MailingsConsent).
			Exec(ctx)
	})
	if ent.IsConstraintError(err) {
		return Item{}, ErrMailingsTaken
	}
	if err != nil {
		return Item{}, err
	}
	return m.Get(ctx, id)
}

// Archive takes the item out of the sign-up form and its public page; its published versions
// stay. Archiving an archived item keeps the moment it was first archived. ErrNotFound for an
// unknown id, ErrArchiveMailings for the item that stands for consent to the operator's mailings.
func (m *Module) Archive(ctx context.Context, id int64) (Item, error) {
	row, err := m.ent.SignupItem.Get(ctx, id)
	if ent.IsNotFound(err) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, err
	}
	if row.MailingsConsent {
		return Item{}, ErrArchiveMailings
	}
	// Only an item in use moves: a second archive leaves the first moment, and an item marked as the
	// mailings consent in between is not archived.
	_, err = m.ent.SignupItem.Update().
		Where(signupitem.ID(id), signupitem.DiscardedAtIsNil(), signupitem.MailingsConsent(false)).
		SetDiscardedAt(m.now()).
		Save(ctx)
	if err != nil {
		return Item{}, err
	}
	return m.Get(ctx, id)
}

// Reorder puts the items in the order of ids, the order the sign-up form asks them in. ErrOrder
// unless ids is the id of every item, archived ones included, exactly once.
func (m *Module) Reorder(ctx context.Context, ids []int64) ([]Item, error) {
	err := m.bus.WithinTx(ctx, func(tx *ent.Client, _ events.Publisher) error {
		return reorder(ctx, tx, ids)
	})
	if err != nil {
		return nil, err
	}
	return m.List(ctx)
}

// reorder writes the positions of ids in the transaction tx, once it has checked they are every
// item exactly once.
func reorder(ctx context.Context, tx *ent.Client, ids []int64) error {
	existing, err := tx.SignupItem.Query().IDs(ctx)
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

// checkMailings refuses a mailings consent on an item of another kind than an optional consent,
// and a second item that stands for it. id is the item being changed, 0 for a new one.
func (m *Module) checkMailings(ctx context.Context, id int64, kind signupitem.Kind, on bool) error {
	if !on {
		return nil
	}
	if kind != signupitem.KindOptionalConsent {
		return ErrMailingsKind
	}
	taken, err := m.ent.SignupItem.Query().Where(signupitem.MailingsConsent(true), signupitem.IDNEQ(id)).Exist(ctx)
	if err != nil {
		return err
	}
	if taken {
		return ErrMailingsTaken
	}
	return nil
}

// SaveDraft replaces the draft of the item's Legal document. A visitor sees nothing of it until it
// is published. ErrNotFound for an unknown id.
func (m *Module) SaveDraft(ctx context.Context, id int64, markdown string) (Item, error) {
	exists, err := m.ent.SignupItem.Query().Where(signupitem.ID(id)).Exist(ctx)
	if err != nil {
		return Item{}, err
	}
	if !exists {
		return Item{}, ErrNotFound
	}
	err = m.ent.LegalDocumentDraft.Create().
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
// unknown id, ErrNoDraft for an item without a draft, ErrLabelTaken for a label the item has.
func (m *Module) Publish(ctx context.Context, id int64, label string, staffID int64) (Item, error) {
	row, err := m.ent.SignupItem.Query().Where(signupitem.ID(id)).WithDraft().Only(ctx)
	if ent.IsNotFound(err) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, err
	}
	if row.Edges.Draft == nil {
		return Item{}, ErrNoDraft
	}
	taken, err := m.ent.LegalDocumentVersion.Query().
		Where(legaldocumentversion.ItemID(id), legaldocumentversion.LabelEQ(label)).
		Exist(ctx)
	if err != nil {
		return Item{}, err
	}
	if taken {
		return Item{}, ErrLabelTaken
	}
	err = m.ent.LegalDocumentVersion.Create().
		SetItemID(id).
		SetLabel(label).
		SetMarkdown(row.Edges.Draft.Markdown).
		SetPublishedAt(m.now()).
		SetPublishedByID(staffID).
		Exec(ctx)
	if ent.IsConstraintError(err) {
		return Item{}, ErrLabelTaken
	}
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
