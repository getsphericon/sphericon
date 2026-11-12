package site_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/emailtemplate"
	"github.com/getsphericon/sphericon/ent/webhookendpoint"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestSiteAutomationsErrorBranches(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	acme, foreign := siteapi.WorkspaceId(fixtures.AcmePublicID), siteapi.WorkspaceId(fixtures.GlobexPublicID)
	known := idStr(fixtures.AutomationWelcomeSeriesID)
	missing := idStr(99999)

	t.Run("create", func(t *testing.T) {
		in := &siteapi.SiteCreateAutomationInput{Name: "x", TriggerEvent: "contact.created"}
		_, err := c.SiteAutomationsCreate(ctx, in, siteapi.SiteAutomationsCreateParams{WorkspaceId: foreign})
		testhelper.SiteRefusal(t, err, http.StatusNotFound)

		before, err := env.DB.Automation.Query().Count(ctx)
		require.NoError(t, err)
		bad := &siteapi.SiteCreateAutomationInput{Name: "x", TriggerEvent: "contact.created", Steps: []siteapi.SiteAutomationStep{
			{Type: siteapi.SiteAutomationStepTypeApplyTag},
		}}
		_, err = c.SiteAutomationsCreate(ctx, bad, siteapi.SiteAutomationsCreateParams{WorkspaceId: acme})
		testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity, "a tag step without a tag is invalid")
		after, err := env.DB.Automation.Query().Count(ctx)
		require.NoError(t, err)
		assert.Equal(t, before, after)

		// Steps are optional: no steps creates an empty draft.
		_, err = c.SiteAutomationsCreate(ctx, in, siteapi.SiteAutomationsCreateParams{WorkspaceId: acme})
		require.NoError(t, err)
	})

	t.Run("get", func(t *testing.T) {
		_, err := c.SiteAutomationsGet(ctx, siteapi.SiteAutomationsGetParams{WorkspaceId: foreign, ID: known})
		testhelper.SiteRefusal(t, err, http.StatusNotFound)
		_, err = c.SiteAutomationsGet(ctx, siteapi.SiteAutomationsGetParams{WorkspaceId: acme, ID: missing})
		testhelper.SiteRefusal(t, err, http.StatusNotFound)
		r4, err := c.SiteAutomationsGet(ctx, siteapi.SiteAutomationsGetParams{WorkspaceId: acme, ID: known})
		require.NoError(t, err)
		got := r4
		assert.Equal(t, fixtures.AutomationWelcomeSeriesName, got.Name)
	})

	t.Run("update", func(t *testing.T) {
		rename := &siteapi.SiteUpdateAutomationInput{Revision: 1, Name: siteapi.NewOptName("Renamed")}
		_, err := c.SiteAutomationsUpdate(ctx, rename, siteapi.SiteAutomationsUpdateParams{WorkspaceId: foreign, ID: known})
		testhelper.SiteRefusal(t, err, http.StatusNotFound)
		_, err = c.SiteAutomationsUpdate(ctx, rename, siteapi.SiteAutomationsUpdateParams{WorkspaceId: acme, ID: missing})
		testhelper.SiteRefusal(t, err, http.StatusNotFound)
		_, err = c.SiteAutomationsUpdate(ctx, &siteapi.SiteUpdateAutomationInput{Revision: siteapi.Revision(env.AutomationRevision(ctx, t, int64(known))), Steps: []siteapi.SiteAutomationStep{
			{Type: siteapi.SiteAutomationStepTypeRemoveTag},
		}}, siteapi.SiteAutomationsUpdateParams{WorkspaceId: acme, ID: known})
		testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)

		// Valid step edit round-trips.
		r5, err := c.SiteAutomationsUpdate(ctx, &siteapi.SiteUpdateAutomationInput{Revision: siteapi.Revision(env.AutomationRevision(ctx, t, int64(known))),
			Name: siteapi.NewOptName("Renamed"),
			Steps: []siteapi.SiteAutomationStep{
				{Type: siteapi.SiteAutomationStepTypeWait, Seconds: siteapi.NewOptInt32(60)},
			},
		}, siteapi.SiteAutomationsUpdateParams{WorkspaceId: acme, ID: known})
		require.NoError(t, err)
		upd := r5
		assert.Equal(t, "Renamed", upd.Name)
		require.Len(t, upd.Draft.Value.Steps, 1)
		stored, err := env.DB.Automation.Get(ctx, fixtures.AutomationWelcomeSeriesID)
		require.NoError(t, err)
		assert.Equal(t, "Renamed", stored.Name)
	})

	t.Run("activate and deactivate", func(t *testing.T) {
		_, err := c.SiteAutomationsPublish(ctx, &siteapi.RevisionInput{Revision: siteapi.Revision(env.AutomationRevision(ctx, t, int64(known)))}, siteapi.SiteAutomationsPublishParams{WorkspaceId: foreign, ID: known})
		testhelper.SiteRefusal(t, err, http.StatusNotFound)
		_, err = c.SiteAutomationsPublish(ctx, &siteapi.RevisionInput{Revision: siteapi.Revision(env.AutomationRevision(ctx, t, int64(missing)))}, siteapi.SiteAutomationsPublishParams{WorkspaceId: acme, ID: missing})
		testhelper.SiteRefusal(t, err, http.StatusNotFound)

		_, err = c.SiteAutomationsDeactivate(ctx, siteapi.SiteAutomationsDeactivateParams{WorkspaceId: foreign, ID: known})
		testhelper.SiteRefusal(t, err, http.StatusNotFound)
		_, err = c.SiteAutomationsDeactivate(ctx, siteapi.SiteAutomationsDeactivateParams{WorkspaceId: acme, ID: missing})
		testhelper.SiteRefusal(t, err, http.StatusNotFound)
	})

	t.Run("archive", func(t *testing.T) {
		_, err := c.SiteAutomationsArchive(ctx, siteapi.SiteAutomationsArchiveParams{WorkspaceId: foreign, ID: known})
		testhelper.SiteRefusal(t, err, http.StatusNotFound)
		_, err = c.SiteAutomationsArchive(ctx, siteapi.SiteAutomationsArchiveParams{WorkspaceId: acme, ID: missing})
		testhelper.SiteRefusal(t, err, http.StatusNotFound)
	})
}

func TestSiteSegmentsErrorBranches(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	acme, foreign := siteapi.WorkspaceId(fixtures.AcmePublicID), siteapi.WorkspaceId(fixtures.GlobexPublicID)
	known := idStr(fixtures.SegmentProPlanID)
	missing := idStr(99999)

	_, err := c.SiteSegmentsCreate(ctx, &siteapi.SiteCreateSegmentInput{Name: "x", Condition: testhelper.SiteRuleGroup(t, `{"kind":"group","combinator":"and","rules":[]}`)}, siteapi.SiteSegmentsCreateParams{WorkspaceId: foreign})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	_, err = c.SiteSegmentsCreate(ctx, &siteapi.SiteCreateSegmentInput{Name: "x", Condition: testhelper.SiteRuleGroup(t, `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"nope","operator":"eq","value":"x"}]}`)}, siteapi.SiteSegmentsCreateParams{WorkspaceId: acme})
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)

	_, err = c.SiteSegmentsGet(ctx, siteapi.SiteSegmentsGetParams{WorkspaceId: foreign, ID: known})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	_, err = c.SiteSegmentsGet(ctx, siteapi.SiteSegmentsGetParams{WorkspaceId: acme, ID: idStr(fixtures.SegmentGlobexID)})
	testhelper.SiteRefusal(t, err, http.StatusNotFound, "another workspace's segment is invisible")
	g4, err := c.SiteSegmentsGet(ctx, siteapi.SiteSegmentsGetParams{WorkspaceId: acme, ID: known})
	require.NoError(t, err)
	got := g4
	assert.Equal(t, fixtures.SegmentProPlanName, got.Name)

	rename := &siteapi.SiteUpdateSegmentInput{Revision: g4.Revision, Name: siteapi.NewOptName("Renamed")}
	_, err = c.SiteSegmentsUpdate(ctx, rename, siteapi.SiteSegmentsUpdateParams{WorkspaceId: foreign, ID: known})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	_, err = c.SiteSegmentsUpdate(ctx, rename, siteapi.SiteSegmentsUpdateParams{WorkspaceId: acme, ID: missing})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	_, err = c.SiteSegmentsUpdate(ctx, &siteapi.SiteUpdateSegmentInput{Revision: g4.Revision, Condition: siteapi.NewOptSiteConditionGroup(testhelper.SiteRuleGroup(t, `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"nope","operator":"eq","value":"x"}]}`))}, siteapi.SiteSegmentsUpdateParams{WorkspaceId: acme, ID: known})
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	_, err = c.SiteSegmentsUpdate(ctx, rename, siteapi.SiteSegmentsUpdateParams{WorkspaceId: acme, ID: known})
	require.NoError(t, err)

	stored, err := env.DB.Segment.Get(ctx, fixtures.SegmentProPlanID)
	require.NoError(t, err)
	assert.Equal(t, "Renamed", stored.Name)

	_, err = c.SiteSegmentsArchive(ctx, siteapi.SiteSegmentsArchiveParams{WorkspaceId: foreign, ID: known})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	_, err = c.SiteSegmentsArchive(ctx, siteapi.SiteSegmentsArchiveParams{WorkspaceId: acme, ID: missing})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	_, err = c.SiteSegmentsArchive(ctx, siteapi.SiteSegmentsArchiveParams{WorkspaceId: acme, ID: idStr(fixtures.SegmentBlackFridayID)})
	require.NoError(t, err)

	archived, err := env.DB.Segment.Get(ctx, fixtures.SegmentBlackFridayID)
	require.NoError(t, err)
	assert.NotNil(t, archived.ArchivedAt)
}

func TestSiteTemplatesErrorBranches(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	acme, foreign := siteapi.WorkspaceId(fixtures.AcmePublicID), siteapi.WorkspaceId(fixtures.GlobexPublicID)
	known := idStr(fixtures.TemplateWelcomeID)
	missing := idStr(99999)

	_, err := c.SiteTemplatesCreate(ctx, &siteapi.CreateTemplateInput{Name: "x"}, siteapi.SiteTemplatesCreateParams{WorkspaceId: foreign})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	r2, err := c.SiteTemplatesCreate(ctx, &siteapi.CreateTemplateInput{
		Name: "Fresh", Subject: siteapi.NewOptString("S"), Body: siteapi.NewOptString("<p></p>"),
	}, siteapi.SiteTemplatesCreateParams{WorkspaceId: acme})
	require.NoError(t, err)
	created := r2
	stored, err := env.DB.EmailTemplate.Get(ctx, int64(created.ID))
	require.NoError(t, err)
	assert.Equal(t, "S", stored.Subject)

	_, err = c.SiteTemplatesGet(ctx, siteapi.SiteTemplatesGetParams{WorkspaceId: foreign, ID: known})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	_, err = c.SiteTemplatesGet(ctx, siteapi.SiteTemplatesGetParams{WorkspaceId: acme, ID: idStr(fixtures.TemplateGlobexID)})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	g4, err := c.SiteTemplatesGet(ctx, siteapi.SiteTemplatesGetParams{WorkspaceId: acme, ID: known})
	require.NoError(t, err)
	got := g4
	assert.Equal(t, fixtures.TemplateWelcomeName, got.Name)

	rename := &siteapi.UpdateTemplateInput{Revision: got.Revision, Name: siteapi.NewOptName("Renamed")}
	_, err = c.SiteTemplatesUpdate(ctx, rename, siteapi.SiteTemplatesUpdateParams{WorkspaceId: foreign, ID: known})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	_, err = c.SiteTemplatesUpdate(ctx, rename, siteapi.SiteTemplatesUpdateParams{WorkspaceId: acme, ID: missing})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	_, err = c.SiteTemplatesUpdate(ctx, rename, siteapi.SiteTemplatesUpdateParams{WorkspaceId: acme, ID: idStr(fixtures.TemplateGlobexID)})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	_, err = c.SiteTemplatesUpdate(ctx, rename, siteapi.SiteTemplatesUpdateParams{WorkspaceId: acme, ID: known})
	require.NoError(t, err)

	tpl, err := env.DB.EmailTemplate.Get(ctx, fixtures.TemplateWelcomeID)
	require.NoError(t, err)
	assert.Equal(t, "Renamed", tpl.Name)
	other, err := env.DB.EmailTemplate.Get(ctx, fixtures.TemplateGlobexID)
	require.NoError(t, err)
	assert.Equal(t, fixtures.TemplateGlobexName, other.Name)

	err = c.SiteTemplatesDelete(ctx, siteapi.SiteTemplatesDeleteParams{WorkspaceId: foreign, ID: known})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	err = c.SiteTemplatesDelete(ctx, siteapi.SiteTemplatesDeleteParams{WorkspaceId: acme, ID: missing})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	err = c.SiteTemplatesDelete(ctx, siteapi.SiteTemplatesDeleteParams{WorkspaceId: acme, ID: known})
	require.NoError(t, err)
	exists, err := env.DB.EmailTemplate.Query().Where(emailtemplate.ID(fixtures.TemplateWelcomeID)).Exist(ctx)
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestSiteWebhooksErrorBranches(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	acme, foreign := siteapi.WorkspaceId(fixtures.AcmePublicID), siteapi.WorkspaceId(fixtures.GlobexPublicID)
	known := idStr(fixtures.WebhookCodebasicsID)
	missing := idStr(99999)
	events := []string{"contact.created"}

	_, err := c.SiteWebhooksCreate(ctx, &siteapi.CreateWebhookInput{URL: "https://x.test/h", EventTypes: events}, siteapi.SiteWebhooksCreateParams{WorkspaceId: foreign})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	_, err = c.SiteWebhooksCreate(ctx, &siteapi.CreateWebhookInput{URL: "not a url", EventTypes: events}, siteapi.SiteWebhooksCreateParams{WorkspaceId: acme})
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	r3, err := c.SiteWebhooksCreate(ctx, &siteapi.CreateWebhookInput{URL: "https://x.test/h", EventTypes: events, Enabled: siteapi.NewOptBool(false)}, siteapi.SiteWebhooksCreateParams{WorkspaceId: acme})
	require.NoError(t, err)
	created := r3
	stored, err := env.DB.WebhookEndpoint.Get(ctx, int64(created.ID))
	require.NoError(t, err)
	assert.NotNil(t, stored.DisabledAt)
	assert.NotEmpty(t, stored.SecretEncrypted)

	_, err = c.SiteWebhooksGet(ctx, siteapi.SiteWebhooksGetParams{WorkspaceId: foreign, ID: known})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	_, err = c.SiteWebhooksGet(ctx, siteapi.SiteWebhooksGetParams{WorkspaceId: acme, ID: idStr(fixtures.WebhookGlobexID)})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	g4, err := c.SiteWebhooksGet(ctx, siteapi.SiteWebhooksGetParams{WorkspaceId: acme, ID: known})
	require.NoError(t, err)
	got := g4
	assert.Equal(t, fixtures.WebhookCodebasicsURL, got.URL)

	off := &siteapi.UpdateWebhookInput{Enabled: siteapi.NewOptBool(false)}
	_, err = c.SiteWebhooksUpdate(ctx, off, siteapi.SiteWebhooksUpdateParams{WorkspaceId: foreign, ID: known})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	_, err = c.SiteWebhooksUpdate(ctx, off, siteapi.SiteWebhooksUpdateParams{WorkspaceId: acme, ID: missing})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	_, err = c.SiteWebhooksUpdate(ctx, &siteapi.UpdateWebhookInput{URL: siteapi.NewOptString("nope")}, siteapi.SiteWebhooksUpdateParams{WorkspaceId: acme, ID: known})
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	_, err = c.SiteWebhooksUpdate(ctx, &siteapi.UpdateWebhookInput{
		URL: siteapi.NewOptString("https://new.test/h"), Enabled: siteapi.NewOptBool(false), EventTypes: events,
	}, siteapi.SiteWebhooksUpdateParams{WorkspaceId: acme, ID: known})
	require.NoError(t, err)

	hook, err := env.DB.WebhookEndpoint.Get(ctx, fixtures.WebhookCodebasicsID)
	require.NoError(t, err)
	assert.Equal(t, "https://new.test/h", hook.URL)
	assert.NotNil(t, hook.DisabledAt)
	assert.Equal(t, events, hook.EventTypes)

	err = c.SiteWebhooksDelete(ctx, siteapi.SiteWebhooksDeleteParams{WorkspaceId: foreign, ID: known})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	err = c.SiteWebhooksDelete(ctx, siteapi.SiteWebhooksDeleteParams{WorkspaceId: acme, ID: missing})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	err = c.SiteWebhooksDelete(ctx, siteapi.SiteWebhooksDeleteParams{WorkspaceId: acme, ID: known})
	require.NoError(t, err)
	exists, err := env.DB.WebhookEndpoint.Query().Where(webhookendpoint.ID(fixtures.WebhookCodebasicsID)).Exist(ctx)
	require.NoError(t, err)
	assert.False(t, exists)
}
