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
	"entgo.io/ent/dialect/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/segment"
	"github.com/getsphericon/sphericon/internal/emaildoc"
	"github.com/getsphericon/sphericon/internal/emailrender"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/lifecycle"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/patch"
	"github.com/getsphericon/sphericon/internal/validation"
)

var (
	// ErrNotFound: no such Broadcast in this Workspace.
	ErrNotFound = validation.NotFound("broadcasts: broadcast not found", i18n.ErrorsBroadcastNotFound)
	// ErrNotSendable: the Broadcast is past draft/scheduled (sending, sent, failed).
	ErrNotSendable = validation.Refuse("broadcasts: broadcast is not a draft or scheduled", i18n.ErrorsBroadcastNotSendable)
	// ErrNotArchivable: only a sent or failed Broadcast is archived; a draft is deleted and a
	// scheduled or sending one is still work in flight.
	ErrNotArchivable = validation.Refuse("broadcasts: only a sent or failed broadcast can be archived", i18n.ErrorsBroadcastNotArchivable)
	// ErrNotScheduled: the Broadcast is not scheduled, so there is no schedule to clear.
	ErrNotScheduled = validation.Refuse("broadcasts: broadcast is not scheduled", i18n.ErrorsBroadcastNotScheduled)
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
	bus      *events.Bus
	renderer *emaildoc.Renderer
	out      *outbound.Module
}

// New builds the module. The bus runs a claim and its send job in one transaction; the renderer
// builds a body from its document; out is Outbound send, whose Send route Schedule asks whether the
// Broadcast can send and the progress estimate paces by.
func New(bus *events.Bus, renderer *emaildoc.Renderer, out *outbound.Module) *Module {
	return &Module{bus: bus, renderer: renderer, out: out}
}

// Send moves a draft or scheduled Broadcast to sending (dropping any schedule) and queues
// its send job in the same transaction: either both happen or neither does.
func (m *Module) Send(ctx context.Context, s *ent.Scoped, id int64) (*ent.Broadcast, error) {
	return m.claim(ctx, s, id, triggerSend, func(u *ent.BroadcastScopedUpdate) { u.ClearScheduledAt() }, nil, nil)
}

// Schedule moves a draft or scheduled Broadcast to scheduled at `when` and queues a delayed
// job in the same transaction. Scheduling arms the send, so a Broadcast whose Send route cannot
// send (its Integration is disabled or archived, or the Workspace has no default) is refused with
// outbound.ErrRouteHeld and stays as it was; a route that stops sending after it holds the plan.
func (m *Module) Schedule(ctx context.Context, s *ent.Scoped, id int64, when time.Time) (*ent.Broadcast, error) {
	return m.claim(ctx, s, id, triggerSchedule, func(u *ent.BroadcastScopedUpdate) { u.SetScheduledAt(when) }, &when, m.armable)
}

// armable refuses a claimed Broadcast whose Send route cannot send.
func (m *Module) armable(ctx context.Context, ts *ent.Scoped, b *ent.Broadcast) error {
	route, err := m.out.Route(ctx, ts, b.IntegrationID)
	if err != nil {
		return err
	}
	return route.Armable()
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
// cannot be queued rolls the claim back with it. check, when given, may refuse the claimed
// Broadcast after the move, which rolls the move back.
func (m *Module) claim(ctx context.Context, s *ent.Scoped, id int64, t trigger, set update, at *time.Time, check func(context.Context, *ent.Scoped, *ent.Broadcast) error) (*ent.Broadcast, error) {
	var claimed *ent.Broadcast
	err := m.bus.WithinScopedEnqueueTx(ctx, s, func(ts *ent.Scoped, q events.Enqueuer) error {
		if err := fire(ctx, ts, id, t, set); err != nil {
			return notSendable(err)
		}
		var err error
		if claimed, err = ts.Broadcast().Get(ctx, id); err != nil {
			return err
		}
		if check != nil {
			if err := check(ctx, ts, claimed); err != nil {
				return err
			}
		}
		opts := &river.InsertOpts{Queue: Queue}
		if at != nil {
			opts.ScheduledAt = *at
		}
		return q.Enqueue(ctx, SendArgs{BroadcastID: id, ScheduledAt: at}, opts)
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
	ErrNotDraft = validation.Refuse("broadcasts: broadcast is not a draft", i18n.ErrorsBroadcastNotDraft)
	// ErrSegmentNotFound: the Segment does not exist in this Workspace.
	ErrSegmentNotFound = validation.Refuse("broadcasts: segment not found", i18n.ErrorsBroadcastSegmentNotFound)
	// ErrIntegrationNotFound: the Integration does not exist in this Workspace or is archived.
	ErrIntegrationNotFound = validation.Refuse("broadcasts: integration not found", i18n.ErrorsBroadcastIntegrationNotFound)
	// ErrBodyAndDoc: a write carries both a body and a document. A client writes one of them: the
	// document (the server builds the body from it) or the body (which clears the document).
	ErrBodyAndDoc = validation.Refuse("broadcasts: write a body or a doc, not both", i18n.ErrorsBodyAndDoc)
	// ErrFromNotVerified: the From email's domain is not a verified Sending domain of this
	// Workspace (ADR 0010).
	ErrFromNotVerified = validation.Refuse("broadcasts: from email must be on a verified sending domain", i18n.ErrorsBroadcastFromNotVerified)
	// ErrBlankName: the Broadcast name is empty or whitespace only.
	ErrBlankName = validation.Refuse("broadcasts: name must not be empty", i18n.ErrorsNameEmpty)
)

// CreateInput is a new draft Broadcast. A nil field is "not provided": it takes the schema
// default. SegmentID and IntegrationID reference rows of the same Workspace; the scoped client
// refuses a foreign one with ent.ErrNotInWorkspace.
type CreateInput struct {
	Name      *string
	Subject   *string
	FromName  *string
	FromEmail *string
	Body      *string
	// Doc is the editor's JSON document, empty when the Broadcast is written as HTML. The body is
	// built from it in the same save (ADR 0040).
	Doc           json.RawMessage
	SegmentID     *int64
	IntegrationID *int64
}

// UpdateInput is a patch of a draft Broadcast: a field that keeps leaves the stored value, a clear
// drops it. Name, Subject, Body and Doc cannot be cleared; a Body alone clears the stored
// document. FromName and FromEmail clear to the Integration's default, SegmentID to all contacts
// and IntegrationID to the Workspace default.
type UpdateInput struct {
	Name          patch.Field[string]
	Subject       patch.Field[string]
	FromName      patch.Field[string]
	FromEmail     patch.Field[string]
	Body          patch.Field[string]
	Doc           patch.Field[json.RawMessage]
	SegmentID     patch.Field[int64]
	IntegrationID patch.Field[int64]
}

// checkName refuses a given name that is blank after trimming; an absent name is not checked.
func checkName(name *string) error {
	if name != nil && strings.TrimSpace(*name) == "" {
		return ErrBlankName
	}
	return nil
}

// checkFrom refuses a From email whose domain is not a verified Sending domain of the Workspace,
// by the same check the send gate uses (ADR 0010); the refusal names the domain. The caller asks
// only when a write sets or changes the From, so a Broadcast held for a domain that lost
// verification stays editable. The Integration plays no part: the identity is independent of it.
func checkFrom(ctx context.Context, s *ent.Scoped, from string) error {
	ok, err := messaging.HasVerifiedSendingDomain(ctx, s, from)
	if err != nil {
		return err
	}
	if !ok {
		return ErrFromNotVerified.With(messaging.DomainOf(from))
	}
	return nil
}

// setsFrom reports whether a write sets a non-empty From other than the stored one (nil on create).
func setsFrom(from, stored *string) bool {
	return from != nil && *from != "" && (stored == nil || *stored != *from)
}

// Create makes a draft Broadcast. Nothing here schedules or sends. A blank name is ErrBlankName; a
// subject, body or document that cannot render is emailrender.ErrInvalidContent; the body is
// stored sanitized.
func (m *Module) Create(ctx context.Context, s *ent.Scoped, f CreateInput) (*ent.Broadcast, error) {
	if err := checkName(f.Name); err != nil {
		return nil, err
	}
	c, err := emailrender.ResolveContent(ctx, m.renderer, f.Subject, f.Body, f.Doc, ErrBodyAndDoc)
	if err != nil {
		return nil, err
	}
	if err := m.checkRefs(ctx, s, f.SegmentID, f.IntegrationID); err != nil {
		return nil, err
	}
	if setsFrom(f.FromEmail, nil) {
		if err := checkFrom(ctx, s, *f.FromEmail); err != nil {
			return nil, err
		}
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
func (m *Module) checkRefs(ctx context.Context, s *ent.Scoped, segmentID, integrationID *int64) error {
	if segmentID != nil {
		ok, err := s.Segment().Query().Where(segment.ID(*segmentID)).Exist(ctx)
		if err != nil {
			return err
		}
		if !ok {
			return ErrSegmentNotFound
		}
	}
	if integrationID != nil {
		ok, err := messaging.IntegrationKept(ctx, s, *integrationID)
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
func (m *Module) List(ctx context.Context, s *ent.Scoped, p pagination.Params, archived bool, statuses []broadcast.Status, order Order) (pagination.Page[*ent.Broadcast], error) {
	rows := func() *ent.BroadcastQuery {
		q := s.Broadcast().Query()
		if archived {
			q = s.Broadcast().QueryDiscarded()
		}
		if len(statuses) > 0 {
			q = q.Where(broadcast.StatusIn(statuses...))
		}
		return q
	}
	return pagination.List(ctx, p,
		func(ctx context.Context) (int, error) { return rows().Count(ctx) },
		func(ctx context.Context, limit, offset int) ([]*ent.Broadcast, error) {
			return rows().Order(order.terms()...).Limit(limit).Offset(offset).All(ctx)
		})
}

// Order is the order of the Broadcast list.
type Order string

const (
	// OrderNewest is the newest first.
	OrderNewest Order = "newest"
	// OrderScheduledSoonest is the one scheduled soonest first, the unscheduled after.
	OrderScheduledSoonest Order = "scheduledSoonest"
	// OrderSentLatest is the one sent last first, the unsent after.
	OrderSentLatest Order = "sentLatest"
)

// terms is the ORDER BY of o; the newest first breaks every tie, so a page is stable.
func (o Order) terms() []broadcast.OrderOption {
	newest := broadcast.ByID(sql.OrderDesc())
	switch o {
	case OrderScheduledSoonest:
		return []broadcast.OrderOption{broadcast.ByScheduledAt(sql.OrderAsc(), sql.OrderNullsLast()), newest}
	case OrderSentLatest:
		return []broadcast.OrderOption{broadcast.BySentAt(sql.OrderDesc(), sql.OrderNullsLast()), newest}
	case OrderNewest:
	}
	return []broadcast.OrderOption{newest}
}

// Update applies a patch to a draft and returns it. The UPDATE only matches a draft row, so a
// Broadcast that was claimed for sending in the meantime is never edited. A blank name is
// ErrBlankName; a Segment or Integration set here is checked as on Create, and a From email set
// to a new value must be on a verified Sending domain.
func (m *Module) Update(ctx context.Context, s *ent.Scoped, id int64, f UpdateInput) (*ent.Broadcast, error) {
	if err := checkName(f.Name.Ptr()); err != nil {
		return nil, err
	}
	doc, _ := f.Doc.Get()
	c, err := emailrender.ResolveContent(ctx, m.renderer, f.Subject.Ptr(), f.Body.Ptr(), doc, ErrBodyAndDoc)
	if err != nil {
		return nil, err
	}
	if err := m.checkRefs(ctx, s, f.SegmentID.Ptr(), f.IntegrationID.Ptr()); err != nil {
		return nil, err
	}
	if from, ok := f.FromEmail.Get(); ok {
		cur, err := m.Get(ctx, s, id)
		if err != nil {
			return nil, err
		}
		if setsFrom(&from, cur.FromEmail) {
			if err := checkFrom(ctx, s, from); err != nil {
				return nil, err
			}
		}
	}
	u := s.Broadcast().Update().Where(draftOf(id)).SetNillableBody(c.Body)
	patch.ApplySet(f.Name, u.SetName)
	patch.ApplySet(f.Subject, u.SetSubject)
	patch.Apply(f.FromName, u.SetFromName, u.ClearFromName)
	patch.Apply(f.FromEmail, u.SetFromEmail, u.ClearFromEmail)
	patch.Apply(f.SegmentID, u.SetSegmentID, u.ClearSegmentID)
	patch.Apply(f.IntegrationID, u.SetIntegrationID, u.ClearIntegrationID)
	switch {
	case c.Authored != nil:
		u.SetDoc(doc).SetEditorVersion(c.Authored.EditorVersion)
	case c.Body != nil:
		u.ClearDoc().ClearEditorVersion()
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

// Archive discards a sent or failed Broadcast (ADR 0049) and returns it: it leaves the lists but
// its Recipients, report, tracking and figures stay. A draft is purged (DeleteDraft) and a
// scheduled or sending Broadcast is work in flight, so both are ErrNotArchivable. Archiving an
// archived Broadcast changes nothing.
func (m *Module) Archive(ctx context.Context, s *ent.Scoped, id int64) (*ent.Broadcast, error) {
	err := s.Broadcast().Discard(ctx, id, broadcast.StatusIn(broadcast.StatusSent, broadcast.StatusFailed))
	if err != nil && !ent.IsNotFound(err) {
		return nil, err
	}
	b, err := m.Get(ctx, s, id)
	if err != nil {
		return nil, err
	}
	if b.Status != broadcast.StatusSent && b.Status != broadcast.StatusFailed {
		return nil, ErrNotArchivable
	}
	return b, nil
}

// Unarchive brings an archived Broadcast back to the lists and returns it. Restoring a Broadcast
// that is not archived changes nothing.
func (m *Module) Unarchive(ctx context.Context, s *ent.Scoped, id int64) (*ent.Broadcast, error) {
	err := s.Broadcast().Restore(ctx, id)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return m.Get(ctx, s, id)
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
