package site_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A version saved through /site without ids comes back as a root path of typed Steps, the path and
// each Step given an id; the next save keeps the ids it sends, and a Step added then gets one of
// its own (ADR 0122).
func TestSiteAutomationVersionIsARootPathOfTypedStepsWithIDs(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := t.Context()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)

	created, err := c.SiteAutomationsCreate(ctx, &siteapi.CreateAutomationInput{
		Name: "Ids", Trigger: testhelper.SiteTrigger("signup"),
		Steps: testhelper.SitePathEdit(
			siteapi.EmailStepInput{Subject: "Hi", Body: siteapi.NewOptString("<p>Hi</p>")},
			siteapi.WaitStepInput{Seconds: 60},
		),
	}, siteapi.SiteAutomationsCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)

	first := created.Draft.Value.Steps
	require.Len(t, first.Steps, 2)
	assert.NotEmpty(t, first.ID, "the root path is given an id")
	assert.Equal(t, siteapi.EmailStepAutomationStepSum, first.Steps[0].OneOf.Type)
	assert.Equal(t, "Hi", first.Steps[0].OneOf.EmailStep.Subject)
	assert.EqualValues(t, 60, first.Steps[1].OneOf.WaitStep.Seconds)
	ids := siteStepIDs(first)
	assert.NotEmpty(t, ids[0])
	assert.NotEmpty(t, ids[1])
	assert.NotEqual(t, ids[0], ids[1])

	resent := testhelper.SitePath(
		siteapi.ApplyTagStepInput{Tag: "new"},
		siteapi.EmailStepInput{ID: siteapi.NewOptStepId(siteapi.StepId(ids[0])), Subject: "Hi again", Body: siteapi.NewOptString("<p>Hi</p>")},
		siteapi.WaitStepInput{ID: siteapi.NewOptStepId(siteapi.StepId(ids[1])), Seconds: 120},
	)
	resent.ID = siteapi.NewOptStepId(first.ID)
	updated, err := c.SiteAutomationsUpdate(ctx, &siteapi.UpdateAutomationInput{
		Revision: created.Revision, Steps: siteapi.NewOptAutomationPathInput(resent),
	}, siteapi.SiteAutomationsUpdateParams{WorkspaceId: workspaceID, ID: created.ID})
	require.NoError(t, err)

	second := updated.Draft.Value.Steps
	assert.Equal(t, first.ID, second.ID, "the path keeps its id")
	got := siteStepIDs(second)
	require.Len(t, got, 3)
	assert.NotEmpty(t, got[0])
	assert.NotContains(t, ids, got[0], "an added Step gets an id of its own")
	assert.Equal(t, ids, got[1:], "each Step keeps the id it was saved with")
}

const siteKeptStepID = "0f6b8d1e-6c2a-4a55-8f7e-3c1d2b4a5e61"

// A malformed id, and one two Steps (or a Step and the path) share, refuse the save, naming the
// Step by its id and kind.
func TestSiteAutomationRefusesAMalformedOrDuplicateStepID(t *testing.T) {
	cases := map[string]struct {
		path  siteapi.AutomationPathInput
		names string
	}{
		"malformed": {
			testhelper.SitePath(siteapi.WaitStepInput{ID: siteapi.NewOptStepId("step-1"), Seconds: 60}),
			`step "step-1" (wait)`,
		},
		"duplicate": {
			testhelper.SitePath(
				siteapi.WaitStepInput{ID: siteapi.NewOptStepId(siteKeptStepID), Seconds: 60},
				siteapi.ApplyTagStepInput{ID: siteapi.NewOptStepId(siteKeptStepID), Tag: "vip"},
			),
			"step " + siteKeptStepID + " (apply_tag)",
		},
		"shared with the path": {
			siteapi.AutomationPathInput{
				ID:    siteapi.NewOptStepId(siteKeptStepID),
				Steps: []siteapi.AutomationStepInput{testhelper.SiteStep(siteapi.WaitStepInput{ID: siteapi.NewOptStepId(siteKeptStepID), Seconds: 60})},
			},
			"step " + siteKeptStepID + " (wait)",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			c := env.SiteActor(t, fixtures.OwnerJohnEmail)

			_, err := c.SiteAutomationsCreate(t.Context(), &siteapi.CreateAutomationInput{
				Name: "Bad ids", Trigger: testhelper.SiteTrigger("signup"), Steps: siteapi.NewOptAutomationPathInput(tc.path),
			}, siteapi.SiteAutomationsCreateParams{WorkspaceId: siteapi.WorkspaceId(fixtures.AcmePublicID)})

			refused := testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
			assert.Contains(t, refused.Detail.Value, tc.names)
		})
	}
}

// A Step is closed (ADR 0148): a field nobody declares, and a field of another kind, are a 400
// naming the path of the property, the Step by its index.
func TestSiteAutomationRefusesAFieldItsStepKindDoesNotHold(t *testing.T) {
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
			edited := env.SiteActorVia(t, fixtures.OwnerJohnEmail, testhelper.EditBody(func(body map[string]any) {
				steps := body["steps"].(map[string]any)["steps"].([]any)
				steps[1].(map[string]any)[tc.field] = 5
			}))

			_, err := edited.SiteAutomationsCreate(t.Context(), &siteapi.CreateAutomationInput{
				Name: "Closed", Trigger: testhelper.SiteTrigger("signup"),
				Steps: testhelper.SitePathEdit(
					siteapi.WaitStepInput{Seconds: 60},
					siteapi.ApplyTagStepInput{Tag: "vip"},
				),
			}, siteapi.SiteAutomationsCreateParams{WorkspaceId: siteapi.WorkspaceId(fixtures.AcmePublicID)})

			refused := testhelper.SiteRefusal(t, err, http.StatusBadRequest)
			assert.Contains(t, refused.Errors.Or(nil), "steps.steps.1."+tc.field)
		})
	}
}
