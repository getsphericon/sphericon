package stepexecution

import (
	"context"
	"fmt"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/automationsteps"
	"github.com/getsphericon/sphericon/internal/infra/httpcall"
	"github.com/getsphericon/sphericon/internal/infra/webhook"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
)

// HTTPExecution is the queue-neutral decision of an HTTP Step. Its fields are mutually
// exclusive: Retry asks for an ordinary attempt, Cancel ends the job after a permanent failure,
// and Wait asks again without spending an attempt. Zero means completed or stale work. A final
// failure is recorded on the Enrollment before returning Cancel or an exhausted Retry.
type HTTPExecution struct {
	Retry  error
	Cancel error
	Wait   time.Duration
}

// ExecuteHTTP selects the pinned Step, renders and executes its protected call under the
// Enrollment's exit fence, then advances or fails the Enrollment. The adapter supplies only the
// Workspace scope, ordinary attempt data, clock and existing secret and HTTP dependencies. An
// accepted request whose advancement fails can be repeated; external delivery is not exactly-once.
func (m *UseCase) ExecuteHTTP(ctx context.Context, s *ent.Scoped, runID int64, step int, attempt httpcall.Attempt, cipher *secrets.Cipher, client webhook.Doer, now func() time.Time) (HTTPExecution, error) {
	run, err := s.AutomationRun().Get(ctx, runID)
	if ent.IsNotFound(err) {
		return HTTPExecution{}, nil
	}
	if err != nil {
		return HTTPExecution{}, err
	}
	if active, err := automations.IsActive(ctx, s, run.ID); err != nil {
		return HTTPExecution{}, err
	} else if !active || run.CurrentStep != step {
		return HTTPExecution{}, nil
	}
	version, err := s.AutomationVersion().Get(ctx, run.VersionID)
	if err != nil {
		return HTTPExecution{}, err
	}
	steps, err := automationsteps.Decode(version.Steps)
	if err != nil {
		return m.failHTTP(ctx, s, run.ID, automations.Failure{Kind: automationrun.FailureKindInvalidBody}, HTTPExecution{Cancel: err})
	}
	if step < 0 || step >= len(steps) || steps[step].Type != automationsteps.HTTP {
		return HTTPExecution{}, nil
	}
	st := steps[step]
	c, err := s.Contact().Get(ctx, run.ContactID)
	if ent.IsNotFound(err) {
		return HTTPExecution{}, nil
	}
	if err != nil {
		return HTTPExecution{}, err
	}
	opened, err := automations.OpenHTTP(cipher, st)
	if err != nil {
		return HTTPExecution{}, err
	}
	body, err := automations.RenderBody(st.Body, c, run)
	if err != nil {
		return m.failHTTP(ctx, s, run.ID, automations.Failure{Kind: automationrun.FailureKindInvalidBody}, HTTPExecution{Cancel: err})
	}
	// An exit committed before this fence prevents the call; an exit during it waits until
	// the call returns. Release it before the lifecycle move, which takes the row's write lock.
	sendErr := m.fence(ctx, s, run.ID, func(ctx context.Context) error {
		return webhook.Post(ctx, client, webhook.Call{
			URL: opened.URL, Secret: opened.Secret, Headers: opened.Headers, Body: []byte(body),
			ID: fmt.Sprintf("automation_%d_%d", run.ID, step),
		})
	})
	if automations.Stale(sendErr) {
		return HTTPExecution{}, nil
	}
	if sendErr == nil {
		return HTTPExecution{}, m.advance(ctx, s, run.ID, step, move{})
	}
	f := automations.FailureOfDelivery(sendErr)
	if verdict, limited := webhook.DecideRateLimited(sendErr, attempt.CreatedAt, now()); limited {
		if verdict.GiveUp {
			return m.failHTTP(ctx, s, run.ID, f, HTTPExecution{Cancel: sendErr})
		}
		return HTTPExecution{Wait: verdict.Wait}, nil
	}
	if !webhook.Retryable(sendErr) {
		return m.failHTTP(ctx, s, run.ID, f, HTTPExecution{Cancel: sendErr})
	}
	if attempt.Number >= attempt.Limit {
		return m.failHTTP(ctx, s, run.ID, f, HTTPExecution{Retry: sendErr})
	}
	return HTTPExecution{Retry: sendErr}, nil
}

func (m *UseCase) failHTTP(ctx context.Context, s *ent.Scoped, runID int64, f automations.Failure, decision HTTPExecution) (HTTPExecution, error) {
	if err := quiet(automations.Fail(ctx, s, runID, m.now(), f)); err != nil {
		return HTTPExecution{}, err
	}
	return decision, nil
}
