// Package broadcasts owns the Broadcast send/schedule state machine (ADR 0016):
// draft -> scheduled -> sending, clearing a schedule, and reverting to draft when
// the job cannot be enqueued. Every surface that sends or schedules a Broadcast
// (the /site SPA today, /api and MCP next) calls it; none re-implements a
// transition. Handlers stay thin adapters: scope check, call, map errors.
//
// What happens after "sending" (recipients, delivery, sent/failed) is the job
// worker's business, via Outbound send (ADR 0015).
package broadcasts

import (
	"context"
	"errors"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/segment"
	"github.com/getsphericon/sphericon/internal/emailrender"
	"github.com/getsphericon/sphericon/internal/pagination"
)

var (
	// ErrNotFound: no such Broadcast in this Workspace.
	ErrNotFound = errors.New("broadcasts: broadcast not found")
	// ErrNotSendable: the Broadcast is past draft/scheduled (sending, sent, failed).
	ErrNotSendable = errors.New("broadcasts: broadcast is not a draft or scheduled")
	// ErrNotScheduled: the Broadcast is not scheduled, so there is no schedule to clear.
	ErrNotScheduled = errors.New("broadcasts: broadcast is not scheduled")
)

// Enqueuer hands a Broadcast to the job queue (river in prod, inline in tests). A nil
// scheduledAt means "as soon as possible".
type Enqueuer interface {
	EnqueueBroadcast(ctx context.Context, broadcastID int64, scheduledAt *time.Time) error
}

// Module is the Broadcast send/schedule state machine. It holds no client: every
// call receives the Workspace-scoped client (ADR 0017) built by the entry point, so
// the module writes no Workspace predicate and cannot name another Workspace.
type Module struct {
	q Enqueuer
}

func New(q Enqueuer) *Module { return &Module{q: q} }

// Send moves a draft or scheduled Broadcast to sending (dropping any schedule) and
// enqueues it for immediate dispatch. If the enqueue fails the Broadcast reverts to
// draft, so it is never stranded in sending with no job behind it.
func (m *Module) Send(ctx context.Context, s *ent.Scoped, id int64) (*ent.Broadcast, error) {
	b, err := m.claim(ctx, s, id, func(u *ent.BroadcastScopedUpdate) *ent.BroadcastScopedUpdate {
		return u.SetStatus(broadcast.StatusSending).ClearScheduledAt()
	})
	if err != nil {
		return nil, err
	}
	// Status is written BEFORE the enqueue and never after: the inline adapter runs
	// the send synchronously and advances the row to "sent"; a later write would
	// clobber it.
	if err := m.q.EnqueueBroadcast(ctx, b.ID, nil); err != nil {
		m.revert(ctx, s, b.ID)
		return nil, err
	}
	return b, nil
}

// Schedule moves a draft or scheduled Broadcast to scheduled at `when` and enqueues
// a delayed job. If the enqueue fails the Broadcast reverts to draft with no schedule.
func (m *Module) Schedule(ctx context.Context, s *ent.Scoped, id int64, when time.Time) (*ent.Broadcast, error) {
	b, err := m.claim(ctx, s, id, func(u *ent.BroadcastScopedUpdate) *ent.BroadcastScopedUpdate {
		return u.SetStatus(broadcast.StatusScheduled).SetScheduledAt(when)
	})
	if err != nil {
		return nil, err
	}
	if err := m.q.EnqueueBroadcast(ctx, b.ID, &when); err != nil {
		m.revert(ctx, s, b.ID)
		return nil, err
	}
	return b, nil
}

// Unschedule returns a scheduled Broadcast to draft and drops its schedule. The
// delayed job stays queued; when it fires it finds a draft and does nothing.
func (m *Module) Unschedule(ctx context.Context, s *ent.Scoped, id int64) (*ent.Broadcast, error) {
	n, err := s.Broadcast().Update().
		Where(broadcast.ID(id), broadcast.StatusEQ(broadcast.StatusScheduled)).
		SetStatus(broadcast.StatusDraft).ClearScheduledAt().Save(ctx)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		exists, err := s.Broadcast().Query().Where(broadcast.ID(id)).Exist(ctx)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, ErrNotFound
		}
		return nil, ErrNotScheduled
	}
	return s.Broadcast().Get(ctx, id)
}

// claim applies a transition atomically: the UPDATE only matches a draft or
// scheduled row in this Workspace, so two racing requests cannot both win.
func (m *Module) claim(ctx context.Context, s *ent.Scoped, id int64, set func(*ent.BroadcastScopedUpdate) *ent.BroadcastScopedUpdate) (*ent.Broadcast, error) {
	n, err := set(s.Broadcast().Update().Where(
		broadcast.ID(id),
		broadcast.StatusIn(broadcast.StatusDraft, broadcast.StatusScheduled),
	)).Save(ctx)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		exists, err := s.Broadcast().Query().Where(broadcast.ID(id)).Exist(ctx)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, ErrNotFound
		}
		return nil, ErrNotSendable
	}
	return s.Broadcast().Get(ctx, id)
}

// revert is best effort: the caller is already returning the enqueue error.
func (m *Module) revert(ctx context.Context, s *ent.Scoped, id int64) {
	_ = s.Broadcast().UpdateOneID(id).SetStatus(broadcast.StatusDraft).ClearScheduledAt().Exec(ctx)
}

// --- Authoring (ADR 0016): drafts, audience, report ---

var (
	// ErrNotDraft: the Broadcast is past draft (scheduled, sending, sent, failed) and
	// can no longer be edited, re-targeted or deleted through the authoring surface.
	ErrNotDraft = errors.New("broadcasts: broadcast is not a draft")
	// ErrSegmentNotFound: the audience Segment does not exist in this Workspace.
	ErrSegmentNotFound = errors.New("broadcasts: segment not found")
)

// Fields are the author-editable content of a Broadcast. A nil field is "not
// provided": Create falls back to the schema default, Update keeps the stored value.
// The Clear flags are JSON Merge Patch nulls for Update: they drop the stored value.
// SegmentID and IntegrationID reference rows of the same Workspace; the scoped
// client refuses a foreign one with ent.ErrNotInWorkspace.
type Fields struct {
	Name          *string
	Subject       *string
	FromName      *string
	FromEmail     *string
	Body          *string
	SegmentID     *int64
	IntegrationID *int64

	ClearFromName    bool
	ClearFromEmail   bool
	ClearSegment     bool
	ClearIntegration bool
}

// Create makes a draft Broadcast. Nothing here schedules or sends. A subject or body that
// cannot render is emailrender.ErrInvalidContent; the body is stored sanitized.
func (m *Module) Create(ctx context.Context, s *ent.Scoped, f Fields) (*ent.Broadcast, error) {
	body, err := emailrender.PrepareFields(f.Subject, f.Body)
	if err != nil {
		return nil, err
	}
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
	return q.Save(ctx)
}

// Get returns one Broadcast of the Workspace.
func (m *Module) Get(ctx context.Context, s *ent.Scoped, id int64) (*ent.Broadcast, error) {
	b, err := s.Broadcast().Query().Where(broadcast.ID(id)).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	return b, err
}

// List returns one page of the Workspace's Broadcasts, newest first.
func (m *Module) List(ctx context.Context, s *ent.Scoped, p pagination.Params) (pagination.Page[*ent.Broadcast], error) {
	return pagination.List(ctx, p,
		func(ctx context.Context) (int, error) { return s.Broadcast().Query().Count(ctx) },
		func(ctx context.Context, limit, offset int) ([]*ent.Broadcast, error) {
			return s.Broadcast().Query().Order(ent.Desc(broadcast.FieldID)).Limit(limit).Offset(offset).All(ctx)
		})
}

// Update edits a draft. The UPDATE only matches a draft row, so a Broadcast that was
// claimed for sending in the meantime is never edited.
func (m *Module) Update(ctx context.Context, s *ent.Scoped, id int64, f Fields) (*ent.Broadcast, error) {
	body, err := emailrender.PrepareFields(f.Subject, f.Body)
	if err != nil {
		return nil, err
	}
	u := s.Broadcast().Update().Where(draftOf(id)).
		SetNillableName(f.Name).
		SetNillableSubject(f.Subject).
		SetNillableFromName(f.FromName).
		SetNillableFromEmail(f.FromEmail).
		SetNillableBody(body).
		SetNillableSegmentID(f.SegmentID).
		SetNillableIntegrationID(f.IntegrationID)
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

// SetAudience points a draft at a Segment, or at all active contacts when segmentID
// is nil. It never schedules or sends.
func (m *Module) SetAudience(ctx context.Context, s *ent.Scoped, id int64, segmentID *int64) (*ent.Broadcast, error) {
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

// DeleteDraft removes a draft Broadcast. Past draft the Broadcast is history (its
// recipients and report) and is not removable here.
func (m *Module) DeleteDraft(ctx context.Context, s *ent.Scoped, id int64) error {
	n, err := s.Broadcast().Delete().Where(draftOf(id)).Exec(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return m.notDraftOrNotFound(ctx, s, id)
	}
	return nil
}

// Delete removes a Broadcast of any status together with its recipient rows, which
// FK the Broadcast (the engagement Event log keys on subject_id and is untouched).
// It is the human's history cleanup in the SPA; the authoring surface (/api, MCP)
// uses DeleteDraft and cannot reach a sent Broadcast.
func (m *Module) Delete(ctx context.Context, s *ent.Scoped, id int64) error {
	if _, err := s.BroadcastRecipient().Delete().Where(broadcastrecipient.BroadcastID(id)).Exec(ctx); err != nil {
		return err
	}
	err := s.Broadcast().DeleteOneID(id).Exec(ctx)
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	return err
}

// Report is a Broadcast's delivery report. Rates are ratios in [0,1]; a zero
// denominator yields 0.
type Report struct {
	Status       broadcast.Status
	HoldReason   *string
	Recipients   int
	Sent         int
	Skipped      int
	Failed       int
	Opened       int
	Clicked      int
	Unsubscribed int
	OpenRate     float32
	ClickRate    float32
}

// Report reads the Broadcast's denormalized delivery counters.
func (m *Module) Report(ctx context.Context, s *ent.Scoped, id int64) (Report, error) {
	b, err := m.Get(ctx, s, id)
	if err != nil {
		return Report{}, err
	}
	return Report{
		Status: b.Status, HoldReason: b.HoldReason,
		Recipients: b.RecipientsTotal, Sent: b.SentCount, Skipped: b.SkippedCount, Failed: b.FailedCount,
		Opened: b.OpenedCount, Clicked: b.ClickedCount, Unsubscribed: b.UnsubscribedCount,
		OpenRate: ratio(b.OpenedCount, b.SentCount), ClickRate: ratio(b.ClickedCount, b.SentCount),
	}, nil
}

func ratio(num, denom int) float32 {
	if denom <= 0 {
		return 0
	}
	return float32(num) / float32(denom)
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
