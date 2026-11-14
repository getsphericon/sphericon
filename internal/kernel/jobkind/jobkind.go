// Package jobkind names the river job kinds that hold personal data in their
// arguments, with the argument each is purged by, the field every job of a Workspace
// names its Workspace at, and the queue two modules share. It is a leaf package so that Erasure and the erase of a Workspace
// can clear those jobs without importing the job workers (internal/jobs), which pull in the
// send path. The job definitions take their kind from here.
package jobkind

import (
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// Workspace is embedded by the args of every job that works on one Workspace's rows: it names
// the Workspace at the args' top-level field WorkspaceArg, where the erase of a Workspace finds
// every job the Workspace still has, whatever its kind or queue (events.QueuePurger
// PurgeWorkspaceJobs). An event consumer's job names it at the same field through its envelope
// (events.EventJob). A job of the Instance (a periodic sweep, a Notice to a User) carries none.
type Workspace struct {
	WorkspaceID int64 `json:"workspaceId"`
}

// JobWorkspace is the Workspace the job works on (OfWorkspace).
func (w Workspace) JobWorkspace() int64 { return w.WorkspaceID }

// OfWorkspace is the args of a job of one Workspace: every args type that embeds Workspace. The
// job runner refuses to queue one whose Workspace is 0, which no erase would ever find.
type OfWorkspace interface {
	JobWorkspace() int64
}

// QueueHTTPCalls isolates the jobs that call a third party over HTTP, an Automation's HTTP step
// and a Rule's http Action (ADR 0039): such a call can take its whole timeout and be retried for
// days, so it must neither hold the default queue's automation steps nor the Webhook deliveries'
// queue. It is here because both the Automations and the Rules modules queue into it.
const QueueHTTPCalls = "http_steps"

// QueuedOnce is the uniqueness of a check queued again on a schedule (a periodic tick's fan-out,
// a button that queues the same check): one job of the args waits or runs at a time, and a job
// that finished does not block the next one, as river's default states (which keep a completed
// job) would until the job is cleaned out.
func QueuedOnce() river.UniqueOpts {
	return river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{
		rivertype.JobStateAvailable,
		rivertype.JobStatePending,
		rivertype.JobStateRetryable,
		rivertype.JobStateRunning,
		rivertype.JobStateScheduled,
	}}
}

// WorkspaceArg is the args field that names a job's Workspace (Workspace, events.Envelope).
const WorkspaceArg = "workspaceId"

// Key is a job kind with the integer argument Erasure matches its queued jobs by.
type Key struct {
	Kind string
	Arg  string
}

// Purge keys of the queued jobs whose arguments Erasure clears.
var (
	// DeliverWebhook is matched by the endpoint it delivers to; the body is matched
	// through DeliverWebhookBodyArg.
	DeliverWebhook  = Key{Kind: "deliver_webhook", Arg: "endpoint_id"}
	EvaluateTrigger = Key{Kind: "automation_evaluate_trigger", Arg: "contact_id"}
	RunStep         = Key{Kind: "automation_run_step", Arg: "run_id"}
	HTTPStep        = Key{Kind: "automation_http_step", Arg: "run_id"}
	SendRecipient   = Key{Kind: "send_broadcast_recipient", Arg: "recipient_id"}
	// RuleEvaluate carries the Event's properties, so it is matched by its Contact.
	RuleEvaluate = Key{Kind: "rule_evaluate_event", Arg: "contact_id"}
)

// DeliverWebhookBodyArg is the argument of a queued webhook delivery that holds the event
// payload (stored by river as a base64 JSON string).
const DeliverWebhookBodyArg = "body"

// Purged lists every kind Erasure purges.
var Purged = []Key{DeliverWebhook, EvaluateTrigger, RunStep, HTTPStep, SendRecipient, RuleEvaluate}
