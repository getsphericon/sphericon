// Package stepping walks an Enrollment through its Automation's steps: one operation
// executes one step of one Enrollment, moves it and queues the job of the next step in the
// same transaction. The Automations module keeps the Enrollment's state and lifecycle
// (ADR 0046); it cannot hold the loop because the Tags module imports it. The module holds
// no raw database client: a job adapter loads the Enrollment's row, builds the Workspace
// scope from it (the job entry point of ADR 0017) and passes the scope in.
package stepping

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/jobkind"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/tags"
	"github.com/getsphericon/sphericon/internal/webhook"
)

// QueueHTTPSteps isolates the HTTP steps of Automations (ADR 0039): an outbound call to a
// third party can take its whole timeout and be retried for days, so it must neither hold
// the default queue's automation steps nor the Webhook deliveries' queue.
const QueueHTTPSteps = "http_steps"

// RunStepArgs is the job that executes one step of one Enrollment. Step is the position the
// job expects the Enrollment to be at: a job delivered again after its step moved the
// Enrollment finds it elsewhere and ends quietly. The args carry the run id, so Erasure can
// delete a queued step by it (ADR 0021): renaming `run_id` breaks that purge.
type RunStepArgs struct {
	RunID int64 `json:"run_id"`
	Step  int   `json:"step"`
}

func (RunStepArgs) Kind() string { return jobkind.RunStep.Kind }

// RunHTTPStepArgs is the job that makes the call of one HTTP step of one Enrollment, on a
// queue of its own. Like RunStepArgs it names the step it expects the Enrollment to be at,
// and carries the run id for Erasure.
type RunHTTPStepArgs struct {
	RunID int64 `json:"run_id"`
	Step  int   `json:"step"`
}

func (RunHTTPStepArgs) Kind() string { return jobkind.HTTPStep.Kind }

// InsertOpts puts the call on its own queue with the retry horizon of a hook. The job is unique
// by its args: an HTTP step makes no move of the Enrollment, so a step job delivered again while
// the call is in flight would queue a second one and the receiver would be called twice.
func (RunHTTPStepArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       QueueHTTPSteps,
		MaxAttempts: webhook.HookMaxAttempts(),
		UniqueOpts:  river.UniqueOpts{ByArgs: true},
	}
}

// Enroll enrolls the Contact in the Automation, once ever (ADR 0046), and queues the job of its
// first step in the same transaction: a failed queue insert leaves no Enrollment that nothing
// will move, and a retry starts over. enrolled is false, and nothing is queued, when the Contact
// was enrolled before or another writer won the race. It is the one way a trigger or a Rule
// Action starts an Enrollment.
func Enroll(ctx context.Context, bus *events.Bus, s *ent.Scoped, automationID, contactID int64) (enrolled bool, err error) {
	err = bus.WithinScopedEnqueueTx(ctx, s, func(ts *ent.Scoped, q events.Enqueuer) error {
		runID, ok, err := automations.Enroll(ctx, ts, automationID, contactID)
		if err != nil || !ok {
			return err
		}
		enrolled = true
		return q.Enqueue(ctx, RunStepArgs{RunID: runID}, nil)
	})
	return enrolled, err
}

// Module executes the steps of Enrollments.
type Module struct {
	bus   *events.Bus
	out   *outbound.Module
	fence automations.Fencer
	now   func() time.Time
}

// Option configures the module.
type Option func(*Module)

// WithClock replaces the wall clock the module reads to honour an Enrollment's resume time
// and to time a wait.
func WithClock(now func() time.Time) Option { return func(m *Module) { m.now = now } }

// New builds the module. The bus runs a move and the job that follows it in one transaction;
// out sends an email step and acts as the `system` actor for a tag step; fence orders a send
// against an exit of the Enrollment.
func New(bus *events.Bus, out *outbound.Module, fence automations.Fencer, opts ...Option) *Module {
	m := &Module{bus: bus, out: out, fence: fence, now: time.Now}
	for _, o := range opts {
		o(m)
	}
	return m
}

// Step executes step of the Enrollment runID and queues what follows it, in one transaction
// with the move. It does nothing, and returns nil, when the Enrollment can no longer take
// that step: it is erased with its Contact (ADR 0021), has left `active`, or stands elsewhere
// than step because this job was delivered twice or a competing writer moved it. A step that
// runs before the Enrollment's recorded resume time queues itself for that time and does
// nothing else. An email step is one Outbound send under the Automation's Sending source
// (ADR 0015): the module owns eligibility, the unsubscribe footer and the send record, and
// Step only turns its Outcome into the Enrollment's next state.
func (m *Module) Step(ctx context.Context, s *ent.Scoped, runID int64, step int) error {
	run, err := s.AutomationRun().Get(ctx, runID)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load run %d: %w", runID, err)
	}
	// The lifecycle table decides whether the Enrollment can still move: a step that finds it
	// exited, completed or failed stops before it sends, tags or advances.
	if active, err := automations.IsActive(ctx, s, run.ID); err != nil {
		return err
	} else if !active || run.CurrentStep != step {
		return nil
	}

	a, err := s.Automation().Get(ctx, run.AutomationID)
	if err != nil {
		return err
	}
	// The steps are the ones of the version the Enrollment began on, whatever the Automation
	// holds now (ADR 0049).
	version, err := s.AutomationVersion().Get(ctx, run.VersionID)
	if err != nil {
		return fmt.Errorf("load version %d of run %d: %w", run.VersionID, run.ID, err)
	}
	steps, err := automations.Decode(version.Steps)
	if err != nil {
		return m.fail(ctx, s, run.ID, fmt.Errorf("automation %d version %d steps: %w", a.ID, version.Version, err))
	}

	if run.ResumeAt != nil && run.ResumeAt.After(m.now()) {
		return m.requeue(ctx, s, run.ID, step, *run.ResumeAt)
	}
	if step >= len(steps) {
		return m.complete(ctx, s, run.ID)
	}

	switch st := steps[step]; st.Type {
	case automations.StepWait:
		resume := m.now().Add(time.Duration(st.Seconds) * time.Second)
		return m.advance(ctx, s, run.ID, step, move{resumeAt: &resume})

	case automations.StepApplyTag, automations.StepRemoveTag:
		// A tag step changes the Contact's Tags and moves straight on; it sends nothing. The
		// move is made first, so a step that lost a race tags nothing, and the change commits
		// with it. The change is the `system` actor's (ADR 0022): a Tag the step creates is
		// audited.
		return m.advance(ctx, s, run.ID, step, move{with: func(ts *ent.Scoped) error {
			sys := m.out.AsSystem(ts)
			if st.Type == automations.StepApplyTag {
				_, err := tags.New().Apply(ctx, sys, run.ContactID, st.Tag)
				return err
			}
			return tags.New().Remove(ctx, sys, run.ContactID, st.Tag)
		}})

	case automations.StepHTTP:
		// The call runs on its own queue (ADR 0039); the job queued here makes it and moves
		// the Enrollment on through FinishHTTPStep.
		return m.enqueue(ctx, s, RunHTTPStepArgs{RunID: run.ID, Step: step}, nil)

	case automations.StepEmail:
		return m.email(ctx, s, a, run, step, st)

	default:
		return m.fail(ctx, s, run.ID, fmt.Errorf("unknown step type %q", st.Type))
	}
}

// FinishHTTPStep moves the Enrollment past its HTTP step and queues the step after it, in one
// transaction. Nothing happens when the Enrollment no longer stands on step: an exit won while
// the call was in flight, or an earlier attempt of the call already moved it.
func (m *Module) FinishHTTPStep(ctx context.Context, s *ent.Scoped, runID int64, step int) error {
	return m.advance(ctx, s, runID, step, move{})
}

// FailHTTPStep ends the Enrollment `failed` with the record of the call that could not
// succeed. An Enrollment that already left `active` (an exit won) or is gone stays as it is.
func (m *Module) FailHTTPStep(ctx context.Context, s *ent.Scoped, runID int64, f automations.Failure) error {
	return quiet(automations.Fail(ctx, s, runID, f))
}

func (m *Module) email(ctx context.Context, s *ent.Scoped, a *ent.Automation, run *ent.AutomationRun, step int, st automations.Step) error {
	c, err := s.Contact().Get(ctx, run.ContactID)
	if err != nil {
		return err
	}
	// Email channel: a Contact with no email address can't receive this step. Not an opt-out:
	// the sequence simply completes.
	if c.Email == nil {
		return m.complete(ctx, s, run.ID)
	}
	// The send is fenced against an exit (Fencer): it starts only on an Enrollment that is
	// still active and an exit waits for it, so nothing is sent after an exit.
	var res outbound.Result
	err = m.fence(ctx, s, run.ID, func(ctx context.Context) error {
		var sendErr error
		res, sendErr = m.out.Send(ctx, s, outbound.AutomationStep(a, run, step, c, st.Subject, st.Body))
		return sendErr
	})
	if automations.Stale(err) {
		return nil
	}
	if err != nil {
		return err // retryable: the job's retry replays the same key
	}
	// A Hold or a Deferral waits, unchanged, and asks again later (an Automation step cannot
	// snooze, so it queues itself).
	if retry := res.Retry(0); retry.Wanted {
		return m.requeue(ctx, s, run.ID, step, m.now().Add(retry.After))
	}
	switch res.Outcome {
	case outbound.Sent:
		// A crash between the send and this move replays the recorded Sent on retry, so the
		// step never sends twice.
		return m.advance(ctx, s, run.ID, step, move{})
	case outbound.Skipped:
		// An ineligible destination (suppressed, or unsubscribed from this Automation or from
		// everything) exits the Enrollment: a run never silently keeps walking steps while
		// skipping every email.
		return quiet(automations.ExitRun(ctx, s, run.ID))
	default: // outbound.Failed
		return m.fail(ctx, s, run.ID, fmt.Errorf("email step %d: %s", step, res.Reason))
	}
}

// move is what an advance does besides moving the Enrollment: resumeAt, when set, parks it and
// schedules the next job for then; with makes writes of its own in the same transaction.
type move struct {
	resumeAt *time.Time
	with     func(*ent.Scoped) error
}

// advance moves the Enrollment from step to the next one and queues that step's job, in one
// transaction with mv's own writes. A move that finds the Enrollment gone or elsewhere writes
// nothing and returns nil.
func (m *Module) advance(ctx context.Context, s *ent.Scoped, runID int64, from int, mv move) error {
	err := m.bus.WithinScopedEnqueueTx(ctx, s, func(ts *ent.Scoped, q events.Enqueuer) error {
		if err := automations.Advance(ctx, ts, runID, from, from+1, mv.resumeAt); err != nil {
			return err
		}
		if mv.with != nil {
			if err := mv.with(ts); err != nil {
				return err
			}
		}
		return q.Enqueue(ctx, RunStepArgs{RunID: runID, Step: from + 1}, scheduled(mv.resumeAt))
	})
	return quiet(err)
}

// requeue queues the same step again for at.
func (m *Module) requeue(ctx context.Context, s *ent.Scoped, runID int64, step int, at time.Time) error {
	return m.enqueue(ctx, s, RunStepArgs{RunID: runID, Step: step}, &at)
}

func (m *Module) enqueue(ctx context.Context, s *ent.Scoped, args river.JobArgs, at *time.Time) error {
	return m.bus.WithinScopedEnqueueTx(ctx, s, func(_ *ent.Scoped, q events.Enqueuer) error {
		return q.Enqueue(ctx, args, scheduled(at))
	})
}

func scheduled(at *time.Time) *river.InsertOpts {
	if at == nil {
		return nil
	}
	return &river.InsertOpts{ScheduledAt: *at}
}

// complete ends the Enrollment `completed`; one that is no longer active stays as it is.
func (m *Module) complete(ctx context.Context, s *ent.Scoped, runID int64) error {
	return quiet(automations.Complete(ctx, s, runID))
}

// fail ends the Enrollment `failed` and returns cause for the job, or nothing when the run
// had already left `active`. A failed write is returned with the cause, never dropped.
func (m *Module) fail(ctx context.Context, s *ent.Scoped, runID int64, cause error) error {
	err := automations.Fail(ctx, s, runID, automations.Failure{})
	if automations.Stale(err) {
		return nil
	}
	if err != nil {
		return errors.Join(cause, fmt.Errorf("fail run %d: %w", runID, err))
	}
	return cause
}

// quiet turns a write on an Enrollment that is gone or no longer active (an exit won while the
// step ran, or the Contact was erased) into a finished step, and returns any other error as it
// is.
func quiet(err error) error {
	if automations.Stale(err) {
		return nil
	}
	return err
}
