// Package jobkind names the river job kinds that hold personal data in their
// arguments, with the argument each is purged by, and the field every job of a Workspace
// names its Workspace at. It is a leaf package so that Erasure and the erase of a Workspace
// can clear those jobs without importing the job workers (internal/jobs), which pull in the
// send path. The job definitions take their kind from here.
package jobkind

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
