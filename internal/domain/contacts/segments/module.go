// Package segments is the Segment domain module: named Conditions on Contacts, with the rows they
// select and the live work that uses them.
package segments

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/segment"
	"github.com/getsphericon/sphericon/ent/tag"
	"github.com/getsphericon/sphericon/internal/domain/contacts/condition"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// Domain errors returned by Module. Callers match with errors.Is.
var (
	// ErrInvalidCondition: the condition does not parse or does not fit the
	// Contact schema. The wrapped message is safe to show to the author.
	ErrInvalidCondition = validation.Refuse("segments: invalid condition", i18n.ErrorsSegmentConditionInvalid)
	// ErrNotFound: no such Segment in this Workspace.
	ErrNotFound = validation.NotFound("segments: segment not found", i18n.ErrorsSegmentNotFound)
	// ErrArchived: the Segment is archived, so it is read-only until it is unarchived (ADR 0100).
	ErrArchived = validation.Conflict("segments: segment is archived", i18n.ErrorsSegmentArchived)
	// ErrRevisionChanged: the update was made on a revision the Segment has moved past (ADR 0119):
	// someone wrote it since the client read it. Nothing is written.
	ErrRevisionChanged = validation.RevisionChanged("segments: segment changed since it was read", i18n.ErrorsSegmentRevisionChanged)
	// ErrUnknownSender: an event test's filter names a Broadcast or an Automation the Workspace does
	// not have (ADR 0153), an unknown reference. The message names the test's path and the id.
	ErrUnknownSender = validation.UnknownReference("segments: unknown sender", i18n.ErrorsSegmentSenderUnknown)
	// ErrBadCursor: a cursor of a Segment's Contacts that no page handed out.
	ErrBadCursor = validation.UnknownReference("segments: invalid cursor", i18n.ErrorsSegmentCursorInvalid)
)

// RefuseUnknownSender is the refusal of a Condition whose event test's filter names the sender n,
// which the Workspace does not have: the segment saving use case asks messagesenders, a module
// above this one, and refuses in this module's words.
func RefuseUnknownSender(n condition.NamedSender) error {
	return ErrUnknownSender.With(n.Missing())
}

// Module is the one place a Segment is validated, previewed, counted and archived (ADR 0016). Its
// save is segmentsunchecked's (ADR 0110): a Condition may name a Broadcast or an Automation, which
// only the segment saving use case can ask about, so create and update run there, over the one
// validation rule set here. Membership is the rule alone — Send-eligibility
// is subtracted only at send, never folded into a count (ADR 0001).
type Module struct {
	bus *events.Bus
	// now is the Instance clock an archive is dated by.
	now func() time.Time
	// conditions compiles a rule over the same clock, which a relative date counts back from.
	conditions  *condition.Module
	eligibility *eligibility.Module
	// keptDays is how many days of analytical Events the Instance keeps (ADR 0019), 0 for every
	// one: an event test that reaches past them counts only the Events kept, which a count says.
	keptDays int
}

// New builds the Segments module. It holds no client: every call takes the
// Workspace's scoped client (ADR 0017); the bus runs a save and the archive each in one
// transaction under the Workspace's lock (ADR 0099). now is the Instance clock; eventsKept the
// Instance's Event retention (config.EventsRetention), 0 when Events are kept forever.
func New(bus *events.Bus, now func() time.Time, eventsKept time.Duration) *Module {
	return &Module{bus: bus, now: now, conditions: condition.New(now), eligibility: eligibility.New(), keptDays: int(eventsKept / (24 * time.Hour))}
}

// EventsKeptDays is how many days of analytical Events the Instance keeps (ADR 0019), 0 when it
// keeps every one: an event test with no window, or a longer one, counts only the Events kept.
func (s *Module) EventsKeptDays() int { return s.keptDays }

// CreateInput is a new Segment. A nil Condition is the empty group: no tests yet, every Contact.
type CreateInput struct {
	Name      string
	Condition *conditiontree.Segment
}

// UpdateInput is a patch of a Segment: a field that keeps leaves the stored value. Neither field
// can be cleared; a Condition with no tests matches all contacts.
type UpdateInput struct {
	// Revision is the Segment's revision the edit was made on (ADR 0119).
	Revision  int64
	Name      patch.Field[string]
	Condition patch.Field[conditiontree.Segment]
}

// Validate checks a condition against the Contact attributes and the Workspace's Custom
// field catalogue. A condition with no tests is valid. This is the single rule set behind
// a save (segmentsunchecked), Preview and Count.
func (s *Module) Validate(ctx context.Context, ws *ent.Scoped, cond conditiontree.Segment) error {
	if _, err := s.conditions.Compile(ctx, ws, cond); err != nil {
		return ErrInvalidCondition.With(err.Error())
	}
	return nil
}

// Get returns one Segment. An unknown id is ErrNotFound.
func (s *Module) Get(ctx context.Context, ws *ent.Scoped, id int64) (*ent.Segment, error) {
	seg, err := ws.Segment().Get(ctx, id)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	return seg, err
}

// RefuseArchived is ErrArchived for an archived Segment, which is read-only until it is unarchived:
// the refusal of an update conditioned on its not being archived (segmentsunchecked, Archive).
func RefuseArchived(s *ent.Segment) error {
	if s.ArchivedAt != nil {
		return ErrArchived
	}
	return nil
}

// PreviewInput is an unsaved Condition to preview, nil for every Contact, and the Channel of a
// Broadcast the audience is asked for, with the Integration it routes through (nil: the Channel's
// default). Without a Channel the preview counts the match alone.
type PreviewInput struct {
	Condition     *conditiontree.Segment
	Channel       *channels.Channel
	IntegrationID *int64
}

// Counts is what a Segment's Condition counts, previewed or stored: the Contacts it matches, the
// ones it leaves out because their engagement is unknown (ADR 0123), and, when a Channel was asked
// for, how many of the matching ones a Broadcast there would take now
// (eligibility.Module.BroadcastAudience).
type Counts struct {
	Count int
	// UnknownEngagement is the Contacts left out because a count of their opens or clicks is
	// unknown: their Destination on its Channel is not tracked now (condition.Module.LeftOut).
	UnknownEngagement int
	Recipients        *int
	// EventsKeptDays is how many days of analytical Events the Instance keeps, set when an event
	// test of the Condition reaches past them and so counts only the Events kept (ADR 0153).
	EventsKeptDays *int
}

// Preview counts the Contacts in the Workspace that match an unsaved Condition, the ones it leaves
// out for unknown engagement, and the recipients of a Broadcast on the Channel the input names. A
// Condition that does not compile is ErrInvalidCondition.
func (s *Module) Preview(ctx context.Context, ws *ent.Scoped, in PreviewInput) (Counts, error) {
	cond := conditiontree.Segment{Group: conditiontree.Empty()}
	if in.Condition != nil {
		cond = *in.Condition
	}
	pred, err := s.conditions.Compile(ctx, ws, cond)
	if err != nil {
		return Counts{}, ErrInvalidCondition.With(err.Error())
	}
	count, err := ws.Contact().Query().Where(pred).Count(ctx)
	if err != nil {
		return Counts{}, err
	}
	res := Counts{Count: count}
	if condition.ReachesPastKept(cond.Group, s.keptDays) {
		kept := s.keptDays
		res.EventsKeptDays = &kept
	}
	if condition.CanBeUnknown(cond.Group) {
		leftOut, err := s.conditions.LeftOut(ctx, ws, cond)
		if err != nil {
			return Counts{}, ErrInvalidCondition.With(err.Error())
		}
		res.UnknownEngagement, err = ws.Contact().Query().Where(leftOut).Count(ctx)
		if err != nil {
			return Counts{}, err
		}
	}
	if in.Channel == nil {
		return res, nil
	}
	reached, err := s.eligibility.BroadcastAudience(ctx, ws, *in.Channel, in.IntegrationID)
	if err != nil {
		return Counts{}, err
	}
	recipients, err := ws.Contact().Query().Where(reached, pred).Count(ctx)
	if err != nil {
		return Counts{}, err
	}
	res.Recipients = &recipients
	return res, nil
}

// Count counts the Contacts matching a stored Segment, evaluated live, and the ones it leaves out
// because their engagement is unknown, as Preview does.
func (s *Module) Count(ctx context.Context, ws *ent.Scoped, id int64) (Counts, error) {
	seg, err := ws.Segment().Get(ctx, id)
	if ent.IsNotFound(err) {
		return Counts{}, ErrNotFound
	}
	if err != nil {
		return Counts{}, err
	}
	return s.Preview(ctx, ws, PreviewInput{Condition: &seg.Condition})
}

// ContactsInput names a page of Contacts to read by cursor: a stored Segment's, or every Contact of
// the Workspace when SegmentID is nil.
type ContactsInput struct {
	SegmentID *int64
	// After is the id of the last Contact of the page before, 0 for the first page.
	After int64
	// Limit is the most Contacts on the page, the contract's default page size when below 1. It is
	// not held to the contract's maximum: an export reads larger pages.
	Limit int
}

// PageInput names a page of a stored Segment's Contacts as a client reads them: by the cursor the
// page before handed out, empty for the first.
type PageInput struct {
	SegmentID int64
	Cursor    string
	// Limit is as ContactsInput's.
	Limit int
}

// ContactPage is a page of a Segment's Contacts and the cursor of the page after it, empty on the
// last.
type ContactPage struct {
	Contacts []*ent.Contact
	Next     string
}

// ContactsPage is Contacts read by the cursor a page handed out (pagination.IDCursor): one no page
// handed out is ErrBadCursor.
func (s *Module) ContactsPage(ctx context.Context, ws *ent.Scoped, in PageInput) (ContactPage, error) {
	after, ok := pagination.ParseIDCursor(in.Cursor)
	if !ok {
		return ContactPage{}, ErrBadCursor
	}
	page, err := s.Contacts(ctx, ws, ContactsInput{SegmentID: &in.SegmentID, After: after, Limit: in.Limit})
	if err != nil {
		return ContactPage{}, err
	}
	out := ContactPage{Contacts: page.Items}
	if page.Next > 0 {
		out.Next = pagination.IDCursor(page.Next)
	}
	return out, nil
}

// Contacts returns one page of the Contacts a stored Segment matches, evaluated live, or of every
// Contact without a Segment, ascending by id after the cursor in.After (a keyset cursor, ADR 0138):
// a Contact that becomes a member while the pages are read neither shifts the next page nor is read
// twice, and one whose id the cursor has not passed yet is read when it does. The page's Next is
// the cursor of the page after it, 0 on the last. An unknown Segment is ErrNotFound.
func (s *Module) Contacts(ctx context.Context, ws *ent.Scoped, in ContactsInput) (pagination.IDPage[*ent.Contact], error) {
	cond := conditiontree.Segment{Group: conditiontree.Empty()}
	if in.SegmentID != nil {
		seg, err := ws.Segment().Get(ctx, *in.SegmentID)
		if ent.IsNotFound(err) {
			return pagination.IDPage[*ent.Contact]{}, ErrNotFound
		}
		if err != nil {
			return pagination.IDPage[*ent.Contact]{}, err
		}
		cond = seg.Condition
	}
	pred, err := s.conditions.Compile(ctx, ws, cond)
	if err != nil {
		return pagination.IDPage[*ent.Contact]{}, ErrInvalidCondition.With(err.Error())
	}
	limit := in.Limit
	if limit < 1 {
		limit = pagination.DefaultPageSize
	}
	rows, err := ws.Contact().Query().Where(pred, contact.IDGT(in.After)).
		WithTags(func(q *ent.TagQuery) { q.Order(ent.Asc(tag.FieldName)) }).
		Order(ent.Asc(contact.FieldID)).
		Limit(limit + 1).
		All(ctx)
	if err != nil {
		return pagination.IDPage[*ent.Contact]{}, err
	}
	return pagination.ByID(rows, limit, func(c *ent.Contact) int64 { return c.ID }), nil
}

// List returns one page of the Workspace's Segments, ascending by id: the kept ones, or with
// archived the archived ones.
func (s *Module) List(ctx context.Context, ws *ent.Scoped, p pagination.Params, archived bool) (pagination.Page[*ent.Segment], error) {
	q := ws.Segment().Query()
	if archived {
		q = ws.Segment().QueryArchived()
	}
	return pagination.List(ctx, p, q.Order(ent.Asc(segment.FieldID)))
}

// Archive puts a Segment aside (ADR 0100): it leaves the lists and new work, and Unarchive brings
// it back. Only Live use would refuse it (ADR 0099), and none needs a Segment: a draft Broadcast
// is not Live use (scheduling it is then refused), and a Broadcast past draft holds a copy of the
// Condition (ADR 0065) and keeps its reference only as provenance. A missing Segment is
// ErrNotFound; archiving an archived one changes nothing. It returns the Segment. The archive
// holds the Workspace's lock (ADR 0099), so a draft being scheduled meanwhile either took its copy
// first or finds the Segment archived.
func (s *Module) Archive(ctx context.Context, ws *ent.Scoped, id int64) (*ent.Segment, error) {
	err := s.bus.WithinWorkspaceLockTx(ctx, ws, func(wl *ent.WorkspaceLocked, _ events.Publisher) error {
		seg, err := wl.Segment().Get(ctx, id)
		if ent.IsNotFound(err) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if seg.ArchivedAt != nil {
			return nil
		}
		return wl.Segment().Archive(ctx, id, s.now())
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, ws, id)
}

// Unarchive brings an archived Segment back to the lists and returns it. Unarchiving one that is not
// archived changes nothing; a missing Segment is ErrNotFound.
func (s *Module) Unarchive(ctx context.Context, ws *ent.Scoped, id int64) (*ent.Segment, error) {
	err := ws.Segment().Unarchive(ctx, id)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, ws, id)
}
