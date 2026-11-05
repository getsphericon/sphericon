// Package jobkind names the river job kinds that hold personal data in their
// arguments, with the argument each is purged by. It is a leaf package so that Erasure
// can clear those jobs by kind without importing the job workers (internal/jobs), which
// pull in the send path. The job definitions take their kind from here.
package jobkind

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
