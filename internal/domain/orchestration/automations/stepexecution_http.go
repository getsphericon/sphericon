package automations

import (
	"context"
	"fmt"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/httpcall"
	"github.com/getsphericon/sphericon/internal/infra/webhook"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
)

// ExecuteHTTPInput is one attempt at the call of an HTTP step: the Enrollment, the id of the Step
// it is expected to stand on, and the queue's attempt data.
type ExecuteHTTPInput struct {
	RunID   int64
	StepID  automationstep.ID
	Attempt httpcall.Attempt
}

// ExecuteHTTP selects the pinned Step, renders and executes its protected call under the
// Enrollment's exit fence, then advances or fails the Enrollment. The adapter supplies only the
// Workspace scope and the ordinary attempt data. An accepted request whose advancement fails can
// be repeated; external delivery is not exactly-once.
// ExecuteHTTP makes one attempt at the call of an HTTP Step and tells its job what next
// (httpcall.Execution): zero for completed or stale work. A final failure is recorded on the
// Enrollment before it returns Cancel or an exhausted Retry.
func (m *StepExecution) ExecuteHTTP(ctx context.Context, s *ent.Scoped, in ExecuteHTTPInput) (httpcall.Execution, error) {
	step, attempt := in.StepID, in.Attempt
	run, err := s.AutomationRun().Get(ctx, in.RunID)
	if ent.IsNotFound(err) {
		return httpcall.Execution{}, nil
	}
	if err != nil {
		return httpcall.Execution{}, err
	}
	// What the move publishes carries the Enrollment's chain (ADR 0127).
	ctx = events.PassingThrough(ctx, run.Chain...)
	if live, err := isActive(ctx, s, run.ID); err != nil {
		return httpcall.Execution{}, err
	} else if !live || lo.FromPtr(run.CurrentStepID) != step {
		return httpcall.Execution{}, nil
	}
	version, err := s.AutomationVersion().Get(ctx, run.VersionID)
	if err != nil {
		return httpcall.Execution{}, err
	}
	st, ok := automationstep.Find(version.Steps, step)
	if !ok {
		return httpcall.Execution{}, nil
	}
	call, ok := st.Payload.(automationstep.HTTP)
	if !ok {
		return httpcall.Execution{}, nil
	}
	c, err := s.Contact().Get(ctx, run.ContactID)
	if ent.IsNotFound(err) {
		return httpcall.Execution{}, nil
	}
	if err != nil {
		return httpcall.Execution{}, err
	}
	opened, err := openHTTP(m.cipher, call)
	if err != nil {
		return httpcall.Execution{}, err
	}
	body, err := renderBody(call.Body, c, run)
	if err != nil {
		return m.failHTTP(ctx, s, run.ID, failure{Kind: automationrun.FailureKindInvalidBody}, httpcall.Execution{Cancel: err})
	}
	// An exit committed before this fence prevents the call; an exit during it waits until
	// the call returns. Release it before the lifecycle move, which takes the row's write lock.
	sendErr := m.fence(ctx, s, run.ID, func(ctx context.Context) error {
		return webhook.Post(ctx, m.client, webhook.Call{
			URL: opened.URL, Secret: opened.Secret, Headers: opened.Headers, Body: []byte(body), At: m.now(),
			// The run and the id of the Step the Enrollment stands on, which every version keeps
			// (ADR 0122).
			ID: fmt.Sprintf("automation_%d_%s", run.ID, step),
		})
	})
	if stale(sendErr) {
		return httpcall.Execution{}, nil
	}
	if sendErr == nil {
		return httpcall.Execution{}, m.advance(ctx, s, run.ID, place{steps: version.Steps, step: step}, stepMove{})
	}
	decision := httpcall.OneOff.Decide(sendErr, attempt, m.now())
	switch {
	case decision.Outcome == httpcall.Wait:
		return httpcall.Execution{Wait: decision.Wait}, nil
	case decision.Outcome == httpcall.Retry:
		return httpcall.Execution{Retry: sendErr}, nil
	case decision.Reason == httpcall.AttemptsExhausted:
		// The last attempt fails as an ordinary one, and the queue ends it as exhausted.
		return m.failHTTP(ctx, s, run.ID, failureOfDelivery(sendErr), httpcall.Execution{Retry: sendErr})
	}
	return m.failHTTP(ctx, s, run.ID, failureOfDelivery(sendErr), httpcall.Execution{Cancel: sendErr})
}

func (m *StepExecution) failHTTP(ctx context.Context, s *ent.Scoped, runID int64, f failure, decision httpcall.Execution) (httpcall.Execution, error) {
	if err := quiet(m.failIn(ctx, s, runID, f)); err != nil {
		return httpcall.Execution{}, err
	}
	return decision, nil
}
