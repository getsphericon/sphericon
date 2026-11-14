package jobs

import (
	"context"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/infra/webhook"
)

// RunRuleHTTPWorker makes the call of an http Action (sealed URL, headers and secret, signed
// Standard Webhooks, the same SSRF-hardened client as an Automation's HTTP step), rendered for
// the Contact and the Event that fired the Rule, and records how it ended on the Firing.
type RunRuleHTTPWorker struct {
	hookRetries[rules.HTTPArgs]

	ent *ent.Client
	mod *rules.Module
}

// NewRunRuleHTTPWorker builds the worker that runs a Firing's HTTP Action through the Rules
// module, which holds the protected HTTP client and the Instance clock, spreading its retries by
// jitter.
func NewRunRuleHTTPWorker(client *ent.Client, mod *rules.Module, jitter webhook.HookJitter) *RunRuleHTTPWorker {
	return &RunRuleHTTPWorker{jitter: jitter, ent: client, mod: mod}
}

func (w *RunRuleHTTPWorker) Work(ctx context.Context, job *river.Job[rules.HTTPArgs]) error {
	// Job entry point (ADR 0017): the scope is built from the Workspace of the loaded Firing.
	f, err := w.ent.RuleFiring.Get(ctx, job.Args.FiringID)
	if ent.IsNotFound(err) {
		return nil // erased with its Contact after the job was queued
	}
	if err != nil {
		return err
	}
	s := w.ent.Scoped(f.WorkspaceID)
	decision, err := w.mod.ExecuteHTTP(ctx, s, f.ID, job.Args.Index, hookAttempt(job.JobRow))
	if err != nil {
		return err
	}
	return executionResult(decision)
}
