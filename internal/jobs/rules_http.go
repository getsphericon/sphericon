package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/httpcall"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/webhook"
)

// httpActionAttempts is how many times an http Action tries its call: a Rule's Actions run at
// once (ADR 0054), so a flaky receiver gets a short, bounded retry in this job rather than the
// days an Automation's HTTP step may take. The last failure is recorded on the Firing.
const httpActionAttempts = 3

// httpActionDelay is the wait before the retry-th retry (1-based): 1s, then 4s.
func httpActionDelay(retry int) time.Duration { return time.Duration(retry*retry) * time.Second }

// httpAction is the Runner of the http Action: the call of an Automation's HTTP step (sealed
// URL, headers and secret, signed Standard Webhooks, the same SSRF-hardened client), rendered for
// the Contact and the Event that fired the Rule.
type httpAction struct {
	mod    *rules.Module
	client webhook.Doer
	// wait pauses between attempts; a test replaces it.
	wait func(context.Context, time.Duration) error
}

func (a httpAction) Run(ctx context.Context, s *ent.Scoped, in rules.ActionInput) (rules.Result, error) {
	c, err := s.Contact().Get(ctx, in.ContactID)
	if ent.IsNotFound(err) {
		return rules.Result{Status: rules.Failed, Detail: "contact not found"}, nil
	}
	if err != nil {
		return rules.Result{}, err
	}
	opened, err := a.mod.OpenHTTP(in.Action)
	if err != nil {
		return rules.Result{Status: rules.Failed, Detail: "the stored call cannot be opened"}, nil
	}
	var props map[string]any
	if p := in.Firing.Properties; p != nil {
		if err := json.Unmarshal([]byte(*p), &props); err != nil {
			props = nil
		}
	}
	// A missing property renders as it does in an HTTP step: empty.
	bindings := outbound.Bindings(c, map[string]any{"event": map[string]any{
		"action":     in.Firing.Action,
		"properties": props,
	}})
	body, err := httpcall.RenderBody(in.Action.Body, bindings)
	if err != nil {
		return rules.Result{Status: rules.Failed, Detail: "invalid_body: " + httpcall.BodyDetail(err)}, nil
	}
	call := webhook.Call{
		URL: opened.URL, Secret: opened.Secret, Headers: opened.Headers, Body: []byte(body),
		ID: fmt.Sprintf("rule_%d_%s", in.Firing.ID, in.Action.ID),
	}
	var sendErr error
	for attempt := 1; attempt <= httpActionAttempts; attempt++ {
		if sendErr = webhook.Post(ctx, a.client, call); sendErr == nil {
			return rules.Result{Status: rules.Succeeded}, nil
		}
		if !retryable(sendErr) || attempt == httpActionAttempts {
			break
		}
		if err := a.wait(ctx, httpActionDelay(attempt)); err != nil {
			return rules.Result{}, err
		}
	}
	return rules.Result{Status: rules.Failed, Detail: failureDetail(sendErr)}, nil
}

// retryable is a failure another try can fix: a timeout, a transport error, a 429 or a 5xx. A
// blocked address or any other status would answer the same.
func retryable(err error) bool {
	var d *webhook.DeliveryError
	if !errors.As(err, &d) {
		return true
	}
	switch d.Kind {
	case webhook.KindTimeout, webhook.KindNetwork:
		return true
	case webhook.KindHTTPStatus:
		return d.Status == 429 || d.Status >= 500
	case webhook.KindBlockedAddress:
		return false
	}
	return false
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

func pause(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
