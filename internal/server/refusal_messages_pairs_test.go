package server_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
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
	scopes := []apitokens.Scope{
		apitokens.ScopeAutomationsRead, apitokens.ScopeAutomationsWrite,
		apitokens.ScopeSegmentsRead, apitokens.ScopeWebhooksRead,
		apitokens.ScopeWebhooksWrite, apitokens.ScopeWebhooksTest,
		apitokens.ScopeIntegrationsRead, apitokens.ScopeContactsRead,
		apitokens.ScopeContactsWrite,
	}

	cases := []pairCase{
		{
			name: "rule not found", want: "rule not found",
			site: func(c *siteapi.Client) error {
				return refusalOf(c.SiteRulesGet(t.Context(), siteapi.SiteRulesGetParams{WorkspaceId: ws, ID: missing}))
			},
			ext: func(c *externalapi.Client) error {
				return refusalOf(c.RulesGet(t.Context(), externalapi.RulesGetParams{ID: missing}))
			},
		},
		{
			name: "invalid rule", want: "invalid rule: ", head: len("invalid rule: "),
			site: func(c *siteapi.Client) error {
				return refusalOf(c.SiteRulesCreate(t.Context(), &siteapi.SiteCreateRuleInput{Name: "x", TriggerEvents: []string{"  "},
					Actions: []siteapi.SiteRuleAction{{Type: siteapi.SiteRuleActionTypeApplyTag, Tag: siteapi.NewOptString("x")}}}, siteapi.SiteRulesCreateParams{WorkspaceId: ws}))
			},
			ext: func(c *externalapi.Client) error {
				return refusalOf(c.RulesCreate(t.Context(), &externalapi.CreateRuleInput{Name: "x", TriggerEvents: []string{"  "},
					Actions: []externalapi.RuleAction{{Type: externalapi.RuleActionTypeApplyTag, Tag: externalapi.NewOptString("x")}}}))
			},
		},
		{
			name: "segment not found", want: "segment not found",
			site: func(c *siteapi.Client) error {
				return refusalOf(c.SiteSegmentsGet(t.Context(), siteapi.SiteSegmentsGetParams{WorkspaceId: ws, ID: missing}))
			},
			ext: func(c *externalapi.Client) error {
				return refusalOf(c.SegmentsGet(t.Context(), externalapi.SegmentsGetParams{ID: missing}))
			},
		},
		{
			name: "automation not found", want: "automation not found",
			site: func(c *siteapi.Client) error {
				return refusalOf(c.SiteAutomationsGet(t.Context(), siteapi.SiteAutomationsGetParams{WorkspaceId: ws, ID: missing}))
			},
			ext: func(c *externalapi.Client) error {
				return refusalOf(c.AutomationsGet(t.Context(), externalapi.AutomationsGetParams{ID: missing}))
			},
		},
		{
			name: "invalid trigger", want: "invalid trigger: ", head: len("invalid trigger: "),
			site: func(c *siteapi.Client) error {
				return refusalOf(c.SiteAutomationsCreate(t.Context(), &siteapi.SiteCreateAutomationInput{Name: "x", TriggerEvent: systemevent.EmailSent},
					siteapi.SiteAutomationsCreateParams{WorkspaceId: ws}))
			},
			ext: func(c *externalapi.Client) error {
				return refusalOf(c.AutomationsCreate(t.Context(), &externalapi.CreateAutomationInput{Name: "x", TriggerEvent: systemevent.EmailSent}))
			},
		},
		{
			name: "a blank tag name", want: "name must not be empty",
			site: func(c *siteapi.Client) error {
				return refusalOf(c.SiteTagsApply(t.Context(), &siteapi.ApplyTagInput{Name: " "},
					siteapi.SiteTagsApplyParams{WorkspaceId: ws, ContactId: fixtures.ContactAliceID}))
			},
			ext: func(c *externalapi.Client) error {
				return refusalOf(c.TagsApply(t.Context(), &externalapi.ApplyTagInput{Name: " "}, externalapi.TagsApplyParams{ContactId: fixtures.ContactAliceID}))
			},
		},
		{
			name: "tag contact not found", want: "contact not found",
			site: func(c *siteapi.Client) error {
				return refusalOf(c.SiteTagsApply(t.Context(), &siteapi.ApplyTagInput{Name: "vip"},
					siteapi.SiteTagsApplyParams{WorkspaceId: ws, ContactId: missing}))
			},
			ext: func(c *externalapi.Client) error {
				return refusalOf(c.TagsApply(t.Context(), &externalapi.ApplyTagInput{Name: "vip"}, externalapi.TagsApplyParams{ContactId: missing}))
			},
		},
		{
			name: "tag not found", want: "tag not found",
			site: func(c *siteapi.Client) error {
				return c.SiteTagsDelete(t.Context(), siteapi.SiteTagsDeleteParams{WorkspaceId: ws, ID: missing})
			},
			ext: func(c *externalapi.Client) error {
				return c.TagsDelete(t.Context(), externalapi.TagsDeleteParams{ID: missing})
			},
		},
		{
			name: "webhook not found", want: "webhook not found",
			site: func(c *siteapi.Client) error {
				return refusalOf(c.SiteWebhooksGet(t.Context(), siteapi.SiteWebhooksGetParams{WorkspaceId: ws, ID: missing}))
			},
			ext: func(c *externalapi.Client) error {
				return refusalOf(c.WebhooksGet(t.Context(), externalapi.WebhooksGetParams{ID: missing}))
			},
		},
		{
			name: "webhook url", want: "url must be an absolute http or https URL",
			site: func(c *siteapi.Client) error {
				return refusalOf(c.SiteWebhooksCreate(t.Context(), &siteapi.CreateWebhookInput{URL: "not a url", EventTypes: []string{systemevent.ContactCreated}},
					siteapi.SiteWebhooksCreateParams{WorkspaceId: ws}))
			},
			ext: func(c *externalapi.Client) error {
				return refusalOf(c.WebhooksCreate(t.Context(), &externalapi.CreateWebhookInput{URL: "not a url", EventTypes: []string{systemevent.ContactCreated}}))
			},
		},
		{
			name: "webhook event type", want: "the event type must be 1 to 100 characters",
			site: func(c *siteapi.Client) error {
				return refusalOf(c.SiteWebhooksTest(t.Context(), &siteapi.TestWebhookInput{EventType: " "},
					siteapi.SiteWebhooksTestParams{WorkspaceId: ws, ID: missing}))
			},
			ext: func(c *externalapi.Client) error {
				return refusalOf(c.WebhooksTest(t.Context(), &externalapi.TestWebhookInput{EventType: " "}, externalapi.WebhooksTestParams{ID: missing}))
			},
		},
		{
			name: "integration not found", want: "integration not found",
			site: func(c *siteapi.Client) error {
				return refusalOf(c.SiteIntegrationsGet(t.Context(), siteapi.SiteIntegrationsGetParams{WorkspaceId: ws, ID: missing}))
			},
			ext: func(c *externalapi.Client) error {
				return refusalOf(c.IntegrationsGet(t.Context(), externalapi.IntegrationsGetParams{ID: missing}))
			},
		},
	}
	runPairs(t, scopes, cases)
}

// refusalOf is the error of a call's two results: the refusal a case asks for.
func refusalOf[T any](_ T, err error) error {
	return err
}

// problemDetail is the detail of the problem a refused call of either surface carries; anything
// else fails the test.
func problemDetail(t *testing.T, err error) string {
	t.Helper()
	if site, ok := errors.AsType[*siteapi.ProblemDetailsStatusCode](err); ok {
		return site.Response.Detail.Value
	}
	ext, ok := errors.AsType[*externalapi.ProblemDetailsStatusCode](err)
	require.Truef(t, ok, "not a refusal: %v", err)
	return ext.Response.Detail.Value
}

// pairCase is one refusal asked of the site and the external handler of the same operation.
type pairCase struct {
	name string
	want string
	// head is how much of the message is compared: all of it when zero.
	head int
	site func(c *siteapi.Client) error
	ext  func(c *externalapi.Client) error
}

// runPairs asks each case of an Acme owner on /site and of an Acme token holding scopes on /api,
// and compares both messages with the one the case wants.
func runPairs(t *testing.T, scopes []apitokens.Scope, cases []pairCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			siteText := problemDetail(t, tc.site(env.SiteActor(t, fixtures.OwnerJohnEmail)))
			extText := problemDetail(t, tc.ext(env.ExternalScoped(t, scopes...)))

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
	scopes := []apitokens.Scope{
		apitokens.ScopeContactsRead, apitokens.ScopeContactsWrite,
		apitokens.ScopeContactsErase, apitokens.ScopeContactsExport,
		apitokens.ScopeCapturedMessagesRead, apitokens.ScopeTokensWrite,
		apitokens.ScopeSendingDomainsRead, apitokens.ScopeSendingDomainsWrite,
		apitokens.ScopeEnrollmentsRead, apitokens.ScopeAutomationsRead,
		apitokens.ScopeAuditRead,
	}

	cases := []pairCase{
		{
			name: "contact not found", want: "contact not found",
			site: func(c *siteapi.Client) error {
				return refusalOf(c.SiteContactsGet(t.Context(), siteapi.SiteContactsGetParams{WorkspaceId: ws, ID: missing}))
			},
			ext: func(c *externalapi.Client) error {
				return refusalOf(c.ContactsGet(t.Context(), externalapi.ContactsGetParams{ID: missing}))
			},
		},
		{
			name: "contact alias key taken", want: "email already exists",
			site: func(c *siteapi.Client) error {
				return refusalOf(c.SiteContactsCreate(t.Context(), &siteapi.SiteCreateContactInput{Email: siteapi.NewOptNilEmailAddress(fixtures.ContactAliceEmail)},
					siteapi.SiteContactsCreateParams{WorkspaceId: ws}))
			},
			ext: func(c *externalapi.Client) error {
				return refusalOf(c.ContactsCreate(t.Context(), &externalapi.CreateContactInput{Email: externalapi.NewOptNilEmailAddress(fixtures.ContactAliceEmail)}))
			},
		},
		{
			name: "erased contact not found", want: "contact not found",
			site: func(c *siteapi.Client) error {
				return c.SiteContactsDelete(t.Context(), siteapi.SiteContactsDeleteParams{WorkspaceId: ws, ID: missing})
			},
			ext: func(c *externalapi.Client) error {
				return c.ContactsDelete(t.Context(), externalapi.ContactsDeleteParams{ID: missing})
			},
		},
		{
			name: "nothing to erase for an address", want: "nothing found for this email address or visitor id",
			site: func(c *siteapi.Client) error {
				return c.SiteContactsErase(t.Context(), siteapi.SiteContactsEraseParams{WorkspaceId: ws, Email: "nobody.at.all@example.com"})
			},
			ext: func(c *externalapi.Client) error {
				return c.ContactsEraseBy(t.Context(), externalapi.ContactsEraseByParams{Email: externalapi.NewOptEmailAddress("nobody.at.all@example.com")})
			},
		},
		{
			name: "export names no contact", want: "pass exactly one of id or email",
			site: func(c *siteapi.Client) error {
				return refusalOf(c.SiteContactsExport(t.Context(), siteapi.SiteContactsExportParams{WorkspaceId: ws}))
			},
			ext: func(c *externalapi.Client) error {
				return refusalOf(c.ContactsExport(t.Context(), externalapi.ContactsExportParams{}))
			},
		},
		{
			name: "export contact not found", want: "contact not found",
			site: func(c *siteapi.Client) error {
				return refusalOf(c.SiteContactsExport(t.Context(), siteapi.SiteContactsExportParams{WorkspaceId: ws, ID: siteapi.NewOptEntityId(missing)}))
			},
			ext: func(c *externalapi.Client) error {
				return refusalOf(c.ContactsExport(t.Context(), externalapi.ContactsExportParams{ID: externalapi.NewOptEntityId(missing)}))
			},
		},
		{
			name: "captured message not found", want: "captured message not found",
			site: func(c *siteapi.Client) error {
				return refusalOf(c.SiteCapturedMessagesGet(t.Context(), siteapi.SiteCapturedMessagesGetParams{WorkspaceId: ws, ID: missing}))
			},
			ext: func(c *externalapi.Client) error {
				return refusalOf(c.CapturedMessagesGet(t.Context(), externalapi.CapturedMessagesGetParams{ID: missing}))
			},
		},
		{
			name: "token not found", want: "token not found",
			site: func(c *siteapi.Client) error {
				return c.SiteTokensDelete(t.Context(), siteapi.SiteTokensDeleteParams{WorkspaceId: ws, ID: missing})
			},
			ext: func(c *externalapi.Client) error {
				return c.AuthTokensDelete(t.Context(), externalapi.AuthTokensDeleteParams{ID: missing})
			},
		},
		{
			name: "enrollments of no automation", want: "automation not found",
			site: func(c *siteapi.Client) error {
				return refusalOf(c.SiteEnrollmentsList(t.Context(), siteapi.SiteEnrollmentsListParams{WorkspaceId: ws, AutomationId: missing}))
			},
			ext: func(c *externalapi.Client) error {
				return refusalOf(c.EnrollmentsList(t.Context(), externalapi.EnrollmentsListParams{AutomationId: missing}))
			},
		},
		{
			name: "enrollment not found", want: "enrollment not found",
			site: func(c *siteapi.Client) error {
				return refusalOf(c.SiteEnrollmentsGet(t.Context(), siteapi.SiteEnrollmentsGetParams{WorkspaceId: ws, AutomationId: fixtures.AutomationWinBackID, EnrollmentId: missing}))
			},
			ext: func(c *externalapi.Client) error {
				return refusalOf(c.EnrollmentsGet(t.Context(), externalapi.EnrollmentsGetParams{AutomationId: fixtures.AutomationWinBackID, EnrollmentId: missing}))
			},
		},
		{
			name: "sending domain not found", want: "sending domain not found",
			site: func(c *siteapi.Client) error {
				return refusalOf(c.SiteSendingDomainsGet(t.Context(), siteapi.SiteSendingDomainsGetParams{WorkspaceId: ws, ID: missing}))
			},
			ext: func(c *externalapi.Client) error {
				return refusalOf(c.SendingDomainsGet(t.Context(), externalapi.SendingDomainsGetParams{ID: missing}))
			},
		},
		{
			name: "sending domain already added", want: "this domain is already added",
			site: func(c *siteapi.Client) error {
				return refusalOf(c.SiteSendingDomainsCreate(t.Context(), &siteapi.CreateSendingDomainInput{Domain: fixtures.SendingDomainCodebasicsDomain},
					siteapi.SiteSendingDomainsCreateParams{WorkspaceId: ws}))
			},
			ext: func(c *externalapi.Client) error {
				return refusalOf(c.SendingDomainsCreate(t.Context(), &externalapi.CreateSendingDomainInput{Domain: fixtures.SendingDomainCodebasicsDomain}))
			},
		},
		{
			name: "audit cursor never handed out", want: "the cursor is not one the audit log returned",
			site: func(c *siteapi.Client) error {
				return refusalOf(c.SiteAuditList(t.Context(), siteapi.SiteAuditListParams{WorkspaceId: ws, Cursor: siteapi.NewOptString("nope")}))
			},
			ext: func(c *externalapi.Client) error {
				return refusalOf(c.AuditEntriesList(t.Context(), externalapi.AuditEntriesListParams{Cursor: externalapi.NewOptString("nope")}))
			},
		},
	}
	runPairs(t, scopes, cases)
}
