package site_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automation"
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
		r1, err := c.SiteAutomationsCreate(ctx, in, siteapi.SiteAutomationsCreateParams{WorkspaceId: foreign})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteAutomationsCreateNotFound{}, r1)

		before, err := env.DB.Automation.Query().Count(ctx)
		require.NoError(t, err)
		bad := &siteapi.SiteCreateAutomationInput{Name: "x", TriggerEvent: "contact.created", Steps: []siteapi.SiteAutomationStep{
			{Type: siteapi.SiteAutomationStepTypeApplyTag},
		}}
		r2, err := c.SiteAutomationsCreate(ctx, bad, siteapi.SiteAutomationsCreateParams{WorkspaceId: acme})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteAutomationsCreateUnprocessableEntity{}, r2, "a tag step without a tag is invalid")
		after, err := env.DB.Automation.Query().Count(ctx)
		require.NoError(t, err)
		assert.Equal(t, before, after)

		// Steps are optional: no steps creates an empty draft.
		r3, err := c.SiteAutomationsCreate(ctx, in, siteapi.SiteAutomationsCreateParams{WorkspaceId: acme})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteAutomationResource{}, r3)
	})

	t.Run("get", func(t *testing.T) {
		r1, err := c.SiteAutomationsGet(ctx, siteapi.SiteAutomationsGetParams{WorkspaceId: foreign, ID: known})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteAutomationsGetNotFound{}, r1)
		r3, err := c.SiteAutomationsGet(ctx, siteapi.SiteAutomationsGetParams{WorkspaceId: acme, ID: missing})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteAutomationsGetNotFound{}, r3)
		r4, err := c.SiteAutomationsGet(ctx, siteapi.SiteAutomationsGetParams{WorkspaceId: acme, ID: known})
		require.NoError(t, err)
		got, ok := r4.(*siteapi.SiteAutomationResource)
		require.Truef(t, ok, "got %T", r4)
		assert.Equal(t, fixtures.AutomationWelcomeSeriesName, got.Name)
	})

	t.Run("update", func(t *testing.T) {
		rename := &siteapi.SiteUpdateAutomationInput{Name: siteapi.NewOptString("Renamed")}
		r1, err := c.SiteAutomationsUpdate(ctx, rename, siteapi.SiteAutomationsUpdateParams{WorkspaceId: foreign, ID: known})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteAutomationsUpdateNotFound{}, r1)
		r3, err := c.SiteAutomationsUpdate(ctx, rename, siteapi.SiteAutomationsUpdateParams{WorkspaceId: acme, ID: missing})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteAutomationsUpdateNotFound{}, r3)
		r4, err := c.SiteAutomationsUpdate(ctx, &siteapi.SiteUpdateAutomationInput{Steps: []siteapi.SiteAutomationStep{
			{Type: siteapi.SiteAutomationStepTypeRemoveTag},
		}}, siteapi.SiteAutomationsUpdateParams{WorkspaceId: acme, ID: known})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteAutomationsUpdateUnprocessableEntity{}, r4)

		// Valid step edit round-trips.
		r5, err := c.SiteAutomationsUpdate(ctx, &siteapi.SiteUpdateAutomationInput{
			Name: siteapi.NewOptString("Renamed"),
			Steps: []siteapi.SiteAutomationStep{
				{Type: siteapi.SiteAutomationStepTypeWait, Seconds: siteapi.NewOptInt32(60)},
			},
		}, siteapi.SiteAutomationsUpdateParams{WorkspaceId: acme, ID: known})
		require.NoError(t, err)
		upd, ok := r5.(*siteapi.SiteAutomationResource)
		require.Truef(t, ok, "got %T", r5)
		assert.Equal(t, "Renamed", upd.Name)
		require.Len(t, upd.Steps, 1)
		stored, err := env.DB.Automation.Get(ctx, fixtures.AutomationWelcomeSeriesID)
		require.NoError(t, err)
		assert.Equal(t, "Renamed", stored.Name)
	})

	t.Run("activate and deactivate", func(t *testing.T) {
		a1, err := c.SiteAutomationsActivate(ctx, siteapi.SiteAutomationsActivateParams{WorkspaceId: foreign, ID: known})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteAutomationsActivateNotFound{}, a1)
		a2, err := c.SiteAutomationsActivate(ctx, siteapi.SiteAutomationsActivateParams{WorkspaceId: acme, ID: missing})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteAutomationsActivateNotFound{}, a2)

		d1, err := c.SiteAutomationsDeactivate(ctx, siteapi.SiteAutomationsDeactivateParams{WorkspaceId: foreign, ID: known})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteAutomationsDeactivateNotFound{}, d1)
		d2, err := c.SiteAutomationsDeactivate(ctx, siteapi.SiteAutomationsDeactivateParams{WorkspaceId: acme, ID: missing})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteAutomationsDeactivateNotFound{}, d2)
	})

	t.Run("delete", func(t *testing.T) {
		r1, err := c.SiteAutomationsDelete(ctx, siteapi.SiteAutomationsDeleteParams{WorkspaceId: foreign, ID: known})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteAutomationsDeleteNotFound{}, r1)
		r3, err := c.SiteAutomationsDelete(ctx, siteapi.SiteAutomationsDeleteParams{WorkspaceId: acme, ID: missing})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteAutomationsDeleteNotFound{}, r3)
		// A freshly created automation (no runs yet) is deleted for real.
		made, err := c.SiteAutomationsCreate(ctx, &siteapi.SiteCreateAutomationInput{Name: "Scratch", TriggerEvent: "contact.created"},
			siteapi.SiteAutomationsCreateParams{WorkspaceId: acme})
		require.NoError(t, err)
		scratch := made.(*siteapi.SiteAutomationResource)
		r4, err := c.SiteAutomationsDelete(ctx, siteapi.SiteAutomationsDeleteParams{WorkspaceId: acme, ID: scratch.ID})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteAutomationsDeleteNoContent{}, r4)
		exists, err := env.DB.Automation.Query().Where(automation.ID(int64(scratch.ID))).Exist(ctx)
		require.NoError(t, err)
		assert.False(t, exists)
	})
}

func TestSiteSegmentsErrorBranches(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	acme, foreign := siteapi.WorkspaceId(fixtures.AcmePublicID), siteapi.WorkspaceId(fixtures.GlobexPublicID)
	known := idStr(fixtures.SegmentProPlanID)
	missing := idStr(99999)

	r1, err := c.SiteSegmentsCreate(ctx, &siteapi.SiteCreateSegmentInput{Name: "x", Definition: testhelper.SiteRuleGroup(t, `{"kind":"group","combinator":"and","rules":[]}`)}, siteapi.SiteSegmentsCreateParams{WorkspaceId: foreign})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSegmentsCreateNotFound{}, r1)
	r2, err := c.SiteSegmentsCreate(ctx, &siteapi.SiteCreateSegmentInput{Name: "x", Definition: testhelper.SiteRuleGroup(t, `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"nope","operator":"eq","value":"x"}]}`)}, siteapi.SiteSegmentsCreateParams{WorkspaceId: acme})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSegmentsCreateUnprocessableEntity{}, r2)

	g1, err := c.SiteSegmentsGet(ctx, siteapi.SiteSegmentsGetParams{WorkspaceId: foreign, ID: known})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSegmentsGetNotFound{}, g1)
	g3, err := c.SiteSegmentsGet(ctx, siteapi.SiteSegmentsGetParams{WorkspaceId: acme, ID: idStr(fixtures.SegmentGlobexID)})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSegmentsGetNotFound{}, g3, "another workspace's segment is invisible")
	g4, err := c.SiteSegmentsGet(ctx, siteapi.SiteSegmentsGetParams{WorkspaceId: acme, ID: known})
	require.NoError(t, err)
	got, ok := g4.(*siteapi.SiteSegmentResource)
	require.Truef(t, ok, "got %T", g4)
	assert.Equal(t, fixtures.SegmentProPlanName, got.Name)

	rename := &siteapi.SiteUpdateSegmentInput{Name: siteapi.NewOptString("Renamed")}
	u1, err := c.SiteSegmentsUpdate(ctx, rename, siteapi.SiteSegmentsUpdateParams{WorkspaceId: foreign, ID: known})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSegmentsUpdateNotFound{}, u1)
	u3, err := c.SiteSegmentsUpdate(ctx, rename, siteapi.SiteSegmentsUpdateParams{WorkspaceId: acme, ID: missing})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSegmentsUpdateNotFound{}, u3)
	u4, err := c.SiteSegmentsUpdate(ctx, &siteapi.SiteUpdateSegmentInput{Definition: siteapi.NewOptSiteSegmentRuleGroup(testhelper.SiteRuleGroup(t, `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"nope","operator":"eq","value":"x"}]}`))}, siteapi.SiteSegmentsUpdateParams{WorkspaceId: acme, ID: known})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSegmentsUpdateUnprocessableEntity{}, u4)
	u5, err := c.SiteSegmentsUpdate(ctx, rename, siteapi.SiteSegmentsUpdateParams{WorkspaceId: acme, ID: known})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSegmentResource{}, u5)
	stored, err := env.DB.Segment.Get(ctx, fixtures.SegmentProPlanID)
	require.NoError(t, err)
	assert.Equal(t, "Renamed", stored.Name)

	d1, err := c.SiteSegmentsArchive(ctx, siteapi.SiteSegmentsArchiveParams{WorkspaceId: foreign, ID: known})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSegmentsArchiveNotFound{}, d1)
	d3, err := c.SiteSegmentsArchive(ctx, siteapi.SiteSegmentsArchiveParams{WorkspaceId: acme, ID: missing})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSegmentsArchiveNotFound{}, d3)
	d4, err := c.SiteSegmentsArchive(ctx, siteapi.SiteSegmentsArchiveParams{WorkspaceId: acme, ID: idStr(fixtures.SegmentBlackFridayID)})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSegmentResource{}, d4)
	archived, err := env.DB.Segment.Get(ctx, fixtures.SegmentBlackFridayID)
	require.NoError(t, err)
	assert.NotNil(t, archived.DiscardedAt)
}

func TestSiteTemplatesErrorBranches(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	acme, foreign := siteapi.WorkspaceId(fixtures.AcmePublicID), siteapi.WorkspaceId(fixtures.GlobexPublicID)
	known := idStr(fixtures.TemplateWelcomeID)
	missing := idStr(99999)

	r1, err := c.SiteTemplatesCreate(ctx, &siteapi.SiteCreateEmailTemplateInput{Name: "x"}, siteapi.SiteTemplatesCreateParams{WorkspaceId: foreign})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTemplatesCreateNotFound{}, r1)
	r2, err := c.SiteTemplatesCreate(ctx, &siteapi.SiteCreateEmailTemplateInput{
		Name: "Fresh", Subject: siteapi.NewOptString("S"), Body: siteapi.NewOptString("<p></p>"),
	}, siteapi.SiteTemplatesCreateParams{WorkspaceId: acme})
	require.NoError(t, err)
	created, ok := r2.(*siteapi.SiteEmailTemplateResource)
	require.Truef(t, ok, "got %T", r2)
	stored, err := env.DB.EmailTemplate.Get(ctx, int64(created.ID))
	require.NoError(t, err)
	assert.Equal(t, "S", stored.Subject)

	g1, err := c.SiteTemplatesGet(ctx, siteapi.SiteTemplatesGetParams{WorkspaceId: foreign, ID: known})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTemplatesGetNotFound{}, g1)
	g3, err := c.SiteTemplatesGet(ctx, siteapi.SiteTemplatesGetParams{WorkspaceId: acme, ID: idStr(fixtures.TemplateGlobexID)})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTemplatesGetNotFound{}, g3)
	g4, err := c.SiteTemplatesGet(ctx, siteapi.SiteTemplatesGetParams{WorkspaceId: acme, ID: known})
	require.NoError(t, err)
	got, ok := g4.(*siteapi.SiteEmailTemplateResource)
	require.Truef(t, ok, "got %T", g4)
	assert.Equal(t, fixtures.TemplateWelcomeName, got.Name)

	rename := &siteapi.SiteUpdateEmailTemplateInput{Name: siteapi.NewOptString("Renamed")}
	u1, err := c.SiteTemplatesUpdate(ctx, rename, siteapi.SiteTemplatesUpdateParams{WorkspaceId: foreign, ID: known})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTemplatesUpdateNotFound{}, u1)
	u3, err := c.SiteTemplatesUpdate(ctx, rename, siteapi.SiteTemplatesUpdateParams{WorkspaceId: acme, ID: missing})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTemplatesUpdateNotFound{}, u3)
	u4, err := c.SiteTemplatesUpdate(ctx, rename, siteapi.SiteTemplatesUpdateParams{WorkspaceId: acme, ID: idStr(fixtures.TemplateGlobexID)})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTemplatesUpdateNotFound{}, u4)
	u5, err := c.SiteTemplatesUpdate(ctx, rename, siteapi.SiteTemplatesUpdateParams{WorkspaceId: acme, ID: known})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteEmailTemplateResource{}, u5)
	tpl, err := env.DB.EmailTemplate.Get(ctx, fixtures.TemplateWelcomeID)
	require.NoError(t, err)
	assert.Equal(t, "Renamed", tpl.Name)
	other, err := env.DB.EmailTemplate.Get(ctx, fixtures.TemplateGlobexID)
	require.NoError(t, err)
	assert.Equal(t, fixtures.TemplateGlobexName, other.Name)

	d1, err := c.SiteTemplatesDelete(ctx, siteapi.SiteTemplatesDeleteParams{WorkspaceId: foreign, ID: known})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTemplatesDeleteNotFound{}, d1)
	d3, err := c.SiteTemplatesDelete(ctx, siteapi.SiteTemplatesDeleteParams{WorkspaceId: acme, ID: missing})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTemplatesDeleteNotFound{}, d3)
	d4, err := c.SiteTemplatesDelete(ctx, siteapi.SiteTemplatesDeleteParams{WorkspaceId: acme, ID: known})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTemplatesDeleteNoContent{}, d4)
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

	r1, err := c.SiteWebhooksCreate(ctx, &siteapi.SiteCreateWebhookEndpointInput{URL: "https://x.test/h", EventTypes: events}, siteapi.SiteWebhooksCreateParams{WorkspaceId: foreign})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteWebhooksCreateNotFound{}, r1)
	r2, err := c.SiteWebhooksCreate(ctx, &siteapi.SiteCreateWebhookEndpointInput{URL: "not a url", EventTypes: events}, siteapi.SiteWebhooksCreateParams{WorkspaceId: acme})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteWebhooksCreateUnprocessableEntity{}, r2)
	r3, err := c.SiteWebhooksCreate(ctx, &siteapi.SiteCreateWebhookEndpointInput{URL: "https://x.test/h", EventTypes: events, Enabled: siteapi.NewOptBool(false)}, siteapi.SiteWebhooksCreateParams{WorkspaceId: acme})
	require.NoError(t, err)
	created, ok := r3.(*siteapi.SiteWebhookEndpointResource)
	require.Truef(t, ok, "got %T", r3)
	stored, err := env.DB.WebhookEndpoint.Get(ctx, int64(created.ID))
	require.NoError(t, err)
	assert.False(t, stored.Enabled)
	assert.NotEmpty(t, stored.SecretEncrypted)

	g1, err := c.SiteWebhooksGet(ctx, siteapi.SiteWebhooksGetParams{WorkspaceId: foreign, ID: known})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteWebhooksGetNotFound{}, g1)
	g3, err := c.SiteWebhooksGet(ctx, siteapi.SiteWebhooksGetParams{WorkspaceId: acme, ID: idStr(fixtures.WebhookGlobexID)})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteWebhooksGetNotFound{}, g3)
	g4, err := c.SiteWebhooksGet(ctx, siteapi.SiteWebhooksGetParams{WorkspaceId: acme, ID: known})
	require.NoError(t, err)
	got, ok := g4.(*siteapi.SiteWebhookEndpointResource)
	require.Truef(t, ok, "got %T", g4)
	assert.Equal(t, fixtures.WebhookCodebasicsURL, got.URL)

	off := &siteapi.SiteUpdateWebhookEndpointInput{Enabled: siteapi.NewOptBool(false)}
	u1, err := c.SiteWebhooksUpdate(ctx, off, siteapi.SiteWebhooksUpdateParams{WorkspaceId: foreign, ID: known})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteWebhooksUpdateNotFound{}, u1)
	u3, err := c.SiteWebhooksUpdate(ctx, off, siteapi.SiteWebhooksUpdateParams{WorkspaceId: acme, ID: missing})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteWebhooksUpdateNotFound{}, u3)
	u4, err := c.SiteWebhooksUpdate(ctx, &siteapi.SiteUpdateWebhookEndpointInput{URL: siteapi.NewOptString("nope")}, siteapi.SiteWebhooksUpdateParams{WorkspaceId: acme, ID: known})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteWebhooksUpdateUnprocessableEntity{}, u4)
	u5, err := c.SiteWebhooksUpdate(ctx, &siteapi.SiteUpdateWebhookEndpointInput{
		URL: siteapi.NewOptString("https://new.test/h"), Enabled: siteapi.NewOptBool(false), EventTypes: events,
	}, siteapi.SiteWebhooksUpdateParams{WorkspaceId: acme, ID: known})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteWebhookEndpointResource{}, u5)
	hook, err := env.DB.WebhookEndpoint.Get(ctx, fixtures.WebhookCodebasicsID)
	require.NoError(t, err)
	assert.Equal(t, "https://new.test/h", hook.URL)
	assert.False(t, hook.Enabled)
	assert.Equal(t, events, hook.EventTypes)

	d1, err := c.SiteWebhooksDelete(ctx, siteapi.SiteWebhooksDeleteParams{WorkspaceId: foreign, ID: known})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteWebhooksDeleteNotFound{}, d1)
	d3, err := c.SiteWebhooksDelete(ctx, siteapi.SiteWebhooksDeleteParams{WorkspaceId: acme, ID: missing})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteWebhooksDeleteNotFound{}, d3)
	d4, err := c.SiteWebhooksDelete(ctx, siteapi.SiteWebhooksDeleteParams{WorkspaceId: acme, ID: known})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteWebhooksDeleteNoContent{}, d4)
	exists, err := env.DB.WebhookEndpoint.Query().Where(webhookendpoint.ID(fixtures.WebhookCodebasicsID)).Exist(ctx)
	require.NoError(t, err)
	assert.False(t, exists)
}
