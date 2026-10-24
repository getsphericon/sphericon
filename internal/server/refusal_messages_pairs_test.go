package server_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The other pairs of site and external handlers state a domain error once too: both surfaces
// render the message of the owning module. A case with a `head` compares the start of the message,
// because the module appends what is wrong with this input.

// headOf is the first n bytes of text, all of it when n is zero or past its end.
func headOf(text string, n int) string {
	if n <= 0 || n >= len(text) {
		return text
	}
	return text[:n]
}

func TestPairRefusalsReadTheSameOnBothSurfaces(t *testing.T) {
	const missing = 99999
	ws := siteapi.WorkspaceId(fixtures.AcmePublicID)
	scopes := []string{
		string(externalapi.ApiTokenScopeAutomationsRead), string(externalapi.ApiTokenScopeAutomationsWrite),
		string(externalapi.ApiTokenScopeSegmentsRead), string(externalapi.ApiTokenScopeWebhooksRead),
		string(externalapi.ApiTokenScopeWebhooksWrite), string(externalapi.ApiTokenScopeWebhooksTest),
		string(externalapi.ApiTokenScopeIntegrationsRead), string(externalapi.ApiTokenScopeContactsRead),
		string(externalapi.ApiTokenScopeContactsWrite),
	}

	cases := []struct {
		name string
		want string
		// head is how much of the message is compared: all of it when zero.
		head int
		site func(c *siteapi.Client) (any, error)
		ext  func(c *externalapi.Client) (any, error)
	}{
		{
			name: "rule not found", want: "rule not found",
			site: func(c *siteapi.Client) (any, error) {
				return c.SiteRulesGet(t.Context(), siteapi.SiteRulesGetParams{WorkspaceId: ws, ID: missing})
			},
			ext: func(c *externalapi.Client) (any, error) {
				return c.RulesGet(t.Context(), externalapi.RulesGetParams{ID: missing})
			},
		},
		{
			name: "invalid rule", want: "invalid rule: ", head: len("invalid rule: "),
			site: func(c *siteapi.Client) (any, error) {
				return c.SiteRulesCreate(t.Context(), &siteapi.SiteCreateRuleInput{Name: "x", TriggerEvents: []string{"  "},
					Actions: []siteapi.SiteRuleAction{{Type: siteapi.SiteRuleActionTypeApplyTag, Tag: siteapi.NewOptString("x")}}}, siteapi.SiteRulesCreateParams{WorkspaceId: ws})
			},
			ext: func(c *externalapi.Client) (any, error) {
				return c.RulesCreate(t.Context(), &externalapi.CreateRuleInput{Name: "x", TriggerEvents: []string{"  "},
					Actions: []externalapi.RuleAction{{Type: externalapi.RuleActionTypeApplyTag, Tag: externalapi.NewOptString("x")}}})
			},
		},
		{
			name: "segment not found", want: "segment not found",
			site: func(c *siteapi.Client) (any, error) {
				return c.SiteSegmentsGet(t.Context(), siteapi.SiteSegmentsGetParams{WorkspaceId: ws, ID: missing})
			},
			ext: func(c *externalapi.Client) (any, error) {
				return c.SegmentsGet(t.Context(), externalapi.SegmentsGetParams{ID: missing})
			},
		},
		{
			name: "automation not found", want: "automation not found",
			site: func(c *siteapi.Client) (any, error) {
				return c.SiteAutomationsGet(t.Context(), siteapi.SiteAutomationsGetParams{WorkspaceId: ws, ID: missing})
			},
			ext: func(c *externalapi.Client) (any, error) {
				return c.AutomationsGet(t.Context(), externalapi.AutomationsGetParams{ID: missing})
			},
		},
		{
			name: "invalid trigger", want: "invalid trigger: ", head: len("invalid trigger: "),
			site: func(c *siteapi.Client) (any, error) {
				return c.SiteAutomationsCreate(t.Context(), &siteapi.SiteCreateAutomationInput{Name: "x", TriggerEvent: "email.sent"},
					siteapi.SiteAutomationsCreateParams{WorkspaceId: ws})
			},
			ext: func(c *externalapi.Client) (any, error) {
				return c.AutomationsCreate(t.Context(), &externalapi.CreateAutomationInput{Name: "x", TriggerEvent: "email.sent"})
			},
		},
		{
			name: "tag name is required", want: "name is required",
			site: func(c *siteapi.Client) (any, error) {
				return c.SiteTagsApply(t.Context(), &siteapi.SiteApplyTagInput{Name: " "},
					siteapi.SiteTagsApplyParams{WorkspaceId: ws, ContactId: fixtures.ContactAliceID})
			},
			ext: func(c *externalapi.Client) (any, error) {
				return c.TagsApply(t.Context(), &externalapi.ApplyTagInput{Name: " "}, externalapi.TagsApplyParams{ContactId: fixtures.ContactAliceID})
			},
		},
		{
			name: "tag contact not found", want: "contact not found",
			site: func(c *siteapi.Client) (any, error) {
				return c.SiteTagsApply(t.Context(), &siteapi.SiteApplyTagInput{Name: "vip"},
					siteapi.SiteTagsApplyParams{WorkspaceId: ws, ContactId: missing})
			},
			ext: func(c *externalapi.Client) (any, error) {
				return c.TagsApply(t.Context(), &externalapi.ApplyTagInput{Name: "vip"}, externalapi.TagsApplyParams{ContactId: missing})
			},
		},
		{
			name: "tag not found", want: "tag not found",
			site: func(c *siteapi.Client) (any, error) {
				return c.SiteTagsDelete(t.Context(), siteapi.SiteTagsDeleteParams{WorkspaceId: ws, ID: missing})
			},
			ext: func(c *externalapi.Client) (any, error) {
				return c.TagsDelete(t.Context(), externalapi.TagsDeleteParams{ID: missing})
			},
		},
		{
			name: "webhook not found", want: "webhook not found",
			site: func(c *siteapi.Client) (any, error) {
				return c.SiteWebhooksGet(t.Context(), siteapi.SiteWebhooksGetParams{WorkspaceId: ws, ID: missing})
			},
			ext: func(c *externalapi.Client) (any, error) {
				return c.WebhooksGet(t.Context(), externalapi.WebhooksGetParams{ID: missing})
			},
		},
		{
			name: "webhook url", want: "url must be an absolute http or https URL",
			site: func(c *siteapi.Client) (any, error) {
				return c.SiteWebhooksCreate(t.Context(), &siteapi.SiteCreateWebhookEndpointInput{URL: "not a url", EventTypes: []string{"contact.created"}},
					siteapi.SiteWebhooksCreateParams{WorkspaceId: ws})
			},
			ext: func(c *externalapi.Client) (any, error) {
				return c.WebhooksCreate(t.Context(), &externalapi.CreateWebhookInput{URL: "not a url", EventTypes: []string{"contact.created"}})
			},
		},
		{
			name: "webhook event type", want: "the event type must be 1 to 100 characters",
			site: func(c *siteapi.Client) (any, error) {
				return c.SiteWebhooksTest(t.Context(), &siteapi.SiteTestWebhookInput{EventType: " "},
					siteapi.SiteWebhooksTestParams{WorkspaceId: ws, ID: missing})
			},
			ext: func(c *externalapi.Client) (any, error) {
				return c.WebhooksTest(t.Context(), &externalapi.TestWebhookInput{EventType: " "}, externalapi.WebhooksTestParams{ID: missing})
			},
		},
		{
			name: "integration not found", want: "integration not found",
			site: func(c *siteapi.Client) (any, error) {
				return c.SiteIntegrationsGet(t.Context(), siteapi.SiteIntegrationsGetParams{WorkspaceId: ws, ID: missing})
			},
			ext: func(c *externalapi.Client) (any, error) {
				return c.IntegrationsGet(t.Context(), externalapi.IntegrationsGetParams{ID: missing})
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			res, err := tc.site(env.SiteActor(t, fixtures.OwnerJohnEmail))
			siteText := detailOf(t, res, err)
			res, err = tc.ext(env.ExternalScoped(t, scopes...))
			extText := detailOf(t, res, err)

			assert.Equal(t, tc.want, headOf(siteText, tc.head), "site")
			assert.Equal(t, tc.want, headOf(extText, tc.head), "external")
		})
	}
}
