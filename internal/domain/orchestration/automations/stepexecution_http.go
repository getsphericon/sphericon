package automations

import (
	"context"
	"fmt"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automationstep"
	"github.com/getsphericon/sphericon/internal/infra/httpcall"
	"github.com/getsphericon/sphericon/internal/infra/webhook"
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

// ExecuteHTTPInput is one attempt at the call of an HTTP step: the Enrollment, the step it is
// expected to be at, and the queue's attempt data.
type ExecuteHTTPInput struct {
	RunID   int64
	Step    int
	Attempt httpcall.Attempt
}

// ExecuteHTTP selects the pinned Step, renders and executes its protected call under the
// Enrollment's exit fence, then advances or fails the Enrollment. The adapter supplies only the
// Workspace scope and the ordinary attempt data. An accepted request whose advancement fails can
// be repeated; external delivery is not exactly-once.
func (m *StepExecution) ExecuteHTTP(ctx context.Context, s *ent.Scoped, in ExecuteHTTPInput) (HTTPExecution, error) {
	step, attempt := in.Step, in.Attempt
	run, err := s.AutomationRun().Get(ctx, in.RunID)
	if ent.IsNotFound(err) {
		return HTTPExecution{}, nil
	}
	if err != nil {
		return HTTPExecution{}, err
	}
	if live, err := isActive(ctx, s, run.ID); err != nil {
		return HTTPExecution{}, err
	} else if !live || run.CurrentStep != step {
		return HTTPExecution{}, nil
	}
	version, err := s.AutomationVersion().Get(ctx, run.VersionID)
	if err != nil {
		return HTTPExecution{}, err
	}
	steps, err := automationstep.Decode(version.Steps)
	if err != nil {
		return m.failHTTP(ctx, s, run.ID, failure{Kind: automationrun.FailureKindInvalidBody}, HTTPExecution{Cancel: err})
	}
	if step < 0 || step >= len(steps) || steps[step].Type != automationstep.HTTP {
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
	opened, err := openHTTP(m.cipher, st)
	if err != nil {
		return HTTPExecution{}, err
	}
	body, err := renderBody(st.Body, c, run)
	if err != nil {
		return m.failHTTP(ctx, s, run.ID, failure{Kind: automationrun.FailureKindInvalidBody}, HTTPExecution{Cancel: err})
	}
	// An exit committed before this fence prevents the call; an exit during it waits until
	// the call returns. Release it before the lifecycle move, which takes the row's write lock.
	sendErr := m.fence(ctx, s, run.ID, func(ctx context.Context) error {
		return webhook.Post(ctx, m.client, webhook.Call{
			URL: opened.URL, Secret: opened.Secret, Headers: opened.Headers, Body: []byte(body), At: m.now(),
			ID: fmt.Sprintf("automation_%d_%d", run.ID, step),
		})
	})
	if stale(sendErr) {
		return HTTPExecution{}, nil
	}
	if sendErr == nil {
		return HTTPExecution{}, m.advance(ctx, s, run.ID, step, stepMove{})
	}
	decision := httpcall.OneOff.Decide(sendErr, attempt, m.now())
	switch {
	case decision.Outcome == httpcall.Wait:
		return HTTPExecution{Wait: decision.Wait}, nil
	case decision.Outcome == httpcall.Retry:
		return HTTPExecution{Retry: sendErr}, nil
	case decision.Reason == httpcall.AttemptsExhausted:
		// The last attempt fails as an ordinary one, and the queue ends it as exhausted.
		return m.failHTTP(ctx, s, run.ID, failureOfDelivery(sendErr), HTTPExecution{Retry: sendErr})
	}
	return m.failHTTP(ctx, s, run.ID, failureOfDelivery(sendErr), HTTPExecution{Cancel: sendErr})
}

func (m *StepExecution) failHTTP(ctx context.Context, s *ent.Scoped, runID int64, f failure, decision HTTPExecution) (HTTPExecution, error) {
	if err := quiet(failRun(ctx, s, runID, m.now(), f)); err != nil {
		return HTTPExecution{}, err
	}
	return decision, nil
}
