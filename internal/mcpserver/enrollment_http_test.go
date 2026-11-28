package mcpserver_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/httpcall"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// answering is a hook that answers every call with a 200 and body.
type answering struct{ body string }

func (a answering) Do(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(a.body))}, nil
}

// An agent reads the response an HTTP step kept, under its name, its body wrapped as untrusted (ADR
// 0161): the hook's owner wrote it.
func TestMCPEnrollmentsMarkTheKeptResponseUntrusted(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](env)
	steps := automationstep.Root(
		automationstep.New(automationstep.HTTP{Name: "coupon", URL: "https://hooks.example.com/coupons", Body: "{}"}),
		automationstep.New(automationstep.Wait{Seconds: 3600}),
	)
	a, err := m.Create(ctx, s, automations.CreateInput{
		Name: "Your coupon", Trigger: automations.EventTrigger{Matcher: eventmatcher.Matcher{Action: "trial.started"}}, Steps: &steps,
	})
	require.NoError(t, err)
	_, err = m.Publish(ctx, s, a.ID, testhelper.AutomationRevisionIn(ctx, t, s, a.ID))
	require.NoError(t, err)
	handAlice(t, env, "trial.started", nil)
	runID, err := env.DB.AutomationRun.Query().Where(automationrun.AutomationID(a.ID)).OnlyID(ctx)
	require.NoError(t, err)
	body, err := json.Marshal(map[string]any{"note": injection})
	require.NoError(t, err)
	unfenced := func(ctx context.Context, _ *ent.Scoped, _ int64, effect func(context.Context) error) error {
		return effect(ctx)
	}
	stepper := automations.NewStepExecution(env.Bus, env.OutboundModule(), testhelper.Module[*contacts.Module](env), unfenced, env.Cipher, answering{body: string(body)}, time.Now)
	_, err = stepper.ExecuteHTTP(ctx, s, automations.ExecuteHTTPInput{
		RunID: runID, StepID: env.CurrentStepID(t, runID), Attempt: httpcall.Attempt{Number: 1, Limit: 3, CreatedAt: time.Now()},
	})
	require.NoError(t, err)
	session := env.MCPClient(t, env.ScopedBearer(t, apitokens.ScopeEnrollmentsRead))

	res := call(t, session, "enrollments_get", map[string]any{"automationId": testhelper.ID(a.ID), "enrollmentId": testhelper.ID(runID)})

	require.False(t, res.IsError, text(t, res))
	enrollment := decode(t, text(t, res))
	assert.Equal(t, []any{map[string]any{
		"name":   "coupon",
		"status": float64(http.StatusOK),
		"body":   map[string]any{"untrusted_data": map[string]any{"note": injection}},
	}}, enrollment["http"])
}
