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
	"errors"
	"fmt"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"

	"github.com/riverqueue/river"
	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/emaildoc"
	"github.com/getsphericon/sphericon/internal/kernel/holdreason"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/kernel/telegramrender"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

var (
	// ErrNotFound: no such Broadcast in this Workspace.
	ErrNotFound = validation.NotFound("broadcasts: broadcast not found", i18n.ErrorsBroadcastNotFound)
	// ErrNotSendable: the Broadcast is past draft/scheduled (sending, sent, failed).
	ErrNotSendable = validation.Refuse("broadcasts: broadcast is not a draft or scheduled", i18n.ErrorsBroadcastNotSendable)
	// ErrNotArchivable: a scheduled or sending Broadcast is work in flight, Live use of itself
	// (ADR 0099), so it is not archived.
	ErrNotArchivable = validation.Conflict("broadcasts: a scheduled or sending broadcast cannot be archived", i18n.ErrorsBroadcastNotArchivable)
	// ErrArchived: the Broadcast is archived, so it is read-only to its owner until it is
	// unarchived (ADR 0100).
	ErrArchived = validation.Conflict("broadcasts: broadcast is archived", i18n.ErrorsBroadcastArchived)
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
	jobkind.Workspace

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
	out      *outboundmessages.Module
}

// New builds the module. The bus runs a claim and its send job in one transaction; the renderer
// builds a body from its document; out is Outbound send, whose Send route Schedule asks whether the
// Broadcast can send and the progress estimate paces by.
func New(bus *events.Bus, renderer *emaildoc.Renderer, out *outboundmessages.Module) *Module {
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
// outboundmessages.ErrRouteHeld and stays as it was; a route that stops sending after it holds the plan.
func (m *Module) Schedule(ctx context.Context, s *ent.Scoped, id int64, when time.Time) (*ent.Broadcast, error) {
	return m.claim(ctx, s, id, triggerSchedule, func(u *ent.BroadcastScopedUpdate) { u.SetScheduledAt(when) }, &when, m.armable)
}

// armable refuses a claimed Broadcast that cannot go out as written: its Send route cannot send, or
// its content breaks its Channel's written-length rule.
func (m *Module) armable(ctx context.Context, ts *ent.Scoped, b *ent.Broadcast) error {
	channel := b.Channel
	route, err := m.out.Route(ctx, ts, channel, b.IntegrationID)
	if err != nil {
		return err
	}
	if err := route.Armable(); err != nil {
		return err
	}
	return checkWritten(b)
}

// checkWritten is the Channel's written-length rule (ADR 0078), checked at Schedule on the content
// as written. On Telegram it is telegramrender.CheckWritten, the rule an Automation's Publish applies
// to a Telegram step, so a text is over its limit (1024 with an image, 4096 without) on both or on
// neither, and a message with neither a text nor an image has nothing to send. What a text renders
// to may still be longer or shorter; a message over the limit once rendered fails its own send. On
// SMS a blank text has nothing to send.
func checkWritten(b *ent.Broadcast) error {
	switch b.Channel {
	case channels.SMS:
		if strings.TrimSpace(lo.FromPtr(b.Text)) == "" {
			return ErrTextBlank
		}
		return nil
	case channels.Email:
		return nil
	case channels.Telegram:
	}
	text, hasImage := lo.FromPtr(b.Text), lo.FromPtr(b.ImageURL) != ""
	if strings.TrimSpace(text) == "" && !hasImage {
		return ErrTextTooLong.With("the message has no text and no image")
	}
	if err := telegramrender.CheckWritten(text, hasImage); err != nil {
		return ErrTextTooLong.With(err.Error())
	}
	return nil
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
// Broadcast after the move, which rolls the move back. An archived Broadcast is read-only and is
// ErrArchived. A claimed Broadcast is Live use, so the claim
// holds the Workspace's lock (ADR 0099): a removal that checks for Live use waits for it, or it for
// the removal, and the one that waits reads what the other committed.
func (m *Module) claim(ctx context.Context, s *ent.Scoped, id int64, t trigger, set update, at *time.Time, check func(context.Context, *ent.Scoped, *ent.Broadcast) error) (*ent.Broadcast, error) {
	var claimed *ent.Broadcast
	err := m.bus.WithinWorkspaceLockEnqueueTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher, q events.Enqueuer) error {
		if err := m.notArchived(ctx, ts, id); err != nil {
			return err
		}
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
		return q.Enqueue(ctx, SendArgs{WorkspaceID: ts.WorkspaceID(), BroadcastID: id, ScheduledAt: at}, opts)
	})
	if err != nil {
		return nil, err
	}
	return claimed, nil
}

// --- Authoring (ADR 0016): drafts, Segment, report ---

var (
	// ErrNotDraft: the Broadcast is past draft (scheduled, sending, sent, failed) and
	// can no longer be edited or re-targeted through the authoring surface.
	ErrNotDraft = validation.Refuse("broadcasts: broadcast is not a draft", i18n.ErrorsBroadcastNotDraft)
	// ErrSegmentNotFound: the Segment does not exist in this Workspace.
	ErrSegmentNotFound = validation.Refuse("broadcasts: segment not found", i18n.ErrorsBroadcastSegmentNotFound)
	// ErrSegmentArchived: a draft leaving draft, or a write that sets the Segment, names an archived
	// Segment, which takes no part in new work (ADR 0100); the refusal names it.
	ErrSegmentArchived = validation.Refuse("broadcasts: segment archived", i18n.ErrorsBroadcastSegmentArchived)
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
	// ErrNoChannel: a new Broadcast names no Channel: it has no content to take the shape of.
	ErrNoChannel = validation.Refuse("broadcasts: a broadcast needs a channel", i18n.ErrorsBroadcastNoChannel)
	// ErrChannelFixed: an edit carries content of another Channel than the Broadcast's, which is
	// fixed at creation: a Broadcast is never converted to another Channel (ADR 0080).
	ErrChannelFixed = validation.Refuse("broadcasts: the channel of a broadcast cannot change", i18n.ErrorsBroadcastChannelFixed)
	// ErrTextTooLong: the message of a Telegram Broadcast is empty or its text is over what Telegram takes,
	// so it is not scheduled (ADR 0078). Not a field refusal: Schedule and Send take no content, so
	// there is no request field to list it under; the editor counts the limit as the author types.
	ErrTextTooLong = validation.Refuse("broadcasts: the text is too long for Telegram", i18n.ErrorsBroadcastTextTooLong)
	// ErrTextBlank: the text of an SMS Broadcast is blank, so it is not scheduled: every recipient's
	// message would fail with nothing to send.
	ErrTextBlank = validation.Refuse("broadcasts: the text is blank", i18n.ErrorsBroadcastTextBlank)
	// ErrChannelNoIntegration: an SMS Broadcast is created in a Workspace with no SMS Integration,
	// which nothing could send it through (ADR 0080). An email draft needs none (the System sender
	// takes its Test send, ADR 0041), and a Telegram one is refused only at Schedule.
	ErrChannelNoIntegration = validation.Refuse("broadcasts: the workspace has no integration of the channel", i18n.ErrorsBroadcastChannelNoIntegration)
	// ErrIntegrationOtherChannel: the Integration is of another Channel than the Broadcast's, so
	// it can never send it (ADR 0074).
	ErrIntegrationOtherChannel = validation.Refuse("broadcasts: the integration is of another channel", i18n.ErrorsBroadcastIntegrationOtherChannel)
)

// CreateInput is a new draft Broadcast. A nil field is "not provided": it takes the schema
// default. Content is required: its Channel is the Broadcast's, fixed from then on (ADR 0080).
// SegmentID and IntegrationID reference rows of the same Workspace; the scoped client refuses a
// foreign one with ent.ErrNotInWorkspace, and the Integration must be of the Broadcast's Channel.
type CreateInput struct {
	Name          *string
	Content       ContentInput
	SegmentID     *int64
	IntegrationID *int64
}

// UpdateInput is a patch of a draft Broadcast: a field that keeps leaves the stored value, a clear
// drops it. Name cannot be cleared; SegmentID clears to all contacts and IntegrationID to the
// Channel's default. A nil Content keeps the content; one of another Channel is ErrChannelFixed.
type UpdateInput struct {
	Name          patch.Field[string]
	Content       ContentPatch
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

// Create makes a draft Broadcast on the Channel of its content. Nothing here schedules or sends.
// A blank name is ErrBlankName, no content ErrNoChannel, an SMS Broadcast in a Workspace with no
// SMS Integration ErrChannelNoIntegration and an Integration of another Channel
// ErrIntegrationOtherChannel; content that cannot render is emailrender.ErrInvalidContent, and an
// email body is stored sanitized.
func (m *Module) Create(ctx context.Context, s *ent.Scoped, f CreateInput) (*ent.Broadcast, error) {
	if err := checkName(f.Name); err != nil {
		return nil, err
	}
	if f.Content == nil {
		return nil, ErrNoChannel
	}
	channel := f.Content.Channel()
	if channel == channels.SMS {
		ok, err := messaging.HasIntegrationOn(ctx, s, channel)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrChannelNoIntegration
		}
	}
	if err := m.checkRefs(ctx, s, channel, f.SegmentID, f.IntegrationID); err != nil {
		return nil, err
	}
	q := s.Broadcast().Create().
		SetChannel(channelOf(channel)).
		SetNillableSegmentID(f.SegmentID).
		SetNillableIntegrationID(f.IntegrationID)
	if f.Name != nil {
		q.SetName(*f.Name)
	}
	if err := f.Content.create(ctx, m, s, q); err != nil {
		return nil, err
	}
	return q.Save(ctx)
}

// checkRefs refuses a Segment or Integration that is missing or archived: an archived row leaves
// new work (ADR 0049). A Broadcast already pointing at one keeps it as history. An Integration of
// another Channel than the Broadcast's never sends it (ADR 0074) and is refused too.
func (m *Module) checkRefs(ctx context.Context, s *ent.Scoped, channel channels.Channel, segmentID, integrationID *int64) error {
	if segmentID != nil {
		if _, err := keptSegment(ctx, s, *segmentID); err != nil {
			return err
		}
	}
	if integrationID == nil {
		return nil
	}
	kept, err := messaging.IntegrationKept(ctx, s, *integrationID)
	if err != nil {
		return err
	}
	if !kept {
		return ErrIntegrationNotFound
	}
	onChannel, err := messaging.KeptIntegrationOn(ctx, s, channel, *integrationID)
	if err != nil {
		return err
	}
	if !onChannel {
		return ErrIntegrationOtherChannel
	}
	return nil
}

// keptSegment returns the Segment new work of a Broadcast takes (a write that sets it, a draft that
// is armed), refusing one that is missing from the Workspace (ErrSegmentNotFound) or archived
// (ErrSegmentArchived, naming it): an archived Segment leaves new work (ADR 0100). A Broadcast
// already pointing at one keeps it as history.
func keptSegment(ctx context.Context, s *ent.Scoped, id int64) (*ent.Segment, error) {
	seg, err := s.Segment().Get(ctx, id)
	if ent.IsNotFound(err) {
		return nil, ErrSegmentNotFound
	}
	if err != nil {
		return nil, err
	}
	if seg.ArchivedAt != nil {
		return nil, ErrSegmentArchived.With(seg.Name)
	}
	return seg, nil
}

// Get returns one Broadcast of the Workspace.
func (m *Module) Get(ctx context.Context, s *ent.Scoped, id int64) (*ent.Broadcast, error) {
	b, err := s.Broadcast().Get(ctx, id)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	return b, err
}

// ListFilter narrows the Broadcast list: the archived ones instead of the kept ones, only some
// statuses (every one when empty), only one Channel (every one when nil).
type ListFilter struct {
	Archived bool
	Statuses []broadcast.Status
	Channel  *channels.Channel
}

// List returns one page of the Workspace's Broadcasts that f lets through, in order.
func (m *Module) List(ctx context.Context, s *ent.Scoped, p pagination.Params, f ListFilter, order Order) (pagination.Page[*ent.Broadcast], error) {
	rows := func() *ent.BroadcastQuery {
		q := s.Broadcast().Query()
		if f.Archived {
			q = s.Broadcast().QueryArchived()
		}
		if len(f.Statuses) > 0 {
			q = q.Where(broadcast.StatusIn(f.Statuses...))
		}
		if f.Channel != nil {
			q = q.Where(broadcast.ChannelEQ(*f.Channel))
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
// ErrBlankName; a Segment or Integration set here is checked as on Create; content of another
// Channel than the Broadcast's is ErrChannelFixed, since a Broadcast is never converted (ADR 0080).
func (m *Module) Update(ctx context.Context, s *ent.Scoped, id int64, f UpdateInput) (*ent.Broadcast, error) {
	if err := checkName(f.Name.Ptr()); err != nil {
		return nil, err
	}
	cur, err := m.Get(ctx, s, id)
	if err != nil {
		return nil, err
	}
	if err := refuseArchived(cur); err != nil {
		return nil, err
	}
	channel := cur.Channel
	if f.Content != nil && f.Content.Channel() != channel {
		return nil, ErrChannelFixed
	}
	if err := m.checkRefs(ctx, s, channel, f.SegmentID.Ptr(), f.IntegrationID.Ptr()); err != nil {
		return nil, err
	}
	u := s.Broadcast().Update().Where(draftOf(id))
	if f.Content != nil {
		if err := f.Content.update(ctx, m, s, cur, u); err != nil {
			return nil, err
		}
	}
	patch.ApplySet(f.Name, u.SetName)
	patch.Apply(f.SegmentID, u.SetSegmentID, u.ClearSegmentID)
	patch.Apply(f.IntegrationID, u.SetIntegrationID, u.ClearIntegrationID)
	return m.editDraft(ctx, s, id, u)
}

// SetSegment points a draft at a Segment, or at all contacts when segmentID
// is nil. It never schedules or sends.
func (m *Module) SetSegment(ctx context.Context, s *ent.Scoped, id int64, segmentID *int64) (*ent.Broadcast, error) {
	if segmentID != nil {
		if _, err := keptSegment(ctx, s, *segmentID); err != nil {
			// A foreign or unknown broadcast is still reported as such first.
			if _, getErr := m.Get(ctx, s, id); getErr != nil {
				return nil, getErr
			}
			return nil, err
		}
	}
	u := s.Broadcast().Update().Where(draftOf(id)).SetNillableSegmentID(segmentID)
	if segmentID == nil {
		u.ClearSegmentID()
	}
	return m.editDraft(ctx, s, id, u)
}

// archivable is the statuses a Broadcast is archived in: a draft and a finished one. A scheduled
// or sending Broadcast is work in flight, Live use of itself (ADR 0099).
var archivable = []broadcast.Status{broadcast.StatusDraft, broadcast.StatusSent, broadcast.StatusFailed}

// Archive puts a draft, sent or failed Broadcast aside (ADR 0100) and returns it: it leaves the
// lists for the archived view, its Recipients, report, tracking and figures stay, and it is
// read-only until unarchived. A Broadcast is never deleted. A scheduled or sending one is
// ErrNotArchivable. Archiving an archived Broadcast changes nothing. It holds the Workspace's
// lock, as a claim does, and the claim refuses an archived draft, so an Archive and a Schedule
// that race cannot leave an archived Broadcast scheduled.
func (m *Module) Archive(ctx context.Context, s *ent.Scoped, id int64) (*ent.Broadcast, error) {
	var archived *ent.Broadcast
	err := m.bus.WithinWorkspaceLockTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		err := ts.Broadcast().Archive(ctx, id, m.out.Now(), broadcast.StatusIn(archivable...))
		if err != nil && !ent.IsNotFound(err) {
			return err
		}
		got, err := m.Get(ctx, ts, id)
		if err != nil {
			return err
		}
		archived = got
		if !lo.Contains(archivable, archived.Status) {
			return ErrNotArchivable
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return archived, nil
}

// Unarchive brings an archived Broadcast back to the lists, in the status it had, and returns it.
// Unarchiving a Broadcast that is not archived changes nothing.
func (m *Module) Unarchive(ctx context.Context, s *ent.Scoped, id int64) (*ent.Broadcast, error) {
	err := s.Broadcast().Unarchive(ctx, id)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return m.Get(ctx, s, id)
}

// Report is a Broadcast's delivery report: its status, why it is held and its Figures, the Stats
// as its Channel reports them. The rates are ratios in [0,1] where a zero denominator yields 0.
type Report struct {
	Figures

	Status     broadcast.Status
	HoldReason *holdreason.Reason
}

// Report reads the Broadcast's delivery figures (see Stats).
func (m *Module) Report(ctx context.Context, s *ent.Scoped, id int64) (Report, error) {
	b, err := m.Get(ctx, s, id)
	if err != nil {
		return Report{}, err
	}
	figures, err := StatsFor(ctx, s, b)
	if err != nil {
		return Report{}, err
	}
	return Report{Status: b.Status, HoldReason: b.HoldReason, Figures: figures[id].Figures()}, nil
}

// draftOf matches the Broadcast while it is a draft its owner may edit: not archived.
func draftOf(id int64) predicate.Broadcast {
	return broadcast.And(broadcast.ID(id), broadcast.StatusEQ(broadcast.StatusDraft), broadcast.ArchivedAtIsNil())
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
	if err := m.notArchived(ctx, s, id); err != nil {
		return err
	}
	return ErrNotDraft
}

// notArchived is ErrArchived for an archived Broadcast and ErrNotFound for one the Workspace does
// not have.
func (m *Module) notArchived(ctx context.Context, s *ent.Scoped, id int64) error {
	b, err := m.Get(ctx, s, id)
	if err != nil {
		return err
	}
	return refuseArchived(b)
}

// refuseArchived is ErrArchived for an archived Broadcast: it is read-only to its owner until it
// is unarchived (ADR 0100).
func refuseArchived(b *ent.Broadcast) error {
	if b.ArchivedAt != nil {
		return ErrArchived
	}
	return nil
}
