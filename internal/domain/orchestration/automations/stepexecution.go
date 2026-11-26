package automations

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/riverqueue/river"
	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationskippedentry"
	"github.com/getsphericon/sphericon/internal/domain/contacts/condition"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contactlock"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/webhook"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/datetrigger"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
)

// RunStepArgs is the job that executes one step of one Enrollment. StepID is the id of the Step
// the job expects the Enrollment to stand on (ADR 0122): a job delivered again after its step moved
// the Enrollment finds it elsewhere and ends quietly. The args carry the run id, so Erasure can
// delete a queued step by it (ADR 0021): renaming `run_id` breaks that purge.
type RunStepArgs struct {
	jobkind.Workspace

	RunID  int64             `json:"run_id"`
	StepID automationstep.ID `json:"step_id"`
}

func (RunStepArgs) Kind() string { return jobkind.RunStep.Kind }

// RunHTTPStepArgs is the job that makes the call of one HTTP step of one Enrollment, on a
// queue of its own. Like RunStepArgs it names the step it expects the Enrollment to be at,
// and carries the run id for Erasure.
type RunHTTPStepArgs struct {
	jobkind.Workspace

	RunID  int64             `json:"run_id"`
	StepID automationstep.ID `json:"step_id"`
}

func (RunHTTPStepArgs) Kind() string { return jobkind.HTTPStep.Kind }

// InsertOpts puts the call on its own queue with the retry horizon of a hook. The job is unique
// by its args: an HTTP step makes no move of the Enrollment, so a step job delivered again while
// the call is in flight would queue a second one and the receiver would be called twice.
func (RunHTTPStepArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       jobkind.QueueHTTPCalls,
		MaxAttempts: webhook.HookMaxAttempts(),
		UniqueOpts:  river.UniqueOpts{ByArgs: true},
	}
}

// StepExecution walks Enrollments through their Automation's steps (ADR 0112): one operation
// executes one step of one Enrollment, moves it and queues the job of the next step in the same
// transaction, and the Enrollments an Event or a Rule starts are made with the job of their first
// step. It is this module's, beside the Enrollment's lifecycle it moves, and composes only modules
// below orchestration: a tag step's change of the Contact's Tags is the Contacts module's
// (contacts.Module.ApplyTagIn, RemoveTagIn) and a send step is one Outbound send. It holds no raw database
// client: a job adapter loads the Enrollment's row, builds the Workspace scope from it (the job
// entry point of ADR 0017) and passes the scope in.
type StepExecution struct {
	bus    *events.Bus
	out    *outboundmessages.Module
	fence  Fencer
	cipher *secrets.Cipher
	client webhook.Doer
	now    func() time.Time
	// contacts changes a Contact's Tags for a tag step, in the transaction of the step's move.
	contacts *contacts.Module
	// locks takes the Contact's row lock that orders its Enrollments (lockContact).
	locks *contactlock.Module
	// firings reads which Event the Firing whose Enroll Action made an Enrollment fired on.
	firings *rules.Firings
	// conditions reads a Branch's Conditions for the enrolled Contact (branch).
	conditions *condition.Module
}

// NewStepExecution builds the Step execution. The bus runs a move and the job that follows it in
// one transaction; out sends an email step and acts as the `system` actor for a tag step; people
// is the contacts module, which changes a Contact's Tags for a tag step and publishes in the
// transaction of the bus the move runs in, so it is built over the same bus; fence orders a send
// against an exit of the Enrollment; cipher opens an HTTP step's sealed secrets and client makes
// its call; now is the Instance clock, read to honour an Enrollment's resume time and to time a
// wait.
func NewStepExecution(bus *events.Bus, out *outboundmessages.Module, people *contacts.Module, fence Fencer, cipher *secrets.Cipher, client webhook.Doer, now func() time.Time) *StepExecution {
	return &StepExecution{bus: bus, out: out, contacts: people, fence: fence, cipher: cipher, client: client, now: now,
		locks: contactlock.New(), firings: rules.NewFirings(), conditions: condition.New(now)}
}

// OnEventInput is an Event of a Contact handed to the Automations: the Contact and the Event.
type OnEventInput struct {
	ContactID int64
	Event     Event
}

// EnrollOnRuleInput is a Rule's Action that enrolls a Contact: the Automation, the Contact, the
// Firing whose Action it is, the source id of the Event the Rule fired on, and the Automations the
// chain of that Event passed through (events.Envelope.Automations, ADR 0127), which the entry
// skips as `cycle` when they name the Automation.
type EnrollOnRuleInput struct {
	AutomationID int64
	ContactID    int64
	FiringID     int64
	SourceID     string
	Chain        []int64
}

// OnEvent is what an Event of the Contact does to the Automations (#751): it first ends the
// Contact's active Enrollments whose version has an exit or goal event matching it
// (endOnEvent, in a transaction of its own, which it needs for the Contact's lock),
// then enrolls the Contact into every active Automation whose live version's Trigger is the action
// (ADR 0065), one transaction per Automation (enrollOnTrigger), again after an earlier Enrollment
// ended when the version's Entry policy is `again` (ADR 0127). Each entry runs the entry checks,
// which skip one whose Event's chain already passed through the Automation (Event.Chain, `cycle`):
// its own Tag or lifecycle Event never enrolls the Contact again. An open or click of the
// Automation's own message carries no chain: it runs the same checks, and one active Enrollment at
// a time and the Entry policy bound it (ADR 0053 as amended, ADR 0127). The Event's OccurredAt is
// when it occurred, which
// an Event without one does not say, so it is refused. Every Enrollment it makes copies the stored
// Event's action and properties, read once (originOf, #752). An Event a write through a Connection
// without mcp:send published (Event.SendLocked) ends Enrollments as any other, and enrolls no one:
// it starts nothing that sends (ADR 0132), and the lifecycle Events of the endings it makes
// (automation.exited, automation.goal_reached) carry its mark, so they start nothing either.
func (m *StepExecution) OnEvent(ctx context.Context, s *ent.Scoped, in OnEventInput) error {
	contactID, ev := in.ContactID, in.Event
	if ev.OccurredAt.IsZero() {
		return errEventWithoutTime
	}
	if ev.SendLocked {
		// The lifecycle Events its endings publish carry the mark on: they enroll no one either.
		ctx = events.SendLocked(ctx)
	}
	err := m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		return m.endOnEvent(ctx, ts, pub, contactID, ev)
	})
	if err != nil || ev.SendLocked {
		return err
	}
	triggered, err := triggeredBy(ctx, s, ev)
	if err != nil {
		return err
	}
	if len(triggered) == 0 {
		return nil
	}
	o, err := originOf(ctx, s, ev)
	if err != nil {
		return err
	}
	for _, automationID := range triggered {
		if _, err := m.startOnTrigger(ctx, s, automationID, contactID, o); err != nil {
			return err
		}
	}
	return nil
}

// EnrollOnRuleResult is what a Rule's Enroll Action made: whether an Enrollment was made and, when
// the entry checks refused it, why (the reason its skipped entry records). Neither is set for a
// Firing run again whose Enrollment is there already.
type EnrollOnRuleResult struct {
	Enrolled bool
	Skipped  automationskippedentry.Reason
}

// SkipDetail says why the Action made no Enrollment, as the Rule's Firing records it: the reason the
// entry checks recorded, or none for a Firing whose Enrollment is there already.
func (r EnrollOnRuleResult) SkipDetail() string {
	if r.Skipped == "" {
		return "the contact is already enrolled by this firing"
	}
	return "the entry was skipped: " + string(r.Skipped)
}

// EnrollOnRule enrolls the Contact in the Automation for a Rule's Action that fired on the Event
// stored with in.SourceID, by the entry checks and the Entry policy of the live version as a Trigger
// does (ADR 0127), and queues the job of its first step in the same transaction: a failed queue
// insert leaves no Enrollment that nothing will move, and a retry starts over. It reports no
// Enrollment, and queues nothing, when the entry checks refused it (with their reason) or the Firing
// made one already. An Enrollment an exit or goal Event recorded since that Event ended at once is
// made, with no step queued (enrollOnRule).
func (m *StepExecution) EnrollOnRule(ctx context.Context, s *ent.Scoped, in EnrollOnRuleInput) (EnrollOnRuleResult, error) {
	return m.start(ctx, s, func(ts *ent.Scoped, pub events.Publisher) (entered, error) {
		return m.enrollOnRule(ctx, ts, pub, in, m.now())
	})
}

// startOnTrigger is enrollOnRule for the Event of the origin (originOf): it enrolls
// nobody unless, when the Enrollment is made, the Automation is still active and started by that
// Event's action in its live version (enrollOnTrigger).
func (m *StepExecution) startOnTrigger(ctx context.Context, s *ent.Scoped, automationID, contactID int64, o origin) (EnrollOnRuleResult, error) {
	return m.start(ctx, s, func(ts *ent.Scoped, pub events.Publisher) (entered, error) {
		return m.enrollOnTrigger(ctx, ts, pub, automationID, contactID, o)
	})
}

// start makes the Enrollment makeEnrollment makes and, when a step is to run, queues the job of its first
// step, the one the Enrollment begins on, in one transaction: the transaction the Contact's lock is
// held in until both commit.
func (m *StepExecution) start(ctx context.Context, s *ent.Scoped, makeEnrollment func(ts *ent.Scoped, pub events.Publisher) (entered, error)) (EnrollOnRuleResult, error) {
	var made EnrollOnRuleResult
	err := m.bus.WithinScopedEnqueueTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher, q events.Enqueuer) error {
		e, err := makeEnrollment(ts, pub)
		made = EnrollOnRuleResult{Skipped: e.skipped}
		if err != nil || e.runID == 0 {
			return err
		}
		made.Enrolled = true
		if !e.stepping {
			return nil
		}
		run, err := ts.AutomationRun().Get(ctx, e.runID)
		if err != nil {
			return err
		}
		// A first Step that is a wait is due later: its job runs then.
		return q.Enqueue(ctx, RunStepArgs{WorkspaceID: ts.WorkspaceID(), RunID: e.runID, StepID: lo.FromPtr(run.CurrentStepID)}, scheduled(run.ResumeAt))
	})
	if err != nil {
		return EnrollOnRuleResult{}, err
	}
	return made, nil
}

// Step executes the Step of the id step of the Enrollment runID and queues what follows it, in one
// transaction with the move. It does nothing, and returns nil, when the Enrollment can no longer
// take that step: it is erased with its Contact (ADR 0021), has left `active`, or stands elsewhere
// than step because this job was delivered twice or a competing writer moved it. A wait holds the
// Enrollment on itself until the due moment it recorded when the Enrollment reached it (ADR
// 0124): its job run before that moment queues itself for then and does nothing else, and at that
// moment it moves the Enrollment on. An End completes it at once. A Step id its version does not
// hold fails it. A send step is one Outbound send under the Automation's Sending source
// (ADR 0015): the Outbound module owns eligibility, the unsubscribe footer and the send record, and
// Step only turns its Outcome into the Enrollment's next state. Whatever the Step publishes carries
// the Enrollment's chain (ADR 0127).
func (m *StepExecution) Step(ctx context.Context, s *ent.Scoped, runID int64, step automationstep.ID) error {
	run, err := s.AutomationRun().Get(ctx, runID)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load run %d: %w", runID, err)
	}
	// What the Step publishes is published on behalf of the Enrollment: it carries the chain the
	// Enrollment keeps, however long it waited (ADR 0127).
	ctx = events.PassingThrough(ctx, run.Chain...)
	// The lifecycle table decides whether the Enrollment can still move: a step that finds it
	// exited, completed or failed stops before it sends, tags or advances.
	if live, err := isActive(ctx, s, run.ID); err != nil {
		return err
	} else if !live || lo.FromPtr(run.CurrentStepID) != step {
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
	// The tree is indexed once, as it is decoded: where each Step stands and what follows it.
	index := automationstep.NewIndex(version.Steps)
	st, ok := index.Step(step)
	if !ok {
		return m.fail(ctx, s, run.ID, fmt.Errorf("step %s is not a step of version %d", step, version.ID))
	}
	at := place{index: index, step: step}

	switch p := st.Payload.(type) {
	case automationstep.Wait, automationstep.WaitUntil:
		if run.ResumeAt != nil && run.ResumeAt.After(m.now()) {
			return m.requeue(ctx, s, run.ID, step, *run.ResumeAt)
		}
		// Its due moment, recorded when the Enrollment reached it, has come; or a Wait until had
		// none to wait for, which its record says why, and goes on at once.
		return m.advance(ctx, s, run.ID, at, stepMove{})

	case automationstep.ApplyTag:
		return m.tag(ctx, s, run, at, func(sys *ent.Scoped) error {
			_, err := m.contacts.ApplyTagIn(ctx, sys, run.ContactID, p.Tag)
			return err
		})

	case automationstep.RemoveTag:
		return m.tag(ctx, s, run, at, func(sys *ent.Scoped) error {
			return m.contacts.RemoveTagIn(ctx, sys, run.ContactID, p.Tag)
		})

	case automationstep.End:
		// An End finishes the path and completes the Enrollment at once (ADR 0124): whatever
		// its path holds after it is never reached.
		return m.end(ctx, s, run.ID, at)

	case automationstep.Branch:
		return m.branch(ctx, s, run, at, p)

	case automationstep.Split:
		// The draw holds no random state (ADR 0124): the same Enrollment always lands on the same
		// path, and the move down it commits behind the compare-and-set on the cursor, so a job
		// delivered again finds the Enrollment moved on.
		draw := automationstep.Draw(run.ContactID, at.step, splitRound(run))
		return m.advance(ctx, s, run.ID, at, stepMove{path: &pathTaken{id: p.At(draw)}})

	case automationstep.HTTP:
		// The call runs on its own queue (ADR 0039); the job queued here makes it and moves
		// the Enrollment on through ExecuteHTTP.
		return m.enqueue(ctx, s, RunHTTPStepArgs{WorkspaceID: s.WorkspaceID(), RunID: run.ID, StepID: step}, nil)

	default:
		if _, sends := st.Kind().Channel(); sends {
			// The send names its Step by the id the Enrollment stands on (ADR 0122): the send
			// reads no position.
			return m.send(ctx, s, a, run, at, SendStep{Step: *st})
		}
		return m.fail(ctx, s, run.ID, fmt.Errorf("unknown step type %q", st.Kind()))
	}
}

// splitRound is the round the Enrollment's Contact draws in at a Split: the anniversary year of an
// Enrollment a yearly date Trigger made, so a holdout one year draws again the next (ADR 0126), and
// the Enrollment's ordinal otherwise (ADR 0127), a once date's included.
func splitRound(run *ent.AutomationRun) int {
	if run.Occurrence == nil {
		return run.Ordinal
	}
	switch o := run.Occurrence.Occurrence().(type) {
	case datetrigger.Yearly:
		return o.Year
	case datetrigger.Once:
	}
	return run.Ordinal
}

// branch decides a Branch for the Enrollment (ADR 0124): every path's Condition is read for its
// Contact, on its live data now, in one read, the Enrollment's `event` copy given to the property
// tests (ADR 0180), and the first path whose Condition is not false decides: true takes it,
// unknown takes the unknown path, and none takes otherwise. The move down the path, the Branch's
// record of the path and of every path's outcome, and the reach of the Step it leads to commit in
// one transaction behind the compare-and-set on the cursor, so the path is decided once: a job
// delivered again finds the Enrollment moved on. A Condition that no longer reads (a Custom field
// retyped past it) fails the Enrollment; a failed read is retried.
func (m *StepExecution) branch(ctx context.Context, s *ent.Scoped, run *ent.AutomationRun, at place, b automationstep.Branch) error {
	truths, err := m.conditions.Outcomes(ctx, s, condition.OutcomesInput{ContactID: run.ContactID, Conditions: b.Conditions(), Properties: run.EventProperties})
	if errors.Is(err, condition.ErrStorage) {
		return err
	}
	if err != nil {
		return m.fail(ctx, s, run.ID, fmt.Errorf("branch step %s: %w", at.step, err))
	}
	return m.advance(ctx, s, run.ID, at, stepMove{path: chosen(b, truths)})
}

// chosen is the path of the Branch its paths' truths send the Enrollment down (ADR 0124), with each
// listed path's truth: the first whose truth is not false decides, true taking it and unknown the
// unknown path; none takes otherwise. Publish refuses a Branch whose Conditions can be unknown
// without an unknown path, so a version that runs has one wherever a truth is unknown; were it
// missing, otherwise is taken.
func chosen(b automationstep.Branch, truths []conditiontree.Truth) *pathTaken {
	taken := &pathTaken{id: b.Otherwise.ID, outcomes: make(map[automationstep.ID]conditiontree.Truth, len(b.Paths))}
	decided := false
	for i, p := range b.Paths {
		truth := truths[i]
		taken.outcomes[p.ID] = truth
		if decided || truth == conditiontree.TruthFalse {
			continue
		}
		decided = true
		switch {
		case truth == conditiontree.TruthTrue:
			taken.id = p.ID
		case b.Unknown != nil:
			taken.id = b.Unknown.ID
		}
	}
	return taken
}

// tag runs a tag step: change changes the Contact's Tags, and the Enrollment moves straight on;
// it sends nothing. It writes the Contact's row, so it locks the Contact before it moves the
// Enrollment, in the order an exit takes them (lockContact, #811). The move comes before the
// change, so a step that lost a race tags nothing, and the change commits with it. The change is
// the `system` actor's (ADR 0022): a Tag the step creates is audited.
func (m *StepExecution) tag(ctx context.Context, s *ent.Scoped, run *ent.AutomationRun, at place, change func(sys *ent.Scoped) error) error {
	return m.advance(ctx, s, run.ID, at, stepMove{contactID: run.ContactID, with: func(ts *ent.Scoped, _ events.Publisher) error {
		return change(m.out.AsSystem(ts))
	}})
}

// send executes a send step: one Outbound send of the step's content on its Channel, through the
// Integration the step names (ADR 0081). Outbound send owns eligibility, the unsubscribe and the
// send record; send only turns its Outcome into the Enrollment's next state. A Hold or a Deferral
// asks again later at the same step, so a held Integration stops only the Enrollments standing on
// the steps that send through it; a Deferral records on the Enrollment since when, why and until
// when the send waits (wait, ADR 0178), and the send carries since when and why to its message. A Skipped send moves on to the next step, the skip recorded on
// the step's Outbound message for its report, whatever its reason: an unsubscribe from this
// Automation too, which exited the Enrollment active when it arrived and skips the sends of a later
// one to the Destination that refused it, while its other Steps run (ADR 0081, ADR 0127). A Failed
// send fails the Enrollment, except one the provider refused for that message alone
// (Result.MessageOnly), which moves on too.
func (m *StepExecution) send(ctx context.Context, s *ent.Scoped, a *ent.Automation, run *ent.AutomationRun, at place, st SendStep) error {
	content, err := st.Content()
	if err != nil {
		return m.fail(ctx, s, run.ID, err)
	}
	c, err := s.Contact().Get(ctx, run.ContactID)
	if err != nil {
		return err
	}
	// The send is fenced against an exit (Fencer): it starts only on an Enrollment that is
	// still active and an exit waits for it, so nothing is sent after an exit.
	var res outboundmessages.Result
	err = m.fence(ctx, s, run.ID, func(ctx context.Context) error {
		var sendErr error
		res, sendErr = m.out.Send(ctx, s, outboundmessages.AutomationStep(a, run, at.step, st.Step.Integration(), c, content, waitedOf(run)))
		return sendErr
	})
	if stale(err) {
		return nil
	}
	if err != nil {
		return err // retryable: the job's retry replays the same key
	}
	// A Hold or a Deferral waits and asks again later (an Automation step cannot snooze, so it
	// queues itself). A Deferral records its wait on the Enrollment (ADR 0178).
	now := m.now()
	if retry := res.Retry(0, now); retry.Wanted {
		if res.Outcome == outboundmessages.Deferral {
			return m.wait(ctx, s, run, at.step, deferral{since: lo.FromPtr(run.DeferredSince), cause: res.Cause, until: now.Add(retry.After)}, now)
		}
		return m.requeue(ctx, s, run.ID, at.step, now.Add(retry.After))
	}
	switch res.Outcome {
	case outboundmessages.Sent:
		// A crash between the send and this move replays the recorded Sent on retry, so the
		// step never sends twice.
		return m.advance(ctx, s, run.ID, at, stepMove{})
	case outboundmessages.Skipped:
		// No Destination on the step's Channel, an opt-out from the Channel or from this
		// Automation, or a Suppression: the sequence goes on with its next step, so one Channel's
		// absence never ends it, and the step's record says why it was skipped.
		return m.advance(ctx, s, run.ID, at, stepMove{skipped: res.SkippedReason})
	default: // outboundmessages.Failed
		if res.MessageOnly() {
			// The provider refused this one message (an invalid number, an answer that concerns no
			// one else): it is recorded Failed, and the sequence goes on (ADR 0079).
			return m.advance(ctx, s, run.ID, at, stepMove{})
		}
		return m.fail(ctx, s, run.ID, fmt.Errorf("%s step %s: %s", st.Step.Kind(), at.step, res.Failure))
	}
}

// waitedOf is how the send of the Step the Enrollment stands on has waited so far: none until its
// first Deferral there.
func waitedOf(run *ent.AutomationRun) outboundmessages.Waited {
	if run.DeferredSince == nil {
		return outboundmessages.Waited{}
	}
	return outboundmessages.Waited{Since: *run.DeferredSince, Cause: lo.FromPtr(run.DeferralCause)}
}

// wait records that the send of the Step of the id step is deferred, as d says, and queues the
// step again for d.until, in one transaction: the first Deferral at the Step (d.since zero) waits
// since now, a later one keeps its moment. An Enrollment that moved on or ended meanwhile records
// nothing and queues nothing.
func (m *StepExecution) wait(ctx context.Context, s *ent.Scoped, run *ent.AutomationRun, step automationstep.ID, d deferral, now time.Time) error {
	if d.since.IsZero() {
		d.since = now
	}
	err := m.bus.WithinScopedEnqueueTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher, q events.Enqueuer) error {
		if err := deferRun(ctx, ts, run.ID, step, d); err != nil {
			return err
		}
		return q.Enqueue(ctx, RunStepArgs{WorkspaceID: ts.WorkspaceID(), RunID: run.ID, StepID: step}, scheduled(&d.until))
	})
	return quiet(err)
}

// stepMove is what an advance does besides moving the Enrollment: leaving is how the Enrollment
// leaves the Step, which its record keeps (skipped, or down a fork's path, which is where the move
// goes); with makes writes of its own in the same transaction. contactID, set when with writes the
// Contact's row, is that Contact: its lock is taken before the Enrollment's (the lock order of
// lockContact).
type stepMove struct {
	leaving

	contactID int64
	with      func(*ent.Scoped, events.Publisher) error
}

// place is the Step an Enrollment stands on: its id, and the index of the tree of the
// Enrollment's version that holds it.
type place struct {
	index *automationstep.Index
	step  automationstep.ID
}

// advance moves the Enrollment off the Step it stands on, in one transaction with mv's own writes
// and the Step records of the move (ADR 0124): onto the next Step, which it reaches then, queueing
// the job that runs it (at the due moment of a wait, which holds the Enrollment on itself until
// then), or, with no next Step, to its end: it completes, on the Step it left, and nothing is
// queued. The next Step is the one after the Step left (Index.Next) or, down a fork's path, the
// path's first Step or, for an empty path, the Step after the fork (Index.Enter). A move that finds
// the Enrollment gone or elsewhere, or its Contact gone, writes nothing and returns nil.
func (m *StepExecution) advance(ctx context.Context, s *ent.Scoped, runID int64, from place, mv stepMove) error {
	next, more := from.index.Next(from.step)
	if mv.path != nil {
		next, more = from.index.Enter(mv.path.id)
	}
	now := m.now()
	err := m.bus.WithinScopedEnqueueTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher, q events.Enqueuer) error {
		if mv.contactID != 0 {
			_, locked, err := m.lockContact(ctx, ts, mv.contactID)
			if err != nil || !locked {
				return err // an erased Contact's Enrollment is erased with it
			}
		}
		var arrived automationstep.Due
		if more {
			var err error
			arrived, err = arrive(next, now, func() (automationstep.Contact, error) { return runCalendar(ctx, ts, runID) })
			if err != nil {
				return err
			}
			if err := advanceRun(ctx, ts, runID, from.step, next.ID, dueAt(arrived)); err != nil {
				return err
			}
			if err := leave(ctx, ts, runID, from.step, now, mv.leaving); err != nil {
				return err
			}
		} else if err := completeOn(ctx, ts, pub, runID, from.step, now, mv.leaving); err != nil {
			return err
		}
		if mv.with != nil {
			if err := mv.with(ts, pub); err != nil {
				return err
			}
		}
		if !more {
			return nil
		}
		if err := reach(ctx, ts, runID, next.ID, now, arrived); err != nil {
			return err
		}
		return q.Enqueue(ctx, RunStepArgs{WorkspaceID: ts.WorkspaceID(), RunID: runID, StepID: next.ID}, scheduled(dueAt(arrived)))
	})
	return quiet(err)
}

// end completes the Enrollment on the End it stands on (ADR 0124), the End's record stamped in
// the same transaction; one that is no longer active or no longer there stays as it is.
func (m *StepExecution) end(ctx context.Context, s *ent.Scoped, runID int64, at place) error {
	now := m.now()
	return quiet(m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		return completeOn(ctx, ts, pub, runID, at.step, now, leaving{})
	}))
}

// completeOn completes the Enrollment on the Step of the id, which it leaves at now as how says, in
// the transaction s is bound to.
func completeOn(ctx context.Context, s *ent.Scoped, pub events.Publisher, runID int64, step automationstep.ID, now time.Time, how leaving) error {
	if err := completeRun(ctx, s, pub, runID, step, now); err != nil {
		return err
	}
	return leave(ctx, s, runID, step, now, how)
}

// requeue queues the same step again for at.
func (m *StepExecution) requeue(ctx context.Context, s *ent.Scoped, runID int64, step automationstep.ID, at time.Time) error {
	return m.enqueue(ctx, s, RunStepArgs{WorkspaceID: s.WorkspaceID(), RunID: runID, StepID: step}, &at)
}

func (m *StepExecution) enqueue(ctx context.Context, s *ent.Scoped, args river.JobArgs, at *time.Time) error {
	return m.bus.WithinScopedEnqueueTx(ctx, s, func(_ *ent.Scoped, _ events.Publisher, q events.Enqueuer) error {
		return q.Enqueue(ctx, args, scheduled(at))
	})
}

func scheduled(at *time.Time) *river.InsertOpts {
	if at == nil {
		return nil
	}
	return &river.InsertOpts{ScheduledAt: *at}
}

// fail ends the Enrollment `failed` and returns cause for the job, or nothing when the run
// had already left `active`. A failed write is returned with the cause, never dropped. The end and
// the leaving of the Step it stood on commit in one transaction.
func (m *StepExecution) fail(ctx context.Context, s *ent.Scoped, runID int64, cause error) error {
	err := m.failIn(ctx, s, runID, failure{})
	if stale(err) {
		return nil
	}
	if err != nil {
		return errors.Join(cause, fmt.Errorf("fail run %d: %w", runID, err))
	}
	return cause
}

// failIn ends the Enrollment `failed` with f in a transaction of its own, so the end and the
// leaving of the Step it stood on commit together (moved).
func (m *StepExecution) failIn(ctx context.Context, s *ent.Scoped, runID int64, f failure) error {
	now := m.now()
	return m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		return failRun(ctx, ts, runID, now, f)
	})
}

// quiet turns a write on an Enrollment that is gone or no longer active (an exit won while the
// step ran, or the Contact was erased) into a finished step, and returns any other error as it
// is.
func quiet(err error) error {
	if stale(err) {
		return nil
	}
	return err
}
