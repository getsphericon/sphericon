package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/httpcall"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/stepping"
	"github.com/getsphericon/sphericon/internal/webhook"
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
		Queue:       stepping.QueueHTTPSteps,
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
	action, pending, err := rules.PendingHTTP(ctx, s, f, job.Args.Index)
	if err != nil || !pending {
		return err // an attempt of this job already recorded the call
	}
	record := func(res rules.Result) error {
		res.Type = rules.ActionHTTP
		_, err := w.mod.Record(ctx, s, f.ID, job.Args.Index, res)
		return err
	}

	c, err := s.Contact().Get(ctx, f.ContactID)
	if ent.IsNotFound(err) {
		return record(rules.Result{Status: rules.Failed, Detail: "contact not found"})
	}
	if err != nil {
		return err
	}
	opened, err := w.mod.OpenHTTP(action)
	if err != nil {
		return record(rules.Result{Status: rules.Failed, Detail: "the stored call cannot be opened"})
	}
	var props map[string]any
	if p := f.Properties; p != nil {
		if err := json.Unmarshal([]byte(*p), &props); err != nil {
			props = nil
		}
	}
	// A missing property renders as it does in an HTTP step: empty.
	bindings := outbound.Bindings(c, outbound.EventVariables(f.Action, props))
	body, err := httpcall.RenderBody(action.Body, bindings)
	if err != nil {
		// A render that is not valid JSON would fail the same way on every attempt.
		return record(rules.Result{Status: rules.Failed, Detail: "invalid_body: " + httpcall.BodyDetail(err)})
	}

	sendErr := webhook.Post(ctx, w.client, webhook.Call{
		URL: opened.URL, Secret: opened.Secret, Headers: opened.Headers, Body: []byte(body),
		ID: fmt.Sprintf("rule_%d_%s", f.ID, action.ID),
	})
	if sendErr == nil {
		return record(rules.Result{Status: rules.Succeeded})
	}

	failed := rules.Result{Status: rules.Failed, Detail: failureDetail(sendErr)}
	now := w.now()
	if limited := webhook.RateLimited(sendErr); limited != nil {
		if now.Sub(job.CreatedAt) >= webhook.HookRetryHorizon {
			return record(failed) // a receiver that never lets up
		}
		return river.JobSnooze(webhook.HookSnooze(limited.RetryAfter, now)) // never an attempt
	}
	if !webhook.Retryable(sendErr) || job.Attempt >= job.MaxAttempts {
		return record(failed) // no retry opens a blocked address or turns a 4xx into a 2xx
	}
	return sendErr
}

// NextRetry is the call's retry policy (webhook.HookNextRetry): days, as for a Webhook delivery.
func (w *RunRuleHTTPWorker) NextRetry(job *river.Job[RunRuleHTTPArgs]) time.Time {
	if job.AttemptedAt == nil {
		return time.Time{} // river falls back to its default
	}
	return webhook.HookNextRetry(*job.AttemptedAt, job.Attempt)
}

// failureDetail is what the Firing records of a failed call: the closed kind, the status and the
// first 1 KiB of the receiver's answer. It never carries the URL, which is a secret.
func failureDetail(err error) string {
	var d *webhook.DeliveryError
	if !errors.As(err, &d) {
		return string(webhook.KindNetwork)
	}
	if d.Kind != webhook.KindHTTPStatus {
		return string(d.Kind)
	}
	detail := fmt.Sprintf("%s %d", d.Kind, d.Status)
	if d.Excerpt != "" {
		detail += ": " + d.Excerpt
	}
	return detail
}
