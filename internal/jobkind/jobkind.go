// Package jobkind names the river job kinds that hold personal data in their
// arguments. It is a leaf package so that Erasure can clear those jobs by kind
// without importing the job workers (internal/jobs), which pull in the send path.
package jobkind

// Kinds of the queued jobs whose arguments Erasure clears.
const (
	DeliverWebhook  = "deliver_webhook"
	EvaluateTrigger = "automation_evaluate_trigger"
	RunStep         = "automation_run_step"
	HTTPStep        = "automation_http_step"
	SendRecipient   = "send_broadcast_recipient"
)
