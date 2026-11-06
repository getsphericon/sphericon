package jobs

import (
	"context"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/infra/httpcall"
	"github.com/getsphericon/sphericon/internal/infra/webhook"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/usecase/stepexecution"
)

// RunRuleHTTPArgs is one http Action of one Firing. It carries no personal data: the Firing is
// read from the database and goes with its Contact on Erasure, which leaves the job nothing to do.
type RunRuleHTTPArgs struct {
	FiringID int64 `json:"firing_id"`
	// Index is the Action's position in the Firing, so two http Actions of one Firing are two jobs.
	Index int `json:"index"`
}

func (RunRuleHTTPArgs) Kind() string { return "rule_http_action" }

// InsertOpts puts the call on the queue of the HTTP steps with the retry horizon of a hook
// (webhook.HookMaxAttempts), as an Automation's HTTP step: an Action that calls a third party can be
// retried for days and must not hold the Actions job, or the Actions after it.
func (RunRuleHTTPArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       stepexecution.QueueHTTPSteps,
		MaxAttempts: webhook.HookMaxAttempts(),
		UniqueOpts:  river.UniqueOpts{ByArgs: true},
	}
}

// httpAction is the Runner of the http Action: it hands the call to a job of its own and leaves
// the Action Pending, which the job records when the call ends.
type httpAction struct{}

func (httpAction) Run(_ context.Context, _ *ent.Scoped, in rules.ActionInput) (rules.Result, error) {
	return rules.Result{Status: rules.Pending, Job: RunRuleHTTPArgs{FiringID: in.Firing.ID, Index: in.Index}}, nil
}

// RunRuleHTTPWorker makes the call of an http Action (sealed URL, headers and secret, signed
// Standard Webhooks, the same SSRF-hardened client as an Automation's HTTP step), rendered for
// the Contact and the Event that fired the Rule, and records how it ended on the Firing.
type RunRuleHTTPWorker struct {
	river.WorkerDefaults[RunRuleHTTPArgs]

	ent    *ent.Client
	mod    *rules.Module
	client webhook.Doer
	now    func() time.Time
}

// NewRunRuleHTTPWorker builds the worker that runs a Firing's HTTP Action through doer, on the clock now.
func NewRunRuleHTTPWorker(client *ent.Client, mod *rules.Module, doer webhook.Doer, now func() time.Time) *RunRuleHTTPWorker {
	return &RunRuleHTTPWorker{ent: client, mod: mod, client: doer, now: now}
}

func (w *RunRuleHTTPWorker) Work(ctx context.Context, job *river.Job[RunRuleHTTPArgs]) error {
	// Job entry point (ADR 0017): the scope is built from the Workspace of the loaded Firing.
	f, err := w.ent.RuleFiring.Get(ctx, job.Args.FiringID)
	if ent.IsNotFound(err) {
		return nil // erased with its Contact after the job was queued
	}
	if err != nil {
		return err
	}
	s := w.ent.Scoped(f.WorkspaceID)
	decision, err := w.mod.ExecuteHTTP(ctx, s, f.ID, job.Args.Index, httpcall.Attempt{
		Number: job.Attempt, Limit: job.MaxAttempts, CreatedAt: job.CreatedAt,
	}, w.client, w.now)
	if err != nil {
		return err
	}
	if decision.Wait > 0 {
		return river.JobSnooze(decision.Wait)
	}
	return decision.Retry
}

// NextRetry is the call's retry policy (webhook.HookNextRetry): days, as for a Webhook delivery.
func (w *RunRuleHTTPWorker) NextRetry(job *river.Job[RunRuleHTTPArgs]) time.Time {
	if job.AttemptedAt == nil {
		return time.Time{} // river falls back to its default
	}
	return webhook.HookNextRetry(*job.AttemptedAt, job.Attempt)
}
