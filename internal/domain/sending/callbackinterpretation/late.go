package callbackinterpretation

import (
	"context"
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/sending/preset"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
)

// A Delivery notice can reach the hook before the message it is about is committed: Twilio posts a
// text's final status as soon as the carrier answers, which can be before the send's transaction
// records the SID, and Twilio does not post it again. Such a notice is not lost: it is deferred to
// a job and read again shortly, a few times, and dropped once no message has turned up.

// LateNoticeDelays are the waits before each new reading of a notice that matched no message: the
// first is after the first reading, and once they are spent the notice is dropped.
var LateNoticeDelays = []time.Duration{5 * time.Second, 30 * time.Second, 2 * time.Minute}

// LateNoticeArgs is a Delivery notice read again later, because it matched no message when it came.
// The notice keeps what finds and settles the message (its provider id, kind, status and error
// code) and drops where it went: a job row is not erased with a Contact, so it holds no number.
type LateNoticeArgs struct {
	jobkind.Workspace

	IntegrationID int64                 `json:"integrationId"`
	Preset        string                `json:"preset"`
	Notice        preset.DeliveryNotice `json:"notice"`
	// Attempt is how many times the notice has been read: 1 after the hook's reading.
	Attempt int `json:"attempt"`
}

// Kind identifies the job.
func (LateNoticeArgs) Kind() string { return "ingest_late_notice" }

// InsertOpts retries a reading that fails like any job; a notice that still matches nothing queues
// the next reading itself.
func (LateNoticeArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{MaxAttempts: 5} }

// deferNotice queues n to be read again after the wait of the next attempt, in the request's
// transaction, so a rolled-back request queues nothing. Once the attempts are spent it is dropped
// and logged.
func (tx Tx) deferNotice(ctx context.Context, n preset.DeliveryNotice) error {
	if n.MessageID == "" {
		return nil // nothing would ever find its message
	}
	if tx.attempt >= len(LateNoticeDelays) {
		slog.InfoContext(ctx, "ingest: a delivery notice matched no message and is dropped",
			"integration_id", tx.Integration.ID, "event_id", n.ID, "attempts", tx.attempt+1)
		return nil
	}
	n.Destination, n.Reason = "", ""
	return tx.queue.Enqueue(ctx, LateNoticeArgs{
		WorkspaceID:   tx.Integration.WorkspaceID,
		IntegrationID: tx.Integration.ID, Preset: tx.Preset, Notice: n, Attempt: tx.attempt + 1,
	}, &river.InsertOpts{ScheduledAt: tx.Now.Add(LateNoticeDelays[tx.attempt])})
}

// InterpretLate reads a deferred notice again, as Interpret read it when it came, in s, the
// Workspace of row, its Integration. One that still matches no message is deferred again, until the
// attempts are spent.
func (m *Module) InterpretLate(ctx context.Context, s *ent.Scoped, row *ent.Integration, args LateNoticeArgs) error {
	return m.interpret(ctx, s, row, args.Preset, preset.Result{Notices: []preset.DeliveryNotice{args.Notice}}, m.now(), args.Attempt)
}
