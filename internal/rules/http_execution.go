package rules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/httpcall"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/webhook"
)

// HTTPExecution is the next decision for an HTTP Action. A zero value means the Action is
// decided or no longer exists. Retry leaves it Pending for an ordinary attempt; Wait leaves
// it Pending without spending an attempt. A final failure is recorded on the Firing.
type HTTPExecution struct {
	Retry error
	Wait  time.Duration
}

// ExecuteHTTP selects and executes the pending HTTP Action of a Firing, and records its result.
// The adapter supplies the protected HTTP client, clock and ordinary attempt information. It needs no
// queue job, and keeps the Firing's independent Actions and completion semantics.
func (m *Module) ExecuteHTTP(ctx context.Context, s *ent.Scoped, firingID int64, index int, attempt httpcall.Attempt, client webhook.Doer, now func() time.Time) (HTTPExecution, error) {
	f, err := s.RuleFiring().Get(ctx, firingID)
	if ent.IsNotFound(err) {
		return HTTPExecution{}, nil
	}
	if err != nil {
		return HTTPExecution{}, err
	}
	action, pending, err := pendingHTTP(ctx, s, f, index)
	if err != nil || !pending {
		return HTTPExecution{}, err
	}
	record := func(res Result) (HTTPExecution, error) {
		res.Type = ActionHTTP
		_, err := m.Record(ctx, s, f.ID, index, res)
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
	bindings := outbound.Bindings(c, outbound.EventVariables(f.Action, props))
	body, err := httpcall.RenderBody(action.Body, bindings)
	if err != nil {
		return record(Result{Status: Failed, Detail: "invalid_body: " + httpcall.BodyDetail(err)})
	}
	sendErr := webhook.Post(ctx, client, webhook.Call{
		URL: opened.URL, Secret: opened.Secret, Headers: opened.Headers, Body: []byte(body),
		ID: fmt.Sprintf("rule_%d_%s", f.ID, action.ID),
	})
	if sendErr == nil {
		return record(Result{Status: Succeeded})
	}
	failed := Result{Status: Failed, Detail: failureDetail(sendErr)}
	if verdict, limited := webhook.DecideRateLimited(sendErr, attempt.CreatedAt, now()); limited {
		if verdict.GiveUp {
			return record(failed)
		}
		return HTTPExecution{Wait: verdict.Wait}, nil
	}
	if !webhook.Retryable(sendErr) || attempt.Number >= attempt.Limit {
		return record(failed)
	}
	return HTTPExecution{Retry: sendErr}, nil
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
