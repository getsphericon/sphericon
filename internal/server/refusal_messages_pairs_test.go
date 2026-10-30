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

	cases := []pairCase{
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
	runPairs(t, scopes, cases)
}

// pairCase is one refusal asked of the site and the external handler of the same operation.
type pairCase struct {
	name string
	want string
	// head is how much of the message is compared: all of it when zero.
	head int
	site func(c *siteapi.Client) (any, error)
	ext  func(c *externalapi.Client) (any, error)
}

// runPairs asks each case of an Acme owner on /site and of an Acme token holding scopes on /api,
// and compares both messages with the one the case wants.
func runPairs(t *testing.T, scopes []string, cases []pairCase) {
	t.Helper()
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

// The refusals of the Contact, Privacy request, Captured message, token, Enrollment and Sending
// domain modules read the same on both surfaces too: each module returns its sentinel and the
// error handler renders it.
func TestContactAndSendingRefusalsReadTheSameOnBothSurfaces(t *testing.T) {
	const missing = 99999
	ws := siteapi.WorkspaceId(fixtures.AcmePublicID)
	scopes := []string{
		string(externalapi.ApiTokenScopeContactsRead), string(externalapi.ApiTokenScopeContactsWrite),
		string(externalapi.ApiTokenScopeContactsErase), string(externalapi.ApiTokenScopeContactsExport),
		string(externalapi.ApiTokenScopeCapturedMessagesRead), string(externalapi.ApiTokenScopeTokensWrite),
		string(externalapi.ApiTokenScopeSendingDomainsRead), string(externalapi.ApiTokenScopeSendingDomainsWrite),
		string(externalapi.ApiTokenScopeEnrollmentsRead), string(externalapi.ApiTokenScopeAutomationsRead),
		string(externalapi.ApiTokenScopeAuditRead),
	}

	cases := []pairCase{
		{
			name: "contact not found", want: "contact not found",
			site: func(c *siteapi.Client) (any, error) {
				return c.SiteContactsGet(t.Context(), siteapi.SiteContactsGetParams{WorkspaceId: ws, ID: missing})
			},
			ext: func(c *externalapi.Client) (any, error) {
				return c.ContactsGet(t.Context(), externalapi.ContactsGetParams{ID: missing})
			},
		},
		{
			name: "contact alias key taken", want: "email already exists",
			site: func(c *siteapi.Client) (any, error) {
				return c.SiteContactsCreate(t.Context(), &siteapi.SiteCreateContactInput{Email: siteapi.NewOptNilEmailAddress(fixtures.ContactAliceEmail)},
					siteapi.SiteContactsCreateParams{WorkspaceId: ws})
			},
			ext: func(c *externalapi.Client) (any, error) {
				return c.ContactsCreate(t.Context(), &externalapi.CreateContactInput{Email: externalapi.NewOptNilEmailAddress(fixtures.ContactAliceEmail)})
			},
		},
		{
			name: "erased contact not found", want: "contact not found",
			site: func(c *siteapi.Client) (any, error) {
				return c.SiteContactsDelete(t.Context(), siteapi.SiteContactsDeleteParams{WorkspaceId: ws, ID: missing})
			},
			ext: func(c *externalapi.Client) (any, error) {
				return c.ContactsDelete(t.Context(), externalapi.ContactsDeleteParams{ID: missing})
			},
		},
		{
			name: "nothing to erase for an address", want: "nothing found for this email address or visitor id",
			site: func(c *siteapi.Client) (any, error) {
				return c.SiteContactsErase(t.Context(), siteapi.SiteContactsEraseParams{WorkspaceId: ws, Email: "nobody.at.all@example.com"})
			},
			ext: func(c *externalapi.Client) (any, error) {
				return c.ContactsEraseBy(t.Context(), externalapi.ContactsEraseByParams{Email: externalapi.NewOptEmailAddress("nobody.at.all@example.com")})
			},
		},
		{
			name: "export names no contact", want: "pass exactly one of id or email",
			site: func(c *siteapi.Client) (any, error) {
				return c.SiteContactsExport(t.Context(), siteapi.SiteContactsExportParams{WorkspaceId: ws})
			},
			ext: func(c *externalapi.Client) (any, error) {
				return c.ContactsExport(t.Context(), externalapi.ContactsExportParams{})
			},
		},
		{
			name: "export contact not found", want: "contact not found",
			site: func(c *siteapi.Client) (any, error) {
				return c.SiteContactsExport(t.Context(), siteapi.SiteContactsExportParams{WorkspaceId: ws, ID: siteapi.NewOptEntityId(missing)})
			},
			ext: func(c *externalapi.Client) (any, error) {
				return c.ContactsExport(t.Context(), externalapi.ContactsExportParams{ID: externalapi.NewOptEntityId(missing)})
			},
		},
		{
			name: "captured message not found", want: "captured message not found",
			site: func(c *siteapi.Client) (any, error) {
				return c.SiteCapturedMessagesGet(t.Context(), siteapi.SiteCapturedMessagesGetParams{WorkspaceId: ws, ID: missing})
			},
			ext: func(c *externalapi.Client) (any, error) {
				return c.CapturedMessagesGet(t.Context(), externalapi.CapturedMessagesGetParams{ID: missing})
			},
		},
		{
			name: "token not found", want: "token not found",
			site: func(c *siteapi.Client) (any, error) {
				return c.SiteTokensDelete(t.Context(), siteapi.SiteTokensDeleteParams{WorkspaceId: ws, ID: missing})
			},
			ext: func(c *externalapi.Client) (any, error) {
				return c.AuthTokensDelete(t.Context(), externalapi.AuthTokensDeleteParams{ID: missing})
			},
		},
		{
			name: "enrollments of no automation", want: "automation not found",
			site: func(c *siteapi.Client) (any, error) {
				return c.SiteEnrollmentsList(t.Context(), siteapi.SiteEnrollmentsListParams{WorkspaceId: ws, AutomationId: missing})
			},
			ext: func(c *externalapi.Client) (any, error) {
				return c.EnrollmentsList(t.Context(), externalapi.EnrollmentsListParams{AutomationId: missing})
			},
		},
		{
			name: "enrollment not found", want: "enrollment not found",
			site: func(c *siteapi.Client) (any, error) {
				return c.SiteEnrollmentsGet(t.Context(), siteapi.SiteEnrollmentsGetParams{WorkspaceId: ws, AutomationId: fixtures.AutomationWinBackID, EnrollmentId: missing})
			},
			ext: func(c *externalapi.Client) (any, error) {
				return c.EnrollmentsGet(t.Context(), externalapi.EnrollmentsGetParams{AutomationId: fixtures.AutomationWinBackID, EnrollmentId: missing})
			},
		},
		{
			name: "sending domain not found", want: "sending domain not found",
			site: func(c *siteapi.Client) (any, error) {
				return c.SiteSendingDomainsGet(t.Context(), siteapi.SiteSendingDomainsGetParams{WorkspaceId: ws, ID: missing})
			},
			ext: func(c *externalapi.Client) (any, error) {
				return c.SendingDomainsGet(t.Context(), externalapi.SendingDomainsGetParams{ID: missing})
			},
		},
		{
			name: "sending domain already added", want: "this domain is already added",
			site: func(c *siteapi.Client) (any, error) {
				return c.SiteSendingDomainsCreate(t.Context(), &siteapi.SiteCreateSendingDomainInput{Domain: fixtures.SendingDomainCodebasicsDomain},
					siteapi.SiteSendingDomainsCreateParams{WorkspaceId: ws})
			},
			ext: func(c *externalapi.Client) (any, error) {
				return c.SendingDomainsCreate(t.Context(), &externalapi.CreateSendingDomainInput{Domain: fixtures.SendingDomainCodebasicsDomain})
			},
		},
		{
			name: "audit cursor never handed out", want: "the cursor is not one the audit log returned",
			site: func(c *siteapi.Client) (any, error) {
				return c.SiteAuditList(t.Context(), siteapi.SiteAuditListParams{WorkspaceId: ws, Cursor: siteapi.NewOptString("nope")})
			},
			ext: func(c *externalapi.Client) (any, error) {
				return c.AuditEntriesList(t.Context(), externalapi.AuditEntriesListParams{Cursor: externalapi.NewOptString("nope")})
			},
		},
	}
	runPairs(t, scopes, cases)
}
