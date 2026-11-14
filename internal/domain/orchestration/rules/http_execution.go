package rules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/infra/httpcall"
	"github.com/getsphericon/sphericon/internal/infra/webhook"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
)

// HTTPArgs is the job of one http Action of one Firing, which the module queues when it records
// the Action Pending. It carries no personal data: the Firing is read from the database and goes
// with its Contact on Erasure, which leaves the job nothing to do.
type HTTPArgs struct {
	jobkind.Workspace

	FiringID int64 `json:"firing_id"`
	// Index is the Action's position in the Firing, so two http Actions of one Firing are two jobs.
	Index int `json:"index"`
}

func (HTTPArgs) Kind() string { return "rule_http_action" }

// InsertOpts puts the call on the queue of the HTTP calls with the retry horizon of a hook
// (webhook.HookMaxAttempts), as an Automation's HTTP step: an Action that calls a third party can be
// retried for days and must not hold the Actions job, or the Actions after it.
func (HTTPArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       jobkind.QueueHTTPCalls,
		MaxAttempts: webhook.HookMaxAttempts(),
		UniqueOpts:  river.UniqueOpts{ByArgs: true},
	}
}

// HTTPExecution is the next decision for an HTTP Action. A zero value means the Action is
// decided or no longer exists. Retry leaves it Pending for an ordinary attempt; Wait leaves
// it Pending without spending an attempt. A final failure is recorded on the Firing.
type HTTPExecution struct {
	Retry error
	Wait  time.Duration
}

// ExecuteHTTP selects and executes the pending HTTP Action of a Firing through the module's
// protected HTTP client, and records its result. The adapter supplies only the ordinary attempt
// information. It needs no queue job, and keeps the Firing's independent Actions and completion
// semantics.
func (m *Module) ExecuteHTTP(ctx context.Context, s *ent.Scoped, firingID int64, index int, attempt httpcall.Attempt) (HTTPExecution, error) {
	f, err := s.RuleFiring().Get(ctx, firingID)
	if ent.IsNotFound(err) {
		return HTTPExecution{}, nil
	}
	if err != nil {
		return HTTPExecution{}, err
	}
	actions, pending, err := pendingHTTP(ctx, s, f, index)
	if err != nil || !pending {
		return HTTPExecution{}, err
	}
	action := actions[index]
	record := func(res Result) (HTTPExecution, error) {
		res.Type = ActionHTTP
		_, err := m.record(ctx, s, f.ID, index, res, len(actions))
		return HTTPExecution{}, err
	}
	c, err := s.Contact().Get(ctx, f.ContactID)
	if ent.IsNotFound(err) {
		return record(Result{Status: Failed, Detail: "contact not found"})
	}
	if err != nil {
		return HTTPExecution{}, err
	}
	opened, err := m.OpenHTTP(action)
	if err != nil {
		return record(Result{Status: Failed, Detail: "the stored call cannot be opened"})
	}
	var props map[string]any
	if p := f.Properties; p != nil {
		if err := json.Unmarshal([]byte(*p), &props); err != nil {
			props = nil
		}
	}
	bindings := outboundmessages.Bindings(c, outboundmessages.EventVariables(f.Action, props))
	body, err := httpcall.RenderBody(action.Body, bindings)
	if err != nil {
		return record(Result{Status: Failed, Detail: "invalid_body: " + httpcall.BodyDetail(err)})
	}
	sendErr := webhook.Post(ctx, m.client, webhook.Call{
		URL: opened.URL, Secret: opened.Secret, Headers: opened.Headers, Body: []byte(body), At: m.now(),
		ID: fmt.Sprintf("rule_%d_%s", f.ID, action.ID),
	})
	if sendErr == nil {
		return record(Result{Status: Succeeded})
	}
	decision := httpcall.OneOff.Decide(sendErr, attempt, m.now())
	if decision.Outcome == httpcall.Wait {
		return HTTPExecution{Wait: decision.Wait}, nil
	}
	if decision.Outcome == httpcall.Retry {
		return HTTPExecution{Retry: sendErr}, nil
	}
	// Whatever the reason a call is given up for, its Action is Failed and the job ends.
	return record(Result{Status: Failed, Detail: failureDetail(sendErr)})
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
