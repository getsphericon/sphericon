package site_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// lintBody breaks every warning rule against Acme's saved Brand kit (accent #1a73e8, text
// #202124, Arial) and is valid HTML, so none of it blocks a save.
const lintBody = `<table><tbody><tr><td>` +
	`<img src="https://cdn.example.com/hero.png">` +
	`<p style="color:#cc0000;font-family:Georgia, serif">Sale</p>` +
	`<p style="color:#eeeeee;background-color:#ffffff">Faint print</p>` +
	`<p style="color:#202124">Brand text</p>` +
	`</td></tr></tbody></table>`

func findings(list []siteapi.LintFinding) map[siteapi.LintCode][]string {
	got := map[siteapi.LintCode][]string{}
	for _, f := range list {
		got[f.Code] = append(got[f.Code], f.Element.Value+" "+f.Detail.Value)
	}
	return got
}

func TestSiteTemplateSaveReturnsWarningsAndStoresTheContent(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := t.Context()

	created, err := c.SiteTemplatesCreate(ctx, &siteapi.CreateTemplateInput{
		Name: "Warned", Subject: siteapi.NewOptString("Hi"), Body: siteapi.NewOptString(lintBody),
	}, siteapi.SiteTemplatesCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	res := created

	got := findings(res.Warnings)
	require.Len(t, got[siteapi.LintCodeMissingAlt], 1)
	assert.Contains(t, got[siteapi.LintCodeMissingAlt][0], "https://cdn.example.com/hero.png")
	require.Len(t, got[siteapi.LintCodeLowContrast], 1)
	assert.Contains(t, got[siteapi.LintCodeLowContrast][0], "Faint print")
	assert.Contains(t, got[siteapi.LintCodeLowContrast][0], "needs 4.5:1")
	require.Len(t, got[siteapi.LintCodeOffBrandColor], 2, "red and light gray; white and #202124 are the kit's")
	assert.Contains(t, got[siteapi.LintCodeOffBrandColor][0], "Sale")
	assert.Contains(t, got[siteapi.LintCodeOffBrandColor][0], "#cc0000")
	assert.Contains(t, got[siteapi.LintCodeOffBrandColor][1], "#eeeeee")
	require.Len(t, got[siteapi.LintCodeOffBrandFont], 1)
	assert.Contains(t, got[siteapi.LintCodeOffBrandFont][0], "Sale")

	// Warnings never block: the template is stored as written.
	row := env.DB.EmailTemplate.GetX(ctx, int64(res.ID))
	assert.Equal(t, lintBody, row.Body)
}

func TestSiteTemplateSaveWithoutABrandKitSkipsBrandWarnings(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.VandelayOwnerArtEmail)

	created, err := c.SiteTemplatesCreate(t.Context(), &siteapi.CreateTemplateInput{
		Name: "Warned", Subject: siteapi.NewOptString("Hi"), Body: siteapi.NewOptString(lintBody),
	}, siteapi.SiteTemplatesCreateParams{WorkspaceId: fixtures.VandelayPublicID})
	require.NoError(t, err)
	res := created

	got := findings(res.Warnings)
	assert.Len(t, got[siteapi.LintCodeMissingAlt], 1)
	assert.Len(t, got[siteapi.LintCodeLowContrast], 1)
	assert.Empty(t, got[siteapi.LintCodeOffBrandColor], "no Brand kit saved: nothing to compare to")
	assert.Empty(t, got[siteapi.LintCodeOffBrandFont])
}

func TestSiteTemplateSaveRefusesOnlyWhatCannotRender(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := t.Context()
	before := env.DB.EmailTemplate.Query().CountX(ctx)

	_, err := c.SiteTemplatesCreate(ctx, &siteapi.CreateTemplateInput{
		Name: "Broken", Body: siteapi.NewOptString("<div><span>unclosed</div>"),
	}, siteapi.SiteTemplatesCreateParams{WorkspaceId: fixtures.AcmePublicID})
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Equal(t, before, env.DB.EmailTemplate.Query().CountX(ctx), "nothing stored")
}

func TestSiteBroadcastSaveReturnsWarnings(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := t.Context()

	created, err := c.SiteBroadcastsCreate(ctx, &siteapi.CreateBroadcastInput{
		Name:      "Warned",
		Content:   testhelper.SiteEmail(siteapi.EmailBroadcastContentInput{Subject: siteapi.NewOptString("Hi"), Body: siteapi.NewOptString(lintBody)}),
		SegmentId: siteapi.NilEntityId{Null: true},
	}, siteapi.SiteBroadcastsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	res := created
	got := findings(res.Warnings)
	assert.Len(t, got[siteapi.LintCodeMissingAlt], 1)
	assert.Len(t, got[siteapi.LintCodeOffBrandFont], 1)

	// A write of the subject alone lints what is stored.
	updated, err := c.SiteBroadcastsUpdate(ctx, &siteapi.UpdateBroadcastInput{Revision: siteapi.Revision(env.BroadcastRevision(ctx, t, int64(res.ID))), Content: testhelper.SiteEmailEdit(siteapi.EmailBroadcastContentInput{Subject: siteapi.NewOptString("Hello")})},
		siteapi.SiteBroadcastsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: res.ID})
	require.NoError(t, err)
	upd := updated
	assert.Len(t, findings(upd.Warnings)[siteapi.LintCodeMissingAlt], 1)
}

func TestSiteTemplateSaveOfACleanMessageHasNoWarnings(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	created, err := c.SiteTemplatesCreate(t.Context(), &siteapi.CreateTemplateInput{
		Name: "Clean", Body: siteapi.NewOptString(
			`<p style="color:#202124;font-family:Arial, sans-serif">Hi</p><img src="https://cdn.example.com/a.png" alt="A">`),
	}, siteapi.SiteTemplatesCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	res := created
	assert.Empty(t, res.Warnings)
}

func TestSiteTemplateReadReturnsWarnings(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := t.Context()

	created, err := c.SiteTemplatesCreate(ctx, &siteapi.CreateTemplateInput{
		Name: "Warned", Subject: siteapi.NewOptString("Hi"), Body: siteapi.NewOptString(lintBody),
	}, siteapi.SiteTemplatesCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)

	// The editor opened later shows what a save would answer.
	got, err := c.SiteTemplatesGet(ctx, siteapi.SiteTemplatesGetParams{WorkspaceId: fixtures.AcmePublicID, ID: created.ID})
	require.NoError(t, err)
	assert.Len(t, findings(got.Warnings)[siteapi.LintCodeMissingAlt], 1)
}

func TestSiteBroadcastDraftReadReturnsWarningsAndSchedules(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := t.Context()

	created, err := c.SiteBroadcastsCreate(ctx, &siteapi.CreateBroadcastInput{
		Name:      "Warned",
		Content:   testhelper.SiteEmail(siteapi.EmailBroadcastContentInput{Subject: siteapi.NewOptString("Hi"), Body: siteapi.NewOptString(lintBody)}),
		SegmentId: siteapi.NilEntityId{Null: true},
	}, siteapi.SiteBroadcastsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)

	got, err := c.SiteBroadcastsGet(ctx, siteapi.SiteBroadcastsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: created.ID})
	require.NoError(t, err)
	assert.Len(t, findings(got.Warnings)[siteapi.LintCodeMissingAlt], 1, "a draft's read carries the warnings Schedule shows")

	// Warnings never block a Schedule.
	scheduled, err := c.SiteBroadcastsSchedule(ctx, &siteapi.ScheduleBroadcastInput{
		Revision:    siteapi.Revision(env.BroadcastRevision(ctx, t, int64(created.ID))),
		ScheduledAt: siteapi.Timestamp(time.Now().Add(time.Hour)),
	}, siteapi.SiteBroadcastsScheduleParams{WorkspaceId: fixtures.AcmePublicID, ID: created.ID})
	require.NoError(t, err)
	assert.Equal(t, siteapi.BroadcastStatusScheduled, scheduled.Status)
}

func TestSiteAutomationSaveReturnsTheWarningsOfEachEmailStep(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := t.Context()
	ws := siteapi.WorkspaceId(fixtures.AcmePublicID)
	const warned, clean, nested = "6f1c1f6e-8d0a-4c2e-9a54-1d3b2f6a7c01", "6f1c1f6e-8d0a-4c2e-9a54-1d3b2f6a7c02", "6f1c1f6e-8d0a-4c2e-9a54-1d3b2f6a7c03"

	created, err := c.SiteAutomationsCreate(ctx, &siteapi.CreateAutomationInput{
		Name:    "Warned",
		Trigger: testhelper.SiteTrigger(systemevent.ContactCreated),
		Steps: testhelper.SitePathEdit(
			siteapi.EmailStepInput{ID: siteapi.NewOptStepId(warned), Subject: "Hi", Body: siteapi.NewOptString(lintBody)},
			siteapi.WaitStepInput{Seconds: 3600},
			siteapi.EmailStepInput{ID: siteapi.NewOptStepId(clean), Subject: "Again", Body: siteapi.NewOptString(
				`<p style="color:#202124;font-family:Arial, sans-serif">Hi</p>`)},
			testhelper.SiteSplit(t, `{"type":"split","paths":[`+
				`{"weight":50,"steps":[{"type":"email","id":"`+nested+`","subject":"A","body":"<img src=\"https://cdn.example.com/a.png\">"}]},`+
				`{"weight":50,"steps":[]}]}`),
		),
	}, siteapi.SiteAutomationsCreateParams{WorkspaceId: ws})
	require.NoError(t, err)

	byStep := stepFindings(created.Warnings)
	assert.Len(t, byStep, 2, "only the steps with warnings are listed: %v", byStep)
	require.Len(t, byStep[warned][siteapi.LintCodeMissingAlt], 1)
	assert.Contains(t, byStep[warned][siteapi.LintCodeMissingAlt][0], "https://cdn.example.com/hero.png")
	assert.Len(t, byStep[nested][siteapi.LintCodeMissingAlt], 1, "a step inside a fork is linted too")

	// A rename lints what the draft holds, and so does a read.
	updated, err := c.SiteAutomationsUpdate(ctx, &siteapi.UpdateAutomationInput{
		Revision: siteapi.Revision(env.AutomationRevision(ctx, t, int64(created.ID))), Name: siteapi.NewOptName("Renamed"),
	}, siteapi.SiteAutomationsUpdateParams{WorkspaceId: ws, ID: created.ID})
	require.NoError(t, err)
	assert.Len(t, stepFindings(updated.Warnings)[warned][siteapi.LintCodeMissingAlt], 1)
	got, err := c.SiteAutomationsGet(ctx, siteapi.SiteAutomationsGetParams{WorkspaceId: ws, ID: created.ID})
	require.NoError(t, err)
	assert.Len(t, stepFindings(got.Warnings)[warned][siteapi.LintCodeMissingAlt], 1)

	// Warnings never block a Publish; the live version is linted once there is no draft.
	published, err := c.SiteAutomationsPublish(ctx, &siteapi.RevisionInput{Revision: siteapi.Revision(env.AutomationRevision(ctx, t, int64(created.ID)))},
		siteapi.SiteAutomationsPublishParams{WorkspaceId: ws, ID: created.ID})
	require.NoError(t, err)
	assert.Equal(t, siteapi.AutomationStatusActive, published.Status)
	got, err = c.SiteAutomationsGet(ctx, siteapi.SiteAutomationsGetParams{WorkspaceId: ws, ID: created.ID})
	require.NoError(t, err)
	assert.Len(t, stepFindings(got.Warnings)[warned][siteapi.LintCodeMissingAlt], 1)
}

func TestSiteAutomationSaveRefusesAnEmailStepThatCannotRender(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	_, err := c.SiteAutomationsCreate(t.Context(), &siteapi.CreateAutomationInput{
		Name:    "Broken",
		Trigger: testhelper.SiteTrigger(systemevent.ContactCreated),
		Steps:   testhelper.SitePathEdit(siteapi.EmailStepInput{Subject: "Hi", Body: siteapi.NewOptString("<div><span>unclosed</div>")}),
	}, siteapi.SiteAutomationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
}

// stepFindings are the findings of each email step, by its id, as findings lists them.
func stepFindings(list []siteapi.AutomationStepWarnings) map[string]map[siteapi.LintCode][]string {
	got := map[string]map[siteapi.LintCode][]string{}
	for _, s := range list {
		got[string(s.StepId)] = findings(s.Warnings)
	}
	return got
}
