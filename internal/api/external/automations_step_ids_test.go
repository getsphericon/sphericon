package external_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

var stepAuthorScopes = []apitokens.Scope{apitokens.ScopeAutomationsRead, apitokens.ScopeAutomationsWrite}

const (
	keptPathID = "0f6b8d1e-6c2a-4a55-8f7e-3c1d2b4a5e60"
	keptStepID = "0f6b8d1e-6c2a-4a55-8f7e-3c1d2b4a5e61"
)

// stepIDs are the ids of a version's Steps, in order.
func stepIDs(p externalapi.AutomationPath) []string {
	ids := make([]string, len(p.Steps))
	for i := range p.Steps {
		ids[i] = externalStepID(&p.Steps[i])
	}
	return ids
}

func externalStepID(st *externalapi.AutomationStep) string {
	switch s := &st.OneOf; s.Type {
	case externalapi.EmailStepAutomationStepSum:
		return string(s.EmailStep.ID)
	case externalapi.WaitStepAutomationStepSum:
		return string(s.WaitStep.ID)
	case externalapi.ApplyTagStepAutomationStepSum:
		return string(s.ApplyTagStep.ID)
	case externalapi.RemoveTagStepAutomationStepSum:
		return string(s.RemoveTagStep.ID)
	case externalapi.HttpStepAutomationStepSum:
		return string(s.HttpStep.ID)
	case externalapi.TelegramStepAutomationStepSum:
		return string(s.TelegramStep.ID)
	case externalapi.EndStepAutomationStepSum:
		return string(s.EndStep.ID)
	default:
		return string(s.SmsStep.ID)
	}
}

// A version saved without ids comes back as a root path of typed Steps, the path and each Step
// given an id; the ids it was saved with are kept by the next save, and a Step added then gets one
// of its own (ADR 0122).
func TestExternalAutomationVersionIsARootPathOfTypedStepsWithIDs(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	c := env.ExternalScoped(t, stepAuthorScopes...)

	created, err := c.AutomationsCreate(ctx, &externalapi.CreateAutomationInput{
		Name: "Ids", TriggerEvent: "signup",
		Steps: externalapi.NewOptAutomationPathInput(testhelper.ExternalPath(
			externalapi.EmailStepInput{Subject: "Hi", Body: externalapi.NewOptString("<p>Hi</p>")},
			externalapi.WaitStepInput{Seconds: 60},
		)),
	})
	require.NoError(t, err)

	first := created.Draft.Value.Steps
	require.Len(t, first.Steps, 2)
	assert.NotEmpty(t, first.ID, "the root path is given an id")
	assert.Equal(t, externalapi.EmailStepAutomationStepSum, first.Steps[0].OneOf.Type)
	assert.Equal(t, "Hi", first.Steps[0].OneOf.EmailStep.Subject)
	assert.EqualValues(t, 60, first.Steps[1].OneOf.WaitStep.Seconds)
	ids := stepIDs(first)
	assert.NotEmpty(t, ids[0])
	assert.NotEmpty(t, ids[1])
	assert.NotEqual(t, ids[0], ids[1])

	resent := testhelper.ExternalPath(
		externalapi.ApplyTagStepInput{Tag: "new"},
		externalapi.EmailStepInput{ID: externalapi.NewOptStepId(externalapi.StepId(ids[0])), Subject: "Hi again", Body: externalapi.NewOptString("<p>Hi</p>")},
		externalapi.WaitStepInput{ID: externalapi.NewOptStepId(externalapi.StepId(ids[1])), Seconds: 120},
	)
	resent.ID = externalapi.NewOptStepId(first.ID)
	updated, err := c.AutomationsUpdate(ctx, &externalapi.UpdateAutomationInput{
		Revision: created.Revision, Steps: externalapi.NewOptAutomationPathInput(resent),
	}, externalapi.AutomationsUpdateParams{ID: created.ID})
	require.NoError(t, err)

	second := updated.Draft.Value.Steps
	assert.Equal(t, first.ID, second.ID, "the path keeps its id")
	got := stepIDs(second)
	require.Len(t, got, 3)
	assert.NotEmpty(t, got[0])
	assert.NotContains(t, ids, got[0], "an added Step gets an id of its own")
	assert.Equal(t, ids, got[1:], "each Step keeps the id it was saved with")
}

// An id the client minted is kept as it is.
func TestExternalAutomationKeepsTheIDsAClientMints(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	c := env.ExternalScoped(t, stepAuthorScopes...)
	path := testhelper.ExternalPath(externalapi.WaitStepInput{ID: externalapi.NewOptStepId(keptStepID), Seconds: 60})
	path.ID = externalapi.NewOptStepId(keptPathID)

	created, err := c.AutomationsCreate(ctx, &externalapi.CreateAutomationInput{
		Name: "Minted", TriggerEvent: "signup", Steps: externalapi.NewOptAutomationPathInput(path),
	})
	require.NoError(t, err)

	steps := created.Draft.Value.Steps
	assert.EqualValues(t, keptPathID, steps.ID)
	assert.Equal(t, []string{keptStepID}, stepIDs(steps))
}

// A malformed id, and one two Steps (or a Step and the path) share, refuse the save.
func TestExternalAutomationRefusesAMalformedOrDuplicateStepID(t *testing.T) {
	cases := map[string]externalapi.AutomationPathInput{
		"malformed": testhelper.ExternalPath(externalapi.WaitStepInput{ID: externalapi.NewOptStepId("step-1"), Seconds: 60}),
		"duplicate": testhelper.ExternalPath(
			externalapi.WaitStepInput{ID: externalapi.NewOptStepId(keptStepID), Seconds: 60},
			externalapi.ApplyTagStepInput{ID: externalapi.NewOptStepId(keptStepID), Tag: "vip"},
		),
		"shared with the path": {
			ID:    externalapi.NewOptStepId(keptStepID),
			Steps: []externalapi.AutomationStepInput{testhelper.ExternalStep(externalapi.WaitStepInput{ID: externalapi.NewOptStepId(keptStepID), Seconds: 60})},
		},
	}
	for name, path := range cases {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			c := env.ExternalScoped(t, stepAuthorScopes...)

			_, err := c.AutomationsCreate(t.Context(), &externalapi.CreateAutomationInput{
				Name: "Bad ids", TriggerEvent: "signup", Steps: externalapi.NewOptAutomationPathInput(path),
			})

			testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)
		})
	}
}

// A Step is closed (ADR 0148): a field nobody declares, and a field of another kind, are a 400
// naming the path of the property, the Step by its index.
func TestExternalAutomationRefusesAFieldItsStepKindDoesNotHold(t *testing.T) {
	cases := []struct {
		name  string
		field string
	}{
		{"unknown", "nope"},
		{"of another kind", "seconds"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			edited := env.ExternalVia(t, env.ScopedBearer(t, stepAuthorScopes...), testhelper.EditBody(func(body map[string]any) {
				steps := body["steps"].(map[string]any)["steps"].([]any)
				steps[1].(map[string]any)[tc.field] = 5
			}))

			_, err := edited.AutomationsCreate(t.Context(), &externalapi.CreateAutomationInput{
				Name: "Closed", TriggerEvent: "signup",
				Steps: externalapi.NewOptAutomationPathInput(testhelper.ExternalPath(
					externalapi.WaitStepInput{Seconds: 60},
					externalapi.ApplyTagStepInput{Tag: "vip"},
				)),
			})

			refused := testhelper.ExternalRefusal(t, err, http.StatusBadRequest)
			assert.Contains(t, refused.Errors.Or(nil), "steps.steps.1."+tc.field)
		})
	}
}
