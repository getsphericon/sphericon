// Package broadcasts owns the Broadcast send/schedule state machine (ADR 0016):
// draft -> scheduled -> sending and clearing a schedule; the claim and the send job it
// needs are one transaction. Every surface that sends or schedules a Broadcast
// (the /site SPA today, /api and MCP next) calls it; none re-implements a
// transition. Handlers stay thin adapters: scope check, call, map errors.
//
// Every Broadcast status write goes through the transition table in status.go (ADR 0046):
// a stateless table with a compare-and-swap write, so an illegal move is
// lifecycle.ErrInvalidTransition. Delivery owns the rest of the lifecycle: plan, pace, send
// one recipient through Outbound send (ADR 0015), give up, finalize. The queue adapter (river)
// only implements its Scheduler seam and decides how to retry.
package broadcasts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/integration"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/segment"
	"github.com/getsphericon/sphericon/internal/emailrender"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/lifecycle"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/validation"
)

var (
	// ErrNotFound: no such Broadcast in this Workspace.
	ErrNotFound = validation.Refuse("broadcasts: broadcast not found", "errors.broadcast_not_found")
	// ErrNotSendable: the Broadcast is past draft/scheduled (sending, sent, failed).
	ErrNotSendable = validation.Refuse("broadcasts: broadcast is not a draft or scheduled", "errors.broadcast_not_sendable")
	// ErrNotArchivable: only a sent or failed Broadcast is archived; a draft is deleted and a
	// scheduled or sending one is still work in flight.
	ErrNotArchivable = validation.Refuse("broadcasts: only a sent or failed broadcast can be archived", "errors.broadcast_not_archivable")
	// ErrNotScheduled: the Broadcast is not scheduled, so there is no schedule to clear.
	ErrNotScheduled = validation.Refuse("broadcasts: broadcast is not scheduled", "errors.broadcast_not_scheduled")
)

// Queue is the river queue of the send jobs. A single blast fans out into one job per recipient;
// keeping them off the default queue stops a large broadcast from starving automation steps and
// welcome emails (the same isolation jobs.QueueWebhooks gives outbound webhooks).
const Queue = "broadcasts"

// SendArgs is the fan-out (plan) job payload: which Broadcast to dispatch. The worker plans
// the audience and queues one job per recipient, so no single job carries the whole send.
type SendArgs struct {
	BroadcastID int64 `json:"broadcast_id"`
	// ScheduledAt is the time a delayed job was queued for; nil for an immediate send.
	ScheduledAt *time.Time `json:"scheduled_at,omitempty"`
}

func (SendArgs) Kind() string { return "send_broadcast" }

// Module is the Broadcast send/schedule state machine. It holds no client: every
// call receives the Workspace-scoped client (ADR 0017) built by the entry point, so
// the module writes no Workspace predicate and cannot name another Workspace.
type Module struct {
	bus *events.Bus
}

func New(bus *events.Bus) *Module { return &Module{bus: bus} }

// Send moves a draft or scheduled Broadcast to sending (dropping any schedule) and queues
// its send job in the same transaction: either both happen or neither does.
func (m *Module) Send(ctx context.Context, s *ent.Scoped, id int64) (*ent.Broadcast, error) {
	return m.claim(ctx, s, id, triggerSend, func(u *ent.BroadcastScopedUpdate) { u.ClearScheduledAt() }, nil)
}

// Schedule moves a draft or scheduled Broadcast to scheduled at `when` and queues a delayed
// job in the same transaction.
func (m *Module) Schedule(ctx context.Context, s *ent.Scoped, id int64, when time.Time) (*ent.Broadcast, error) {
	return m.claim(ctx, s, id, triggerSchedule, func(u *ent.BroadcastScopedUpdate) { u.SetScheduledAt(when) }, &when)
}

// Unschedule returns a scheduled Broadcast to draft and drops its schedule. The
// delayed job stays queued; when it fires it finds a draft and does nothing.
func (m *Module) Unschedule(ctx context.Context, s *ent.Scoped, id int64) (*ent.Broadcast, error) {
	err := fire(ctx, s, id, triggerUnschedule, func(u *ent.BroadcastScopedUpdate) { u.ClearScheduledAt() })
	if errors.Is(err, lifecycle.ErrInvalidTransition) {
		return nil, fmt.Errorf("%w: %w", ErrNotScheduled, err)
	}
	if err != nil {
		return nil, err
	}
	return s.Broadcast().Get(ctx, id)
}

// claim makes a move out of draft or scheduled through the transition table and queues the
// send job (at `at`, nil for as soon as possible) in the same transaction. The write is a
// compare-and-swap on the status, so two racing requests cannot both win, and a job that
// cannot be queued rolls the claim back with it.
func (m *Module) claim(ctx context.Context, s *ent.Scoped, id int64, t trigger, set update, at *time.Time) (*ent.Broadcast, error) {
	var claimed *ent.Broadcast
	err := m.bus.WithinScopedEnqueueTx(ctx, s, func(ts *ent.Scoped, q events.Enqueuer) error {
		if err := fire(ctx, ts, id, t, set); err != nil {
			return notSendable(err)
		}
		opts := &river.InsertOpts{Queue: Queue}
		if at != nil {
			opts.ScheduledAt = *at
		}
		if err := q.Enqueue(ctx, SendArgs{BroadcastID: id, ScheduledAt: at}, opts); err != nil {
			return err
		}
		var err error
		claimed, err = ts.Broadcast().Get(ctx, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return claimed, nil
}

// --- Authoring (ADR 0016): drafts, Segment, report ---

var (
	// ErrNotDraft: the Broadcast is past draft (scheduled, sending, sent, failed) and
	// can no longer be edited, re-targeted or deleted through the authoring surface.
	ErrNotDraft = validation.Refuse("broadcasts: broadcast is not a draft", "errors.broadcast_not_draft")
	// ErrSegmentNotFound: the Segment does not exist in this Workspace.
	ErrSegmentNotFound = validation.Refuse("broadcasts: segment not found", "errors.broadcast_segment_not_found")
	// ErrIntegrationNotFound: the Integration does not exist in this Workspace or is archived.
	ErrIntegrationNotFound = validation.Refuse("broadcasts: integration not found", "errors.broadcast_integration_not_found")
	// ErrBodyAndDoc: a write carries both a body and a document. A client writes one of them: the
	// document (the server builds the body from it) or the body (which clears the document).
	ErrBodyAndDoc = validation.Refuse("broadcasts: write a body or a doc, not both", "errors.body_and_doc")
	// ErrBlankName: the Broadcast name is empty or whitespace only.
	ErrBlankName = validation.Refuse("broadcasts: name must not be empty", "errors.name_empty")
)

// Fields are the author-editable content of a Broadcast. A nil field is "not
// provided": Create falls back to the schema default, Update keeps the stored value.
// The Clear flags are JSON Merge Patch nulls for Update: they drop the stored value.
// SegmentID and IntegrationID reference rows of the same Workspace; the scoped
// client refuses a foreign one with ent.ErrNotInWorkspace.
type Fields struct {
	Name      *string
	Subject   *string
	FromName  *string
	FromEmail *string
	Body      *string
	// Doc is the editor's JSON document, empty when the Broadcast is written as HTML. The body is
	// built from it in the same save (ADR 0040); a Body alone clears the stored document.
	Doc           json.RawMessage
	SegmentID     *int64
	IntegrationID *int64

	ClearFromName    bool
	ClearFromEmail   bool
	ClearSegment     bool
	ClearIntegration bool
}

// checkName refuses a given name that is blank after trimming; an absent name is not checked.
func checkName(name *string) error {
	if name != nil && strings.TrimSpace(*name) == "" {
		return ErrBlankName
	}
	return nil
}

// Create makes a draft Broadcast. Nothing here schedules or sends. A blank name is ErrBlankName; a
// subject, body or document that cannot render is emailrender.ErrInvalidContent; the body is
// stored sanitized.
func (m *Module) Create(ctx context.Context, s *ent.Scoped, f Fields) (*ent.Broadcast, error) {
	if err := checkName(f.Name); err != nil {
		return nil, err
	}
	c, err := emailrender.ResolveContent(ctx, f.Subject, f.Body, f.Doc, ErrBodyAndDoc)
	if err != nil {
		return nil, err
	}
	if err := m.checkRefs(ctx, s, f); err != nil {
		return nil, err
	}
	body := c.Body
	q := s.Broadcast().Create().
		SetNillableFromName(f.FromName).
		SetNillableFromEmail(f.FromEmail).
		SetNillableSegmentID(f.SegmentID).
		SetNillableIntegrationID(f.IntegrationID)
	if f.Name != nil {
		q.SetName(*f.Name)
	}
	if f.Subject != nil {
		q.SetSubject(*f.Subject)
	}
	if body != nil {
		q.SetBody(*body)
	}
	if c.Authored != nil {
		q.SetDoc(f.Doc).SetEditorVersion(c.Authored.EditorVersion)
	}
	return q.Save(ctx)
}

// checkRefs refuses a Segment or Integration that is missing or archived: an archived row leaves
// new work (ADR 0049). A Broadcast already pointing at one keeps it as history.
func (m *Module) checkRefs(ctx context.Context, s *ent.Scoped, f Fields) error {
	if f.SegmentID != nil {
		ok, err := s.Segment().Query().Where(segment.ID(*f.SegmentID)).Exist(ctx)
		if err != nil {
			return err
		}
		if !ok {
			return ErrSegmentNotFound
		}
	}
	if f.IntegrationID != nil {
		ok, err := s.Integration().Query().Where(integration.ID(*f.IntegrationID)).Exist(ctx)
		if err != nil {
			return err
		}
		if !ok {
			return ErrIntegrationNotFound
		}
	}
	return nil
}

// Get returns one Broadcast of the Workspace.
func (m *Module) Get(ctx context.Context, s *ent.Scoped, id int64) (*ent.Broadcast, error) {
	b, err := s.Broadcast().Get(ctx, id)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	return b, err
}

// List returns one page of the Workspace's Broadcasts, newest first: the kept ones, or with
// archived the archived ones.
func (m *Module) List(ctx context.Context, s *ent.Scoped, p pagination.Params, archived bool) (pagination.Page[*ent.Broadcast], error) {
	rows := func() *ent.BroadcastQuery {
		if archived {
			return s.Broadcast().QueryDiscarded()
		}
		return s.Broadcast().Query()
	}
	return pagination.List(ctx, p,
		func(ctx context.Context) (int, error) { return rows().Count(ctx) },
		func(ctx context.Context, limit, offset int) ([]*ent.Broadcast, error) {
			return rows().Order(ent.Desc(broadcast.FieldID)).Limit(limit).Offset(offset).All(ctx)
		})
}

// Update edits a draft. The UPDATE only matches a draft row, so a Broadcast that was
// claimed for sending in the meantime is never edited. A blank name is ErrBlankName.
func (m *Module) Update(ctx context.Context, s *ent.Scoped, id int64, f Fields) (*ent.Broadcast, error) {
	if err := checkName(f.Name); err != nil {
		return nil, err
	}
	c, err := emailrender.ResolveContent(ctx, f.Subject, f.Body, f.Doc, ErrBodyAndDoc)
	if err != nil {
		return nil, err
	}
	if err := m.checkRefs(ctx, s, f); err != nil {
		return nil, err
	}
	u := s.Broadcast().Update().Where(draftOf(id)).
		SetNillableName(f.Name).
		SetNillableSubject(f.Subject).
		SetNillableFromName(f.FromName).
		SetNillableFromEmail(f.FromEmail).
		SetNillableBody(c.Body).
		SetNillableSegmentID(f.SegmentID).
		SetNillableIntegrationID(f.IntegrationID)
	switch {
	case c.Authored != nil:
		u.SetDoc(f.Doc).SetEditorVersion(c.Authored.EditorVersion)
	case c.Body != nil:
		u.ClearDoc().ClearEditorVersion()
	}
	if f.ClearFromName {
		u.ClearFromName()
	}
	if f.ClearFromEmail {
		u.ClearFromEmail()
	}
	if f.ClearSegment {
		u.ClearSegmentID()
	}
	if f.ClearIntegration {
		u.ClearIntegrationID()
	}
	return m.editDraft(ctx, s, id, u)
}

// SetSegment points a draft at a Segment, or at all contacts when segmentID
// is nil. It never schedules or sends.
func (m *Module) SetSegment(ctx context.Context, s *ent.Scoped, id int64, segmentID *int64) (*ent.Broadcast, error) {
	if segmentID != nil {
		ok, err := s.Segment().Query().Where(segment.ID(*segmentID)).Exist(ctx)
		if err != nil {
			return nil, err
		}
		if !ok {
			// A foreign or unknown broadcast is still reported as such first.
			if _, err := m.Get(ctx, s, id); err != nil {
				return nil, err
			}
			return nil, ErrSegmentNotFound
		}
	}
	u := s.Broadcast().Update().Where(draftOf(id)).SetNillableSegmentID(segmentID)
	if segmentID == nil {
		u.ClearSegmentID()
	}
	return m.editDraft(ctx, s, id, u)
}

// DeleteDraft purges a draft Broadcast (ADR 0049). Past draft the Broadcast is history (its
// recipients and report) and is archived instead, never removed.
func (m *Module) DeleteDraft(ctx context.Context, s *ent.Scoped, id int64) error {
	n, err := s.Broadcast().Purge().Where(draftOf(id)).Exec(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return m.notDraftOrNotFound(ctx, s, id)
	}
	return nil
}

// Archive discards a sent or failed Broadcast (ADR 0049): it leaves the lists but its Recipients,
// report, tracking and figures stay. A draft is purged (DeleteDraft) and a scheduled or sending
// Broadcast is work in flight, so both are ErrNotArchivable. Archiving an archived Broadcast
// changes nothing.
func (m *Module) Archive(ctx context.Context, s *ent.Scoped, id int64) error {
	err := s.Broadcast().Discard(ctx, id, broadcast.StatusIn(broadcast.StatusSent, broadcast.StatusFailed))
	if !ent.IsNotFound(err) {
		return err
	}
	b, err := m.Get(ctx, s, id)
	if err != nil {
		return err
	}
	if b.Status != broadcast.StatusSent && b.Status != broadcast.StatusFailed {
		return ErrNotArchivable
	}
	return nil
}

// Unarchive brings an archived Broadcast back to the lists. Restoring a Broadcast that is not
// archived changes nothing.
func (m *Module) Unarchive(ctx context.Context, s *ent.Scoped, id int64) error {
	err := s.Broadcast().Restore(ctx, id)
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	return err
}

// Report is a Broadcast's delivery report: its status, why it is held and its Stats. The
// rates are Stats methods, ratios in [0,1] where a zero denominator yields 0.
type Report struct {
	Stats

	Status     broadcast.Status
	HoldReason *string
}

// Report reads the Broadcast's delivery figures (see Stats).
func (m *Module) Report(ctx context.Context, s *ent.Scoped, id int64) (Report, error) {
	b, err := m.Get(ctx, s, id)
	if err != nil {
		return Report{}, err
	}
	figures, err := StatsOf(ctx, s, id)
	if err != nil {
		return Report{}, err
	}
	return Report{Status: b.Status, HoldReason: b.HoldReason, Stats: figures[id]}, nil
}

func draftOf(id int64) predicate.Broadcast {
	return broadcast.And(broadcast.ID(id), broadcast.StatusEQ(broadcast.StatusDraft))
}

func (m *Module) editDraft(ctx context.Context, s *ent.Scoped, id int64, u *ent.BroadcastScopedUpdate) (*ent.Broadcast, error) {
	n, err := u.Save(ctx)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, m.notDraftOrNotFound(ctx, s, id)
	}
	return m.Get(ctx, s, id)
}

func (m *Module) notDraftOrNotFound(ctx context.Context, s *ent.Scoped, id int64) error {
	if _, err := m.Get(ctx, s, id); err != nil {
		return err
	}
	return ErrNotDraft
}
